package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"

	"github.com/edmondlafaydavid/scoreplay/internal/filevalidation"
	"github.com/edmondlafaydavid/scoreplay/internal/model"
	"github.com/edmondlafaydavid/scoreplay/internal/pagination"
	"github.com/edmondlafaydavid/scoreplay/internal/repository"
	"github.com/google/uuid"
)

var (
	ErrMediaNameEmpty    = errors.New("media name cannot be empty")
	ErrMediaNotFound     = errors.New("media not found")
	ErrTagsNotFound      = errors.New("one or more tag IDs not found")
	ErrUnsupportedFormat = filevalidation.ErrUnsupportedFormat
	ErrFileTooLarge      = errors.New("file exceeds maximum allowed size")
)

type MediaRepo interface {
	Create(ctx context.Context, p repository.CreateMediaParams) (*model.Media, error)
	GetByID(ctx context.Context, id string) (*model.Media, error)
	Search(ctx context.Context, tagIDs []string, p pagination.Params) ([]model.Media, *pagination.Cursor, error)
}

type MediaService struct {
	mediaRepo MediaRepo
	tagRepo   TagRepo
	uploadDir string
	baseURL   string
}

func NewMediaService(
	mediaRepo MediaRepo,
	tagRepo TagRepo,
	uploadDir string,
	baseURL string,
) *MediaService {
	return &MediaService{
		mediaRepo: mediaRepo,
		tagRepo:   tagRepo,
		uploadDir: uploadDir,
		baseURL:   baseURL,
	}
}

type CreateMediaInput struct {
	Name     string
	TagIDs   []string
	File     multipart.File
	Filename string
}

func (s *MediaService) Create(ctx context.Context, input CreateMediaInput) (*model.Media, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return nil, ErrMediaNameEmpty
	}

	// Detect type from magic bytes; seek back to start afterwards.
	mediaType, err := filevalidation.SniffType(input.File)
	if err != nil {
		return nil, err // ErrUnsupportedFormat propagates as-is
	}

	// Normalise tag IDs: strip whitespace, drop empty strings, deduplicate.
	// Must happen before ExistsByIDs — duplicates cause COUNT(*) < len(ids), producing a false negative.
	input.TagIDs = uniqueTagIDs(input.TagIDs)

	if len(input.TagIDs) > 0 {
		ok, err := s.tagRepo.ExistsByIDs(ctx, input.TagIDs)
		if err != nil {
			return nil, fmt.Errorf("validate tags: %w", err)
		}
		if !ok {
			return nil, ErrTagsNotFound
		}
	}

	fileURL, err := s.saveFile(input.File, input.Filename, filevalidation.MaxSize(mediaType))
	if err != nil {
		return nil, err
	}

	media, err := s.mediaRepo.Create(ctx, repository.CreateMediaParams{
		Name:    input.Name,
		Type:    mediaType,
		FileURL: fileURL,
		TagIDs:  input.TagIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("create media record: %w", err)
	}

	// Tags are hydrated inside the repository transaction — no second round trip needed.
	return media, nil
}

func (s *MediaService) Search(ctx context.Context, tagIDs []string, p pagination.Params) ([]model.Media, *pagination.Cursor, error) {
	if p.Limit <= 0 {
		p.Limit = pagination.DefaultLimit
	}

	media, cursor, err := s.mediaRepo.Search(ctx, tagIDs, p)
	if err != nil {
		return nil, nil, err
	}
	if media == nil {
		media = []model.Media{}
	}
	return media, cursor, nil
}

func (s *MediaService) GetByID(ctx context.Context, id string) (*model.Media, error) {
	media, err := s.mediaRepo.GetByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMediaNotFound
	}
	if err != nil {
		return nil, err
	}

	return media, nil
}

// uniqueTagIDs returns ids with whitespace trimmed, empty strings removed, and duplicates dropped.
// Order is preserved. This prevents ExistsByIDs from returning a false negative when the caller
// submits the same tag ID more than once (SQL COUNT deduplicated < len(ids)).
func uniqueTagIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// saveFile streams file to disk, enforcing maxBytes. Cleans up on any error.
func (s *MediaService) saveFile(file multipart.File, originalName string, maxBytes int64) (string, error) {
	ext := strings.ToLower(filepath.Ext(originalName))
	filename := uuid.New().String() + ext
	dest := filepath.Join(s.uploadDir, filename)

	out, err := os.Create(dest)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}

	// Fetch one byte beyond the limit so we can detect overflow.
	lr := &io.LimitedReader{R: file, N: maxBytes + 1}
	written, copyErr := io.Copy(out, lr)
	out.Close()

	if copyErr != nil {
		os.Remove(dest) //nolint:errcheck
		return "", fmt.Errorf("write file: %w", copyErr)
	}
	if written > maxBytes {
		os.Remove(dest) //nolint:errcheck
		return "", ErrFileTooLarge
	}

	return s.baseURL + "/uploads/" + filename, nil
}
