package handler_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/edmondlafaydavid/scoreplay/internal/handler"
	"github.com/edmondlafaydavid/scoreplay/internal/model"
	"github.com/edmondlafaydavid/scoreplay/internal/pagination"
	"github.com/edmondlafaydavid/scoreplay/internal/repository"
	"github.com/edmondlafaydavid/scoreplay/internal/service"
)

// stubMediaRepo for handler tests.
type stubMediaRepo struct {
	store map[string]*model.Media
}

func newStubMediaRepo() *stubMediaRepo {
	return &stubMediaRepo{store: make(map[string]*model.Media)}
}

func (s *stubMediaRepo) Create(_ context.Context, p repository.CreateMediaParams) (*model.Media, error) {
	m := &model.Media{
		ID:        "media-abc",
		Name:      p.Name,
		Type:      p.Type,
		FileURL:   p.FileURL,
		Tags:      []model.Tag{},
		CreatedAt: time.Now(),
	}
	s.store[m.ID] = m
	return m, nil
}

func (s *stubMediaRepo) GetByID(_ context.Context, id string) (*model.Media, error) {
	m, ok := s.store[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return m, nil
}

func (s *stubMediaRepo) Search(_ context.Context, tagIDs []string, _ pagination.Params) ([]model.Media, *pagination.Cursor, error) {
	var result []model.Media
	for _, m := range s.store {
		if len(tagIDs) == 0 {
			result = append(result, *m)
			continue
		}
		tagSet := make(map[string]bool, len(m.Tags))
		for _, t := range m.Tags {
			tagSet[t.ID] = true
		}
		match := true
		for _, id := range tagIDs {
			if !tagSet[id] {
				match = false
				break
			}
		}
		if match {
			result = append(result, *m)
		}
	}
	return result, nil, nil
}

func buildMultipart(t *testing.T, fields map[string]string, fileField, filename, fileContent string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	for k, v := range fields {
		w.WriteField(k, v) //nolint:errcheck
	}

	fw, err := w.CreateFormFile(fileField, filename)
	if err != nil {
		t.Fatal(err)
	}
	fw.Write([]byte(fileContent)) //nolint:errcheck
	w.Close()

	return &buf, w.FormDataContentType()
}

func newMediaHandler(t *testing.T) *handler.MediaHandler {
	t.Helper()
	dir := t.TempDir()
	tagRepo := &stubTagRepo{}
	mediaRepo := newStubMediaRepo()
	svc := service.NewMediaService(mediaRepo, tagRepo, dir, "http://localhost")
	return handler.NewMediaHandler(svc)
}

// mp4Magic returns the minimum byte sequence that passes MP4 magic detection.
func mp4Magic() string {
	return string([]byte{0, 0, 0, 0x18, 0x66, 0x74, 0x79, 0x70, 0x6D, 0x70, 0x34, 0x32}) + "fake mp4"
}

func TestCreateMedia_Success(t *testing.T) {
	h := newMediaHandler(t)

	body, ct := buildMultipart(t,
		map[string]string{"name": "Goal Clip"},
		"file", "clip.mp4", mp4Magic(),
	)

	req := httptest.NewRequest(http.MethodPost, "/media", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()

	h.CreateMedia(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d — body: %s", w.Code, w.Body.String())
	}

	var m model.Media
	if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m.Type != model.MediaTypeVideo {
		t.Errorf("expected video, got %s", m.Type)
	}
}

func TestCreateMedia_MissingName(t *testing.T) {
	h := newMediaHandler(t)

	body, ct := buildMultipart(t, map[string]string{}, "file", "img.jpg", "data")
	req := httptest.NewRequest(http.MethodPost, "/media", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()

	h.CreateMedia(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

func TestCreateMedia_UnsupportedFormat(t *testing.T) {
	h := newMediaHandler(t)

	body, ct := buildMultipart(t, map[string]string{"name": "doc"}, "file", "file.pdf", "data")
	req := httptest.NewRequest(http.MethodPost, "/media", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()

	h.CreateMedia(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

func TestCreateMedia_MissingFile(t *testing.T) {
	h := newMediaHandler(t)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("name", "test") //nolint:errcheck
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/media", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()

	h.CreateMedia(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

type mediaPage struct {
	Items      []model.Media `json:"items"`
	NextCursor *string       `json:"next_cursor"`
}

func TestListMedia_Empty(t *testing.T) {
	h := newMediaHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/media", nil)
	w := httptest.NewRecorder()

	h.ListMedia(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp mediaPage
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 0 {
		t.Errorf("expected empty items, got %d", len(resp.Items))
	}
	if resp.NextCursor != nil {
		t.Errorf("expected nil next_cursor")
	}
}

func TestListMedia_WithTagFilter(t *testing.T) {
	h := newMediaHandler(t)

	// List with an unknown tag → empty items
	req := httptest.NewRequest(http.MethodGet, "/media?tag_id=nonexistent", nil)
	w := httptest.NewRecorder()
	h.ListMedia(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp mediaPage
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 0 {
		t.Errorf("expected 0 results for unknown tag, got %d", len(resp.Items))
	}
}

func TestListMedia_InvalidCursor(t *testing.T) {
	h := newMediaHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/media?cursor=notbase64!!!", nil)
	w := httptest.NewRecorder()
	h.ListMedia(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}


func TestGetMedia_NotFound(t *testing.T) {
	h := newMediaHandler(t)

	r := chi.NewRouter()
	r.Get("/media/{id}", h.GetMedia)

	req := httptest.NewRequest(http.MethodGet, "/media/nonexistent", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
