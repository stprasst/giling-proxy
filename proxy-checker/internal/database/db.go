package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

// NewDB creates a new database connection with SQLite optimizations
func NewDB(dbPath string) (*sql.DB, error) {
	// Ensure data directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// Open database connection
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// SQLite optimizations
	pragmas := []string{
		"PRAGMA journal_mode=WAL",      // Write-Ahead Logging for better concurrency
		"PRAGMA foreign_keys=ON",       // Enable foreign key constraints
		"PRAGMA cache_size=10000",      // Increase cache size
		"PRAGMA synchronous=NORMAL",    // Faster writes with acceptable safety
		"PRAGMA temp_store=MEMORY",     // Store temp tables in memory
		"PRAGMA mmap_size=268435456",   // 256MB memory-mapped I/O
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to set pragma %s: %w", pragma, err)
		}
	}

	// Single connection for SQLite write safety
	// Multiple readers are fine with WAL mode
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	// Verify connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}
