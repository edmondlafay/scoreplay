package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/edmondlafaydavid/scoreplay/internal/model"
	"github.com/google/uuid"
)

type ClientRepository struct {
	db *sql.DB
}

func NewClientRepository(db *sql.DB) *ClientRepository {
	return &ClientRepository{db: db}
}

func (r *ClientRepository) Create(ctx context.Context, name string) (*model.Client, error) {
	c := &model.Client{ID: uuid.New().String(), Name: name}

	const q = `INSERT INTO clients (id, name) VALUES ($1, $2) RETURNING created_at`
	if err := r.db.QueryRowContext(ctx, q, c.ID, c.Name).Scan(&c.CreatedAt); err != nil {
		return nil, fmt.Errorf("insert client: %w", err)
	}

	return c, nil
}

func (r *ClientRepository) CreateAPIKey(ctx context.Context, clientID, keyHash string) (*model.APIKey, error) {
	k := &model.APIKey{ID: uuid.New().String(), ClientID: clientID}

	const q = `INSERT INTO api_keys (id, client_id, key_hash) VALUES ($1, $2, $3) RETURNING created_at`
	if err := r.db.QueryRowContext(ctx, q, k.ID, k.ClientID, keyHash).Scan(&k.CreatedAt); err != nil {
		return nil, fmt.Errorf("insert api_key: %w", err)
	}

	return k, nil
}

func (r *ClientRepository) FindByKeyHash(ctx context.Context, keyHash string) (*model.Client, error) {
	const q = `
		SELECT c.id, c.name, c.created_at
		FROM clients c
		JOIN api_keys k ON k.client_id = c.id
		WHERE k.key_hash = $1`

	var c model.Client
	err := r.db.QueryRowContext(ctx, q, keyHash).Scan(&c.ID, &c.Name, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find client by key hash: %w", err)
	}

	return &c, nil
}
