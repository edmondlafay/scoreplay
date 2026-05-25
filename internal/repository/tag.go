package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/edmondlafaydavid/scoreplay/internal/model"
	"github.com/edmondlafaydavid/scoreplay/internal/pagination"
	"github.com/google/uuid"
)

type TagRepository struct {
	db *sql.DB
}

func NewTagRepository(db *sql.DB) *TagRepository {
	return &TagRepository{db: db}
}

func (r *TagRepository) Create(ctx context.Context, name string) (*model.Tag, error) {
	tag := &model.Tag{
		ID:   uuid.New().String(),
		Name: name,
	}

	const q = `
		INSERT INTO tags (id, name)
		VALUES ($1, $2)
		RETURNING created_at`

	if err := r.db.QueryRowContext(ctx, q, tag.ID, tag.Name).Scan(&tag.CreatedAt); err != nil {
		return nil, fmt.Errorf("insert tag: %w", err)
	}

	return tag, nil
}

func (r *TagRepository) List(ctx context.Context, p pagination.Params) ([]model.Tag, *pagination.Cursor, error) {
	fetchLimit := p.Limit + 1 // one extra to detect next page

	var (
		rows *sql.Rows
		err  error
	)

	if p.Cursor == nil {
		const q = `
			SELECT id, name, created_at FROM tags
			ORDER BY created_at DESC, id DESC
			LIMIT $1`
		rows, err = r.db.QueryContext(ctx, q, fetchLimit)
	} else {
		const q = `
			SELECT id, name, created_at FROM tags
			WHERE (created_at, id) < ($2::timestamptz, $3)
			ORDER BY created_at DESC, id DESC
			LIMIT $1`
		rows, err = r.db.QueryContext(ctx, q, fetchLimit, p.Cursor.CreatedAt, p.Cursor.ID)
	}

	if err != nil {
		return nil, nil, fmt.Errorf("list tags: %w", err)
	}
	defer rows.Close()

	var tags []model.Tag
	for rows.Next() {
		var t model.Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedAt); err != nil {
			return nil, nil, fmt.Errorf("scan tag: %w", err)
		}
		tags = append(tags, t)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	var nextCursor *pagination.Cursor
	if len(tags) > p.Limit {
		last := tags[p.Limit-1]
		c := pagination.Cursor{CreatedAt: last.CreatedAt, ID: last.ID}
		nextCursor = &c
		tags = tags[:p.Limit]
	}

	return tags, nextCursor, nil
}

func (r *TagRepository) ExistsByIDs(ctx context.Context, ids []string) (bool, error) {
	if len(ids) == 0 {
		return true, nil
	}

	// Build $1,$2,... placeholders
	placeholders := make([]any, len(ids))
	var params strings.Builder
	for i, id := range ids {
		placeholders[i] = id
		if i > 0 {
			params.WriteString(",")
		}
		fmt.Fprintf(&params, "$%d", i+1)
	}

	q := fmt.Sprintf(`SELECT COUNT(*) FROM tags WHERE id IN (%s)`, params.String())
	var count int
	if err := r.db.QueryRowContext(ctx, q, placeholders...).Scan(&count); err != nil {
		return false, fmt.Errorf("count tags: %w", err)
	}

	return count == len(ids), nil
}
