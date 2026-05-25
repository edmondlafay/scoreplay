//go:build integration

package repository_test

import (
	"context"
	"testing"

	"github.com/edmondlafaydavid/scoreplay/internal/pagination"
	"github.com/edmondlafaydavid/scoreplay/internal/repository"
)

func TestTagRepo_Create(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	repo := repository.NewTagRepository(testDB)

	tag, err := repo.Create(ctx, "sports")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tag.ID == "" {
		t.Error("expected non-empty ID")
	}
	if tag.Name != "sports" {
		t.Errorf("expected name 'sports', got %q", tag.Name)
	}
	if tag.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestTagRepo_List_NoCursor(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	repo := repository.NewTagRepository(testDB)

	names := []string{"alpha", "beta", "gamma"}
	for _, n := range names {
		if _, err := repo.Create(ctx, n); err != nil {
			t.Fatalf("seed tag %q: %v", n, err)
		}
	}

	tags, cursor, err := repo.List(ctx, pagination.Params{Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(tags) != 3 {
		t.Errorf("expected 3 tags, got %d", len(tags))
	}
	if cursor != nil {
		t.Error("expected nil cursor (all fit in one page)")
	}
}

func TestTagRepo_List_Pagination(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	repo := repository.NewTagRepository(testDB)

	for _, n := range []string{"a", "b", "c"} {
		if _, err := repo.Create(ctx, n); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	// Page 1 — limit 2
	page1, next, err := repo.List(ctx, pagination.Params{Limit: 2})
	if err != nil {
		t.Fatalf("List page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("expected 2 items on page1, got %d", len(page1))
	}
	if next == nil {
		t.Fatal("expected non-nil cursor after page1")
	}

	// Page 2
	page2, next2, err := repo.List(ctx, pagination.Params{Limit: 2, Cursor: next})
	if err != nil {
		t.Fatalf("List page2: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("expected 1 item on page2, got %d", len(page2))
	}
	if next2 != nil {
		t.Error("expected nil cursor after last page")
	}

	// All IDs unique across pages
	seen := make(map[string]bool)
	for _, tag := range append(page1, page2...) {
		if seen[tag.ID] {
			t.Errorf("duplicate tag ID %q across pages", tag.ID)
		}
		seen[tag.ID] = true
	}
}

func TestTagRepo_ExistsByIDs_AllFound(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	repo := repository.NewTagRepository(testDB)

	t1, _ := repo.Create(ctx, "x")
	t2, _ := repo.Create(ctx, "y")

	ok, err := repo.ExistsByIDs(ctx, []string{t1.ID, t2.ID})
	if err != nil {
		t.Fatalf("ExistsByIDs: %v", err)
	}
	if !ok {
		t.Error("expected true for existing IDs")
	}
}

func TestTagRepo_ExistsByIDs_Missing(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	repo := repository.NewTagRepository(testDB)

	tag, _ := repo.Create(ctx, "exists")

	ok, err := repo.ExistsByIDs(ctx, []string{tag.ID, "nonexistent-uuid"})
	if err != nil {
		t.Fatalf("ExistsByIDs: %v", err)
	}
	if ok {
		t.Error("expected false when some IDs are missing")
	}
}

func TestTagRepo_ExistsByIDs_Empty(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	repo := repository.NewTagRepository(testDB)

	ok, err := repo.ExistsByIDs(ctx, nil)
	if err != nil {
		t.Fatalf("ExistsByIDs(nil): %v", err)
	}
	if !ok {
		t.Error("expected true for empty ID list (vacuously true)")
	}
}
