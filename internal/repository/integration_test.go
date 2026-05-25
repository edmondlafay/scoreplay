//go:build integration

package repository_test

import (
	"context"
	"database/sql"
	"log"
	"os"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	_ "github.com/lib/pq"

	"github.com/edmondlafaydavid/scoreplay/internal/database"
)

// testDB is shared across all integration tests in this package.
var testDB *sql.DB

func TestMain(m *testing.M) {
	ctx := context.Background()

	pgc, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("scoreplay_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		log.Fatalf("start postgres container: %v", err)
	}
	defer func() {
		if err := testcontainers.TerminateContainer(pgc); err != nil {
			log.Printf("terminate container: %v", err)
		}
	}()

	connStr, err := pgc.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Fatalf("get connection string: %v", err)
	}

	// Use a separate connection for migrations: golang-migrate's m.Close() closes
	// the underlying *sql.DB, which would break the shared testDB used by tests.
	migDB, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("open migration db: %v", err)
	}
	if err := database.RunMigrations(migDB); err != nil {
		log.Fatalf("run migrations: %v", err)
	}
	migDB.Close()

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("open test db: %v", err)
	}
	defer db.Close()

	testDB = db
	os.Exit(m.Run())
}

// truncate resets all tables to empty state between tests.
func truncate(t *testing.T) {
	t.Helper()
	_, err := testDB.Exec(`TRUNCATE api_keys, media_tags, media, tags, clients`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}
