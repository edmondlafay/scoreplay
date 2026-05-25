//go:build integration

package repository_test

import (
	"context"
	"testing"

	"github.com/edmondlafaydavid/scoreplay/internal/repository"
)

func TestClientRepo_Create(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	repo := repository.NewClientRepository(testDB)

	client, err := repo.Create(ctx, "Acme Corp")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if client.ID == "" {
		t.Error("expected non-empty ID")
	}
	if client.Name != "Acme Corp" {
		t.Errorf("expected name 'Acme Corp', got %q", client.Name)
	}
	if client.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestClientRepo_FindByKeyHash(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	repo := repository.NewClientRepository(testDB)

	client, err := repo.Create(ctx, "Test Client")
	if err != nil {
		t.Fatalf("Create client: %v", err)
	}

	const hash = "abc123hashvalue"
	if _, err := repo.CreateAPIKey(ctx, client.ID, hash); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	found, err := repo.FindByKeyHash(ctx, hash)
	if err != nil {
		t.Fatalf("FindByKeyHash: %v", err)
	}
	if found == nil {
		t.Fatal("expected client, got nil")
	}
	if found.ID != client.ID {
		t.Errorf("expected client ID %q, got %q", client.ID, found.ID)
	}
}

func TestClientRepo_FindByKeyHash_NotFound(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	repo := repository.NewClientRepository(testDB)

	found, err := repo.FindByKeyHash(ctx, "nonexistent-hash")
	if err != nil {
		t.Fatalf("FindByKeyHash: %v", err)
	}
	if found != nil {
		t.Errorf("expected nil for unknown hash, got %+v", found)
	}
}

func TestClientRepo_CascadeDelete(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	repo := repository.NewClientRepository(testDB)

	client, err := repo.Create(ctx, "Delete Me")
	if err != nil {
		t.Fatalf("Create client: %v", err)
	}
	const hash = "cascade-test-hash"
	if _, err := repo.CreateAPIKey(ctx, client.ID, hash); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	// Deleting the client should cascade to api_keys.
	if _, err := testDB.ExecContext(ctx, `DELETE FROM clients WHERE id = $1`, client.ID); err != nil {
		t.Fatalf("delete client: %v", err)
	}

	found, err := repo.FindByKeyHash(ctx, hash)
	if err != nil {
		t.Fatalf("FindByKeyHash after cascade: %v", err)
	}
	if found != nil {
		t.Error("expected nil after cascade delete, key still exists")
	}
}

func TestClientRepo_MultipleKeys(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	repo := repository.NewClientRepository(testDB)

	client, _ := repo.Create(ctx, "Multi-key Client")

	hashes := []string{"hash-one", "hash-two", "hash-three"}
	for _, h := range hashes {
		if _, err := repo.CreateAPIKey(ctx, client.ID, h); err != nil {
			t.Fatalf("CreateAPIKey %q: %v", h, err)
		}
	}

	for _, h := range hashes {
		found, err := repo.FindByKeyHash(ctx, h)
		if err != nil {
			t.Fatalf("FindByKeyHash %q: %v", h, err)
		}
		if found == nil || found.ID != client.ID {
			t.Errorf("hash %q: expected client %q, got %+v", h, client.ID, found)
		}
	}
}
