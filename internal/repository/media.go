package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"

	"github.com/edmondlafaydavid/scoreplay/internal/model"
	"github.com/edmondlafaydavid/scoreplay/internal/pagination"
	"github.com/google/uuid"
)

type MediaRepository struct {
	db *sql.DB
}

func NewMediaRepository(db *sql.DB) *MediaRepository {
	return &MediaRepository{db: db}
}

type CreateMediaParams struct {
	Name    string
	Type    model.MediaType
	FileURL string
	TagIDs  []string
}

func (r *MediaRepository) Create(ctx context.Context, p CreateMediaParams) (*model.Media, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	media := &model.Media{
		ID:      uuid.New().String(),
		Name:    p.Name,
		Type:    p.Type,
		FileURL: p.FileURL,
		Tags:    []model.Tag{},
	}

	const insertMedia = `
		INSERT INTO media (id, name, type, file_url)
		VALUES ($1, $2, $3, $4)
		RETURNING created_at`

	if err := tx.QueryRowContext(ctx, insertMedia, media.ID, media.Name, media.Type, media.FileURL).
		Scan(&media.CreatedAt); err != nil {
		return nil, fmt.Errorf("insert media: %w", err)
	}

	if len(p.TagIDs) > 0 {
		if err := insertMediaTags(ctx, tx, media.ID, p.TagIDs); err != nil {
			return nil, err
		}
		// Fetch tag rows within the same transaction so the caller receives a fully
		// hydrated Media — no second round trip needed after commit.
		tags, err := fetchTagsByIDs(ctx, tx, p.TagIDs)
		if err != nil {
			return nil, err
		}
		media.Tags = tags
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return media, nil
}

// insertMediaTags bulk-inserts all tag associations for a media item in one round trip.
func insertMediaTags(ctx context.Context, tx *sql.Tx, mediaID string, tagIDs []string) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO media_tags (media_id, tag_id) SELECT $1, unnest($2::text[])`,
		mediaID, pq.Array(tagIDs),
	)
	if err != nil {
		return fmt.Errorf("insert media_tags: %w", err)
	}
	return nil
}

// fetchTagsByIDs retrieves full Tag rows for the given IDs within an open transaction.
// Used by Create to hydrate the Tags field without an extra round trip after commit.
func fetchTagsByIDs(ctx context.Context, tx *sql.Tx, ids []string) ([]model.Tag, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT id, name, created_at FROM tags WHERE id = ANY($1) ORDER BY created_at`,
		pq.Array(ids),
	)
	if err != nil {
		return nil, fmt.Errorf("fetch tags: %w", err)
	}
	defer rows.Close()

	tags := make([]model.Tag, 0, len(ids))
	for rows.Next() {
		var t model.Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

func (r *MediaRepository) GetByID(ctx context.Context, id string) (*model.Media, error) {
	const q = `
		SELECT m.id, m.name, m.type, m.file_url, m.created_at,
		       t.id, t.name, t.created_at
		FROM media m
		LEFT JOIN media_tags mt ON mt.media_id = m.id
		LEFT JOIN tags t ON t.id = mt.tag_id
		WHERE m.id = $1
		ORDER BY t.created_at`

	rows, err := r.db.QueryContext(ctx, q, id)
	if err != nil {
		return nil, fmt.Errorf("query media: %w", err)
	}
	defer rows.Close()

	results, err := scanMediaRows(rows)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, sql.ErrNoRows
	}
	return &results[0], nil
}

