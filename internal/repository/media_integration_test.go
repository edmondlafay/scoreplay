//go:build integration

package repository_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/edmondlafaydavid/scoreplay/internal/model"
	"github.com/edmondlafaydavid/scoreplay/internal/pagination"
	"github.com/edmondlafaydavid/scoreplay/internal/repository"
)

func createTag(t *testing.T, name string) *model.Tag {
	t.Helper()
	tag, err := repository.NewTagRepository(testDB).Create(context.Background(), name)
	if err != nil {
		t.Fatalf("createTag %q: %v", name, err)
	}
	return tag
}

func createMedia(t *testing.T, p repository.CreateMediaParams) *model.Media {
	t.Helper()
	m, err := repository.NewMediaRepository(testDB).Create(context.Background(), p)
	if err != nil {
		t.Fatalf("createMedia %q: %v", p.Name, err)
	}
	return m
}

func TestMediaRepo_Create_NoTags(t *testing.T) {
	truncate(t)

	m := createMedia(t, repository.CreateMediaParams{
		Name:    "Highlight Reel",
		Type:    model.MediaTypeVideo,
		FileURL: "http://localhost/uploads/abc.mp4",
	})

	if m.ID == "" {
		t.Error("expected non-empty ID")
	}
	if m.Name != "Highlight Reel" {
		t.Errorf("expected name 'Highlight Reel', got %q", m.Name)
	}
	if m.Type != model.MediaTypeVideo {
		t.Errorf("expected video type, got %q", m.Type)
	}
	if m.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestMediaRepo_Create_WithTags(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	tag := createTag(t, "player-a")
	m := createMedia(t, repository.CreateMediaParams{
		Name:    "Goal",
		Type:    model.MediaTypePhoto,
		FileURL: "http://localhost/uploads/goal.jpg",
		TagIDs:  []string{tag.ID},
	})

	// GetByID should include the tag.
	repo := repository.NewMediaRepository(testDB)
	fetched, err := repo.GetByID(ctx, m.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if len(fetched.Tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(fetched.Tags))
	}
	if fetched.Tags[0].ID != tag.ID {
		t.Errorf("expected tag ID %q, got %q", tag.ID, fetched.Tags[0].ID)
	}
}

func TestMediaRepo_GetByID_NotFound(t *testing.T) {
	truncate(t)
	repo := repository.NewMediaRepository(testDB)

	_, err := repo.GetByID(context.Background(), "00000000-0000-0000-0000-000000000000")
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestMediaRepo_Search_NoFilter(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	for i, name := range []string{"clip-1", "clip-2", "clip-3"} {
		createMedia(t, repository.CreateMediaParams{
			Name:    name,
			Type:    model.MediaTypeVideo,
			FileURL: "http://localhost/uploads/" + name + ".mp4",
		})
		_ = i
	}

	repo := repository.NewMediaRepository(testDB)
	results, cursor, err := repo.Search(ctx, nil, pagination.Params{Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
	if cursor != nil {
		t.Error("expected nil cursor (all fit in one page)")
	}
}

func TestMediaRepo_Search_ByTag(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	tagA := createTag(t, "team-a")
	tagB := createTag(t, "team-b")

	createMedia(t, repository.CreateMediaParams{
		Name: "clip-a", Type: model.MediaTypeVideo,
		FileURL: "http://localhost/uploads/a.mp4",
		TagIDs:  []string{tagA.ID},
	})
	createMedia(t, repository.CreateMediaParams{
		Name: "clip-b", Type: model.MediaTypeVideo,
		FileURL: "http://localhost/uploads/b.mp4",
		TagIDs:  []string{tagB.ID},
	})

	repo := repository.NewMediaRepository(testDB)
	results, _, err := repo.Search(ctx, []string{tagA.ID}, pagination.Params{Limit: 10})
	if err != nil {
		t.Fatalf("Search by tagA: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Name != "clip-a" {
		t.Errorf("expected 'clip-a', got %q", results[0].Name)
	}
}

func TestMediaRepo_Search_Intersection(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	tagA := createTag(t, "tag-a")
	tagB := createTag(t, "tag-b")

	// media-ab has both tags
	createMedia(t, repository.CreateMediaParams{
		Name: "media-ab", Type: model.MediaTypePhoto,
		FileURL: "http://localhost/uploads/ab.jpg",
		TagIDs:  []string{tagA.ID, tagB.ID},
	})
	// media-a has only tagA
	createMedia(t, repository.CreateMediaParams{
		Name: "media-a", Type: model.MediaTypePhoto,
		FileURL: "http://localhost/uploads/a.jpg",
		TagIDs:  []string{tagA.ID},
	})

	repo := repository.NewMediaRepository(testDB)

	// Search for both tags → only media-ab
	results, _, err := repo.Search(ctx, []string{tagA.ID, tagB.ID}, pagination.Params{Limit: 10})
	if err != nil {
		t.Fatalf("Search intersection: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for A∩B, got %d", len(results))
	}
	if results[0].Name != "media-ab" {
		t.Errorf("expected 'media-ab', got %q", results[0].Name)
	}
}

func TestMediaRepo_Search_Pagination(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	for _, name := range []string{"m1", "m2", "m3"} {
		createMedia(t, repository.CreateMediaParams{
			Name:    name,
			Type:    model.MediaTypeVideo,
			FileURL: "http://localhost/uploads/" + name + ".mp4",
		})
	}

	repo := repository.NewMediaRepository(testDB)

	// Page 1 — limit 2
	page1, next, err := repo.Search(ctx, nil, pagination.Params{Limit: 2})
	if err != nil {
		t.Fatalf("Search page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("expected 2 items on page1, got %d", len(page1))
	}
	if next == nil {
		t.Fatal("expected non-nil cursor after page1")
	}

	// Page 2
	page2, next2, err := repo.Search(ctx, nil, pagination.Params{Limit: 2, Cursor: next})
	if err != nil {
		t.Fatalf("Search page2: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("expected 1 item on page2, got %d", len(page2))
	}
	if next2 != nil {
		t.Error("expected nil cursor after last page")
	}

	// All IDs unique
	seen := make(map[string]bool)
	for _, m := range append(page1, page2...) {
		if seen[m.ID] {
			t.Errorf("duplicate media ID %q across pages", m.ID)
		}
		seen[m.ID] = true
	}
}

func TestMediaRepo_Search_Empty(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	repo := repository.NewMediaRepository(testDB)

	results, cursor, err := repo.Search(ctx, nil, pagination.Params{Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if results == nil {
		t.Error("expected empty slice, got nil")
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
	if cursor != nil {
		t.Error("expected nil cursor")
	}
}

func TestMediaRepo_TagsDeletedOnMediaCascade(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	tag := createTag(t, "cascade-tag")
	m := createMedia(t, repository.CreateMediaParams{
		Name: "to-delete", Type: model.MediaTypePhoto,
		FileURL: "http://localhost/uploads/del.jpg",
		TagIDs:  []string{tag.ID},
	})

	// Delete media → media_tags rows should cascade.
	if _, err := testDB.ExecContext(ctx, `DELETE FROM media WHERE id = $1`, m.ID); err != nil {
		t.Fatalf("delete media: %v", err)
	}

	var count int
	testDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_tags WHERE media_id = $1`, m.ID).Scan(&count) //nolint:errcheck
	if count != 0 {
		t.Errorf("expected 0 media_tags rows after media delete, got %d", count)
	}

	// Tag itself should still exist.
	repo := repository.NewTagRepository(testDB)
	tags, _, err := repo.List(ctx, pagination.Params{Limit: 10})
	if err != nil {
		t.Fatalf("List tags: %v", err)
	}
	if len(tags) != 1 || tags[0].ID != tag.ID {
		t.Errorf("expected tag to survive media deletion, got %+v", tags)
	}
}
