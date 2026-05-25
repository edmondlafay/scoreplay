package service_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/edmondlafaydavid/scoreplay/internal/model"
	"github.com/edmondlafaydavid/scoreplay/internal/service"
)

type fakeClientRepo struct {
	clients []model.Client
	keys    []struct {
		id       string
		clientID string
		keyHash  string
	}
	nextID int
}

func (f *fakeClientRepo) Create(_ context.Context, name string) (*model.Client, error) {
	f.nextID++
	c := model.Client{ID: fmt.Sprintf("client-%d", f.nextID), Name: name, CreatedAt: time.Now()}
	f.clients = append(f.clients, c)
	return &f.clients[len(f.clients)-1], nil
}

func (f *fakeClientRepo) CreateAPIKey(_ context.Context, clientID, keyHash string) (*model.APIKey, error) {
	f.nextID++
	k := struct {
		id       string
		clientID string
		keyHash  string
	}{fmt.Sprintf("key-%d", f.nextID), clientID, keyHash}
	f.keys = append(f.keys, k)
	return &model.APIKey{ID: k.id, ClientID: k.clientID, CreatedAt: time.Now()}, nil
}

func (f *fakeClientRepo) FindByKeyHash(_ context.Context, keyHash string) (*model.Client, error) {
	for _, k := range f.keys {
		if k.keyHash == keyHash {
			for _, c := range f.clients {
				if c.ID == k.clientID {
					return &c, nil
				}
			}
		}
	}
	return nil, nil
}

func TestClientService_Create(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{"valid name", "Acme Corp", nil},
		{"trims whitespace", "  Acme  ", nil},
		{"empty name", "", service.ErrClientNameEmpty},
		{"whitespace only", "   ", service.ErrClientNameEmpty},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := service.NewClientService(&fakeClientRepo{})
			result, err := svc.Create(context.Background(), tc.input)

			if tc.wantErr != nil {
				if err != tc.wantErr {
					t.Fatalf("want error %v, got %v", tc.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Client.ID == "" {
				t.Error("client ID is empty")
			}
			if result.APIKey.ID == "" {
				t.Error("api key ID is empty")
			}
			if result.RawKey == "" {
				t.Error("raw key is empty")
			}
			if len(result.RawKey) < 67 {
				t.Errorf("raw key too short: %q", result.RawKey)
			}
		})
	}
}

func TestClientService_FindByRawKey(t *testing.T) {
	svc := service.NewClientService(&fakeClientRepo{})
	ctx := context.Background()

	result, err := svc.Create(ctx, "Acme Corp")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	t.Run("valid key", func(t *testing.T) {
		client, err := svc.FindByRawKey(ctx, result.RawKey)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if client == nil {
			t.Fatal("expected client, got nil")
		}
		if client.ID != result.Client.ID {
			t.Errorf("want client %s, got %s", result.Client.ID, client.ID)
		}
	})

	t.Run("unknown key", func(t *testing.T) {
		client, err := svc.FindByRawKey(ctx, "sp_unknown")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if client != nil {
			t.Error("expected nil for unknown key")
		}
	})
}

func TestClientService_AddKey(t *testing.T) {
	svc := service.NewClientService(&fakeClientRepo{})
	ctx := context.Background()

	result, _ := svc.Create(ctx, "Acme Corp") //nolint:errcheck

	apiKey, rawKey, err := svc.AddKey(ctx, result.Client.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if apiKey.ID == "" {
		t.Error("api key ID is empty")
	}
	if rawKey == result.RawKey {
		t.Error("new key should differ from first key")
	}

	// Both keys should resolve to the same client
	c1, _ := svc.FindByRawKey(ctx, result.RawKey)   //nolint:errcheck
	c2, _ := svc.FindByRawKey(ctx, rawKey)            //nolint:errcheck
	if c1 == nil || c2 == nil {
		t.Fatal("both keys should resolve to a client")
	}
	if c1.ID != c2.ID {
		t.Errorf("keys resolve to different clients: %s vs %s", c1.ID, c2.ID)
	}
}
