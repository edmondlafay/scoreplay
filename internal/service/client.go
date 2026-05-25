package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/edmondlafaydavid/scoreplay/internal/model"
)

var ErrClientNameEmpty = errors.New("client name cannot be empty")

type ClientRepo interface {
	Create(ctx context.Context, name string) (*model.Client, error)
	CreateAPIKey(ctx context.Context, clientID, keyHash string) (*model.APIKey, error)
	FindByKeyHash(ctx context.Context, keyHash string) (*model.Client, error)
}

type ClientService struct {
	repo ClientRepo
}

func NewClientService(repo ClientRepo) *ClientService {
	return &ClientService{repo: repo}
}

type CreatedClient struct {
	Client *model.Client
	APIKey *model.APIKey
	RawKey string
}

func (s *ClientService) Create(ctx context.Context, name string) (*CreatedClient, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrClientNameEmpty
	}

	client, err := s.repo.Create(ctx, name)
	if err != nil {
		return nil, err
	}

	rawKey, keyHash, err := generateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("generate api key: %w", err)
	}

	apiKey, err := s.repo.CreateAPIKey(ctx, client.ID, keyHash)
	if err != nil {
		return nil, err
	}

	return &CreatedClient{Client: client, APIKey: apiKey, RawKey: rawKey}, nil
}

func (s *ClientService) AddKey(ctx context.Context, clientID string) (*model.APIKey, string, error) {
	rawKey, keyHash, err := generateAPIKey()
	if err != nil {
		return nil, "", fmt.Errorf("generate api key: %w", err)
	}

	apiKey, err := s.repo.CreateAPIKey(ctx, clientID, keyHash)
	if err != nil {
		return nil, "", err
	}

	return apiKey, rawKey, nil
}

func (s *ClientService) FindByRawKey(ctx context.Context, rawKey string) (*model.Client, error) {
	return s.repo.FindByKeyHash(ctx, hashKey(rawKey))
}

func generateAPIKey() (rawKey, keyHash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	rawKey = "sp_" + hex.EncodeToString(b)
	return rawKey, hashKey(rawKey), nil
}

func hashKey(rawKey string) string {
	h := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(h[:])
}
