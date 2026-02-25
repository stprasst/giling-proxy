package main

import (
	"fmt"
	"log"
	"os"

	"proxy-checker/internal/config"
	"proxy-checker/internal/database"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Starting Proxy Checker...")
	log.Printf("Port: %s, DB: %s, Workers: %d", cfg.Port, cfg.DBPath, cfg.WorkerCount)

	// Initialize database
	db, err := database.NewDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	log.Printf("Database initialized with WAL mode")

	// Run migrations
	if err := database.RunMigrations(db); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	log.Printf("Migrations completed")

	// Ensure data/public directory exists for proxy_alive.txt export
	if err := os.MkdirAll("data/public", 0755); err != nil {
		log.Fatalf("Failed to create public directory: %v", err)
	}

	// TODO: Phase 3 - Initialize Gin server, handlers, scheduler
	// For now, just verify setup works
	fmt.Println("\n✓ Phase 1 Complete: Database setup verified")
	fmt.Println("  - Database: data/proxy.db")
	fmt.Println("  - Tables: migrations, users, proxies, sources, check_logs")
	fmt.Println("  - WAL mode: enabled")
	fmt.Println("\nNext: Run 'go mod tidy' then 'go build' to verify compilation")
}
