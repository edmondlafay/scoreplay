package service_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/edmondlafaydavid/scoreplay/internal/model"
	"github.com/edmondlafaydavid/scoreplay/internal/pagination"
	"github.com/edmondlafaydavid/scoreplay/internal/service"
)

type fakeTagRepo struct {
	tags   []model.Tag
	nextID int
}

func (f *fakeTagRepo) Create(_ context.Context, name string) (*model.Tag, error) {
	f.nextID++
	t := model.Tag{ID: fmt.Sprintf("id-%d", f.nextID), Name: name, CreatedAt: time.Now()}
	f.tags = append(f.tags, t)
	return &f.tags[len(f.tags)-1], nil
}

func (f *fakeTagRepo) List(_ context.Context, p pagination.Params) ([]model.Tag, *pagination.Cursor, error) {
	limit := p.Limit
	if limit <= 0 || limit > len(f.tags) {
		limit = len(f.tags)
	}
	return f.tags[:limit], nil, nil
}

func (f *fakeTagRepo) ExistsByIDs(_ context.Context, ids []string) (bool, error) {
	set := make(map[string]bool)
	for _, t := range f.tags {
		set[t.ID] = true
	}
	for _, id := range ids {
		if !set[id] {
			return false, nil
		}
	}
	return true, nil
}

func TestTagService_Create(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{"valid name", "Messi", nil},
		{"name with spaces", "  Ronaldo  ", nil},
		{"empty name", "", service.ErrTagNameEmpty},
		{"whitespace only", "   ", service.ErrTagNameEmpty},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := service.NewTagService(&fakeTagRepo{})
			tag, err := svc.Create(context.Background(), tc.input)

			if tc.wantErr != nil {
				if err != tc.wantErr {
					t.Fatalf("want error %v, got %v", tc.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tag.Name != "Messi" && tag.Name != "Ronaldo" && tag.Name != tc.input {
				t.Errorf("name not trimmed correctly: %q", tag.Name)
			}
			if tag.ID == "" {
				t.Error("tag ID is empty")
			}
		})
	}
}

func TestTagService_List_Empty(t *testing.T) {
	svc := service.NewTagService(&fakeTagRepo{})
	tags, _, err := svc.List(context.Background(), pagination.Params{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tags == nil {
		t.Error("expected empty slice, got nil")
	}
	if len(tags) != 0 {
		t.Errorf("expected 0 tags, got %d", len(tags))
	}
}

func TestTagService_List(t *testing.T) {
	repo := &fakeTagRepo{}
	svc := service.NewTagService(repo)
	ctx := context.Background()

	svc.Create(ctx, "Tag A") //nolint:errcheck
	svc.Create(ctx, "Tag B") //nolint:errcheck

	tags, _, err := svc.List(ctx, pagination.Params{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(tags))
	}
}
