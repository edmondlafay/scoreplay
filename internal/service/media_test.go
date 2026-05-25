package service_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/edmondlafaydavid/scoreplay/internal/filevalidation"
	"github.com/edmondlafaydavid/scoreplay/internal/pagination"

	"github.com/edmondlafaydavid/scoreplay/internal/model"
	"github.com/edmondlafaydavid/scoreplay/internal/repository"
	"github.com/edmondlafaydavid/scoreplay/internal/service"
)

type fakeMediaRepo struct {
	media  map[string]*model.Media
	nextID int
}

func newFakeMediaRepo() *fakeMediaRepo {
	return &fakeMediaRepo{media: make(map[string]*model.Media)}
}

func (f *fakeMediaRepo) Create(_ context.Context, p repository.CreateMediaParams) (*model.Media, error) {
	f.nextID++
	m := &model.Media{
		ID:        fmt.Sprintf("media-%d", f.nextID),
		Name:      p.Name,
		Type:      p.Type,
		FileURL:   p.FileURL,
		Tags:      []model.Tag{},
		CreatedAt: time.Now(),
	}
	f.media[m.ID] = m
	return m, nil
}

func (f *fakeMediaRepo) GetByID(_ context.Context, id string) (*model.Media, error) {
	m, ok := f.media[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return m, nil
}

func (f *fakeMediaRepo) Search(_ context.Context, tagIDs []string, _ pagination.Params) ([]model.Media, *pagination.Cursor, error) {
	var result []model.Media
	for _, m := range f.media {
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

func newMediaService(t *testing.T, tagRepo service.TagRepo) (*service.MediaService, string) {
	t.Helper()
	dir := t.TempDir()
	return service.NewMediaService(newFakeMediaRepo(), tagRepo, dir, "http://localhost"), dir
}

type nopFile struct{ r *strings.Reader }

func (n *nopFile) Read(p []byte) (int, error)                    { return n.r.Read(p) }
func (n *nopFile) ReadAt(p []byte, off int64) (int, error)       { return n.r.ReadAt(p, off) }
func (n *nopFile) Seek(offset int64, whence int) (int64, error)  { return n.r.Seek(offset, whence) }
func (n *nopFile) Close() error                                  { return nil }
func (n *nopFile) Stat() (os.FileInfo, error)                    { return nil, nil }

func newNopFile(content string) *nopFile {
	return &nopFile{r: strings.NewReader(content)}
}

// minJPEG returns the minimum byte sequence that passes JPEG magic detection.
func minJPEG() string {
	return string([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0, 0, 0, 0, 0, 0, 0}) + "fake jpeg"
}

// minMP4 returns the minimum byte sequence that passes MP4 magic detection.
func minMP4() string {
	return string([]byte{0, 0, 0, 0x18, 0x66, 0x74, 0x79, 0x70, 0x6D, 0x70, 0x34, 0x32}) + "fake mp4"
}

func TestMediaService_Create_MissingName(t *testing.T) {
	svc, _ := newMediaService(t, &fakeTagRepo{})
	_, err := svc.Create(context.Background(), service.CreateMediaInput{
		Name:     "",
		File:     newNopFile("data"),
		Filename: "photo.jpg",
	})
	if !errors.Is(err, service.ErrMediaNameEmpty) {
		t.Fatalf("expected ErrMediaNameEmpty, got %v", err)
	}
}

func TestMediaService_Create_UnsupportedFormat(t *testing.T) {
	svc, _ := newMediaService(t, &fakeTagRepo{})
	_, err := svc.Create(context.Background(), service.CreateMediaInput{
		Name:     "test",
		File:     newNopFile("data"),
		Filename: "file.pdf",
	})
	if !errors.Is(err, service.ErrUnsupportedFormat) {
		t.Fatalf("expected ErrUnsupportedFormat, got %v", err)
	}
}

func TestMediaService_Create_InvalidTagIDs(t *testing.T) {
	svc, _ := newMediaService(t, &fakeTagRepo{})
	_, err := svc.Create(context.Background(), service.CreateMediaInput{
		Name:     "test",
		TagIDs:   []string{"nonexistent"},
		File:     newNopFile(minJPEG()), // valid magic so we reach the tag check
		Filename: "photo.jpg",
	})
	if !errors.Is(err, service.ErrTagsNotFound) {
		t.Fatalf("expected ErrTagsNotFound, got %v", err)
	}
}

func TestMediaService_Create_Photo(t *testing.T) {
	repo := &fakeTagRepo{}
	ctx := context.Background()
	svc, _ := newMediaService(t, repo)

	tagSvc := service.NewTagService(repo)
	tag, _ := tagSvc.Create(ctx, "Player A")

	media, err := svc.Create(ctx, service.CreateMediaInput{
		Name:     "My Photo",
		TagIDs:   []string{tag.ID},
		File:     newNopFile(minJPEG()),
		Filename: "shot.jpg",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if media.Type != model.MediaTypePhoto {
		t.Errorf("expected photo type, got %s", media.Type)
	}
	if media.Name != "My Photo" {
		t.Errorf("expected name 'My Photo', got %s", media.Name)
	}
}

func TestMediaService_Create_Video(t *testing.T) {
	svc, _ := newMediaService(t, &fakeTagRepo{})
	media, err := svc.Create(context.Background(), service.CreateMediaInput{
		Name:     "highlight",
		File:     newNopFile(minMP4()),
		Filename: "clip.mp4",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if media.Type != model.MediaTypeVideo {
		t.Errorf("expected video type, got %s", media.Type)
	}
}

func TestMediaService_GetByID_NotFound(t *testing.T) {
	svc, _ := newMediaService(t, &fakeTagRepo{})
	_, err := svc.GetByID(context.Background(), "missing-id")
	if !errors.Is(err, service.ErrMediaNotFound) {
		t.Fatalf("expected ErrMediaNotFound, got %v", err)
	}
}

func TestMediaService_Search_NoFilter(t *testing.T) {
	tagRepo := &fakeTagRepo{}
	svc, _ := newMediaService(t, tagRepo)
	ctx := context.Background()

	for _, pair := range []struct{ name, content string }{
		{"clip.mp4", minMP4()},
		{"photo.jpg", minJPEG()},
	} {
		if _, err := svc.Create(ctx, service.CreateMediaInput{
			Name: pair.name, File: newNopFile(pair.content), Filename: pair.name,
		}); err != nil {
			t.Fatalf("seed media %q: %v", pair.name, err)
		}
	}

	results, _, err := svc.Search(ctx, nil, pagination.Params{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestMediaService_Search_EmptyStore(t *testing.T) {
	svc, _ := newMediaService(t, &fakeTagRepo{})
	results, _, err := svc.Search(context.Background(), nil, pagination.Params{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results == nil {
		t.Error("expected empty slice, got nil")
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestMediaService_Search_UnknownTag(t *testing.T) {
	tagRepo := &fakeTagRepo{}
	svc, _ := newMediaService(t, tagRepo)
	ctx := context.Background()

	if _, err := svc.Create(ctx, service.CreateMediaInput{
		Name: "clip.mp4", File: newNopFile(minMP4()), Filename: "clip.mp4",
	}); err != nil {
		t.Fatalf("seed media: %v", err)
	}

	results, _, err := svc.Search(ctx, []string{"nonexistent-tag"}, pagination.Params{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestMediaService_Create_BadMagicBytes(t *testing.T) {
	svc, _ := newMediaService(t, &fakeTagRepo{})
	_, err := svc.Create(context.Background(), service.CreateMediaInput{
		Name:     "sneaky",
		File:     newNopFile("not a real media file at all"),
		Filename: "photo.jpg", // extension claims JPEG but magic bytes say otherwise
	})
	if !errors.Is(err, service.ErrUnsupportedFormat) {
		t.Fatalf("expected ErrUnsupportedFormat, got %v", err)
	}
}

func TestMediaService_Create_FileTooLarge(t *testing.T) {
	svc, dir := newMediaService(t, &fakeTagRepo{})

	// Build content: valid JPEG magic + (MaxPhotoSize + 1) bytes total.
	oversize := make([]byte, filevalidation.MaxPhotoSize+1)
	copy(oversize, []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0, 0, 0, 0, 0, 0, 0})

	_, err := svc.Create(context.Background(), service.CreateMediaInput{
		Name:     "huge photo",
		File:     newNopFile(string(oversize)),
		Filename: "big.jpg",
	})
	if !errors.Is(err, service.ErrFileTooLarge) {
		t.Fatalf("expected ErrFileTooLarge, got %v", err)
	}

	// Partial file must be cleaned up.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("expected upload dir to be empty after rejection, got %d files", len(entries))
	}
}
