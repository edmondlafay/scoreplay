package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/edmondlafaydavid/scoreplay/internal/handler"
	"github.com/edmondlafaydavid/scoreplay/internal/model"
	"github.com/edmondlafaydavid/scoreplay/internal/pagination"
	"github.com/edmondlafaydavid/scoreplay/internal/service"
)

// stubTagRepo satisfies service.TagRepo for handler-level tests.
type stubTagRepo struct {
	tags []model.Tag
}

func (s *stubTagRepo) Create(_ context.Context, name string) (*model.Tag, error) {
	t := model.Tag{ID: "tag-1", Name: name, CreatedAt: time.Now()}
	s.tags = append(s.tags, t)
	return &s.tags[len(s.tags)-1], nil
}

func (s *stubTagRepo) List(_ context.Context, _ pagination.Params) ([]model.Tag, *pagination.Cursor, error) {
	return s.tags, nil, nil
}

func (s *stubTagRepo) ExistsByIDs(_ context.Context, ids []string) (bool, error) {
	return true, nil
}

func TestCreateTag_Success(t *testing.T) {
	h := handler.NewTagHandler(service.NewTagService(&stubTagRepo{}))

	body := `{"name":"Messi"}`
	req := httptest.NewRequest(http.MethodPost, "/tags", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateTag(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	var tag model.Tag
	if err := json.NewDecoder(w.Body).Decode(&tag); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if tag.Name != "Messi" {
		t.Errorf("expected name Messi, got %s", tag.Name)
	}
}

func TestCreateTag_EmptyName(t *testing.T) {
	h := handler.NewTagHandler(service.NewTagService(&stubTagRepo{}))

	body := `{"name":""}`
	req := httptest.NewRequest(http.MethodPost, "/tags", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateTag(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

func TestCreateTag_InvalidJSON(t *testing.T) {
	h := handler.NewTagHandler(service.NewTagService(&stubTagRepo{}))

	req := httptest.NewRequest(http.MethodPost, "/tags", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateTag(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestListTags_Empty(t *testing.T) {
	h := handler.NewTagHandler(service.NewTagService(&stubTagRepo{}))

	req := httptest.NewRequest(http.MethodGet, "/tags", nil)
	w := httptest.NewRecorder()

	h.ListTags(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Items      []model.Tag `json:"items"`
		NextCursor *string     `json:"next_cursor"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 0 {
		t.Errorf("expected empty items, got %d", len(resp.Items))
	}
	if resp.NextCursor != nil {
		t.Errorf("expected nil next_cursor, got %q", *resp.NextCursor)
	}
}

// failTagRepo simulates a repo error.
type failTagRepo struct{}

func (f *failTagRepo) Create(_ context.Context, _ string) (*model.Tag, error) {
	return nil, errors.New("db error")
}
func (f *failTagRepo) List(_ context.Context, _ pagination.Params) ([]model.Tag, *pagination.Cursor, error) {
	return nil, nil, errors.New("db error")
}
func (f *failTagRepo) ExistsByIDs(_ context.Context, _ []string) (bool, error) {
	return false, errors.New("db error")
}

func TestCreateTag_DBError(t *testing.T) {
	h := handler.NewTagHandler(service.NewTagService(&failTagRepo{}))

	body := `{"name":"Messi"}`
	req := httptest.NewRequest(http.MethodPost, "/tags", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateTag(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
