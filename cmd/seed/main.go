package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"

	_ "github.com/lib/pq"

	"github.com/edmondlafaydavid/scoreplay/internal/config"
	"github.com/edmondlafaydavid/scoreplay/internal/repository"
	"github.com/edmondlafaydavid/scoreplay/internal/service"
)

func main() {
	name := flag.String("name", "", "client name (required)")
	flag.Parse()

	if *name == "" {
		fmt.Fprintln(os.Stderr, "usage: seed -name <client-name>")
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.PingContext(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "ping db: %v\n", err)
		os.Exit(1)
	}

	svc := service.NewClientService(repository.NewClientRepository(db))

	result, err := svc.Create(context.Background(), *name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create client: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("client_id : %s\n", result.Client.ID)
	fmt.Printf("key       : %s\n", result.RawKey)
}
