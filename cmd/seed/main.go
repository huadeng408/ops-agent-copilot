package main

import (
	"context"
	"log"

	"ops-agent-copilot/internal/app"
)

func main() {
	cfg, err := app.LoadConfig()
	if err != nil {
		log.Fatalf("load config failed: %v", err)
	}
	db, dialect, err := app.OpenDatabase(cfg)
	if err != nil {
		log.Fatalf("open database failed: %v", err)
	}
	defer db.Close()

	if err := app.SeedDemoData(context.Background(), db, dialect); err != nil {
		log.Fatalf("seed demo data failed: %v", err)
	}
	log.Println("Go demo data initialized successfully.")
}