// Search returns a paginated list of media, optionally filtered to those
// having ALL of the given tag IDs. Uses idx_media_tags_tag_id for the
// intersection subquery. Implements keyset pagination on (created_at DESC, id DESC).
func (r *MediaRepository) Search(ctx context.Context, tagIDs []string, p pagination.Params) ([]model.Media, *pagination.Cursor, error) {
	fetchLimit := p.Limit + 1

	// Step 1 — fetch a page of (id, created_at) pairs, one variant per case.
	type idRow struct {
		id        string
		createdAt time.Time
	}

	var (
		idRows []idRow
		rows   *sql.Rows
		err    error
	)

	hasTags := len(tagIDs) > 0
	hasCursor := p.Cursor != nil

	switch {
	case !hasTags && !hasCursor:
		rows, err = r.db.QueryContext(ctx, `
			SELECT id, created_at FROM media
			ORDER BY created_at DESC, id DESC
			LIMIT $1`, fetchLimit)

	case !hasTags && hasCursor:
		rows, err = r.db.QueryContext(ctx, `
			SELECT id, created_at FROM media
			WHERE (created_at, id) < ($2::timestamptz, $3)
			ORDER BY created_at DESC, id DESC
			LIMIT $1`, fetchLimit, p.Cursor.CreatedAt, p.Cursor.ID)

	case hasTags && !hasCursor:
		rows, err = r.db.QueryContext(ctx, `
			SELECT med.id, med.created_at FROM media med
			JOIN media_tags mt ON mt.media_id = med.id
			WHERE mt.tag_id = ANY($2)
			GROUP BY med.id, med.created_at
			HAVING COUNT(DISTINCT mt.tag_id) = $3
			ORDER BY med.created_at DESC, med.id DESC
			LIMIT $1`, fetchLimit, pq.Array(tagIDs), len(tagIDs))

	default: // hasTags && hasCursor
		rows, err = r.db.QueryContext(ctx, `
			SELECT med.id, med.created_at FROM media med
			JOIN media_tags mt ON mt.media_id = med.id
			WHERE mt.tag_id = ANY($2)
			  AND (med.created_at, med.id) < ($4::timestamptz, $5)
			GROUP BY med.id, med.created_at
			HAVING COUNT(DISTINCT mt.tag_id) = $3
			ORDER BY med.created_at DESC, med.id DESC
			LIMIT $1`, fetchLimit, pq.Array(tagIDs), len(tagIDs), p.Cursor.CreatedAt, p.Cursor.ID)
	}

	if err != nil {
		return nil, nil, fmt.Errorf("search media ids: %w", err)
	}

	for rows.Next() {
		var r idRow
		if err := rows.Scan(&r.id, &r.createdAt); err != nil {
			rows.Close()
			return nil, nil, fmt.Errorf("scan media id row: %w", err)
		}
		idRows = append(idRows, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	// Determine next cursor before trimming the extra row.
	var nextCursor *pagination.Cursor
	if len(idRows) > p.Limit {
		last := idRows[p.Limit-1]
		c := pagination.Cursor{CreatedAt: last.createdAt, ID: last.id}
		nextCursor = &c
		idRows = idRows[:p.Limit]
	}

	if len(idRows) == 0 {
		return []model.Media{}, nextCursor, nil
	}

	// Step 2 — fetch full media + tags for the page IDs in one query.
	ids := make([]string, len(idRows))
	for i, r := range idRows {
		ids[i] = r.id
	}

	detailRows, err := r.db.QueryContext(ctx, `
		SELECT m.id, m.name, m.type, m.file_url, m.created_at,
		       t.id, t.name, t.created_at
		FROM media m
		LEFT JOIN media_tags mt ON mt.media_id = m.id
		LEFT JOIN tags t ON t.id = mt.tag_id
		WHERE m.id = ANY($1)
		ORDER BY m.created_at DESC, m.id DESC, t.created_at`,
		pq.Array(ids))
	if err != nil {
		return nil, nil, fmt.Errorf("fetch media details: %w", err)
	}
	defer detailRows.Close()

	results, err := scanMediaRows(detailRows)
	if err != nil {
		return nil, nil, err
	}

	return results, nextCursor, nil
}

// scanMediaRows collapses the tag-joined rows into a slice of Media,
// preserving the order returned by the query.
func scanMediaRows(rows *sql.Rows) ([]model.Media, error) {
	var order []string
	byID := make(map[string]*model.Media)

	for rows.Next() {
		var (
			m            model.Media
			tagID        sql.NullString
			tagName      sql.NullString
			tagCreatedAt sql.NullTime
		)
		if err := rows.Scan(
			&m.ID, &m.Name, &m.Type, &m.FileURL, &m.CreatedAt,
			&tagID, &tagName, &tagCreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan media row: %w", err)
		}

		if _, seen := byID[m.ID]; !seen {
			m.Tags = []model.Tag{}
			byID[m.ID] = &m
			order = append(order, m.ID)
		}

		if tagID.Valid {
			byID[m.ID].Tags = append(byID[m.ID].Tags, model.Tag{
				ID:        tagID.String,
				Name:      tagName.String,
				CreatedAt: tagCreatedAt.Time,
			})
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make([]model.Media, 0, len(order))
	for _, id := range order {
		result = append(result, *byID[id])
	}
	return result, nil
}
