package service

import (
	"context"
	"errors"
	"strings"

	"github.com/edmondlafaydavid/scoreplay/internal/model"
	"github.com/edmondlafaydavid/scoreplay/internal/pagination"
)

var ErrTagNameEmpty = errors.New("tag name cannot be empty")

type TagRepo interface {
	Create(ctx context.Context, name string) (*model.Tag, error)
	List(ctx context.Context, p pagination.Params) ([]model.Tag, *pagination.Cursor, error)
	ExistsByIDs(ctx context.Context, ids []string) (bool, error)
}

type TagService struct {
	repo TagRepo
}

func NewTagService(repo TagRepo) *TagService {
	return &TagService{repo: repo}
}

func (s *TagService) Create(ctx context.Context, name string) (*model.Tag, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrTagNameEmpty
	}

	return s.repo.Create(ctx, name)
}

func (s *TagService) List(ctx context.Context, p pagination.Params) ([]model.Tag, *pagination.Cursor, error) {
	if p.Limit <= 0 {
		p.Limit = pagination.DefaultLimit
	}

	tags, cursor, err := s.repo.List(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	if tags == nil {
		tags = []model.Tag{}
	}

	return tags, cursor, nil
}
