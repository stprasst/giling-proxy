package database

import (
	"database/sql"
	"fmt"
	"log"
)

// Migration represents a database migration
type Migration struct {
	Name string
	SQL  string
}

// migrations is the ordered list of migrations to run
var migrations = []Migration{
	{
		Name: "001_create_migrations_table",
		SQL: `
		CREATE TABLE IF NOT EXISTS migrations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
	},
	{
		Name: "002_create_users_table",
		SQL: `
		CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			password_hash TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
	},
	{
		Name: "003_create_proxies_table",
		SQL: `
		CREATE TABLE IF NOT EXISTS proxies (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			address TEXT NOT NULL UNIQUE,
			type TEXT DEFAULT 'unknown',
			status TEXT DEFAULT 'unchecked',
			latency INTEGER DEFAULT 0,
			country TEXT DEFAULT '',
			source_type TEXT DEFAULT 'manual',
			source_url TEXT DEFAULT '',
			check_count INTEGER DEFAULT 0,
			fail_count INTEGER DEFAULT 0,
			last_checked TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_proxies_status ON proxies(status);
		CREATE INDEX IF NOT EXISTS idx_proxies_type ON proxies(type);`,
	},
	{
		Name: "004_create_sources_table",
		SQL: `
		CREATE TABLE IF NOT EXISTS sources (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			url TEXT NOT NULL UNIQUE,
			auto_refresh INTEGER DEFAULT 1,
			interval_minutes INTEGER DEFAULT 20,
			last_fetched TIMESTAMP,
			last_status TEXT DEFAULT '',
			proxy_count INTEGER DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
	},
	{
		Name: "005_create_check_logs_table",
		SQL: `
		CREATE TABLE IF NOT EXISTS check_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			proxy_id INTEGER NOT NULL,
			status TEXT NOT NULL,
			latency INTEGER DEFAULT 0,
			error_msg TEXT DEFAULT '',
			checked_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (proxy_id) REFERENCES proxies(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_check_logs_proxy_id ON check_logs(proxy_id);
		CREATE INDEX IF NOT EXISTS idx_check_logs_checked_at ON check_logs(checked_at);`,
	},
	{
		Name: "006_create_settings_table",
		SQL: `
		CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			description TEXT DEFAULT '',
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		INSERT OR IGNORE INTO settings (key, value, description) VALUES
			('check_interval', '15m', 'Re-check alive proxies interval (e.g. 15m, 20m)'),
			('scrape_interval', '60m', 'Scrape sources + check all proxies interval (e.g. 60m, 1h)'),
			('worker_count', '100', 'Number of concurrent workers'),
			('check_timeout', '10s', 'Timeout for each proxy check'),
			('test_urls', 'httpbin.org/ip,api.ipify.org,cloudflare.com/cdn-cgi/trace', 'Test URLs for proxy checking (comma separated)');
		`,
	},
	{
		Name: "007_update_check_interval_default",
		SQL: `
		-- Update check_interval from 20m to 15m for existing installations
		-- Only update if it's still the old default (user hasn't changed it)
		UPDATE settings SET value = '15m', description = 'Re-check alive proxies interval (e.g. 15m, 20m)'
		WHERE key = 'check_interval' AND value = '20m';
		-- Add scrape_interval if it doesn't exist (for databases created before this migration)
		INSERT OR IGNORE INTO settings (key, value, description) VALUES
			('scrape_interval', '60m', 'Scrape sources + check all proxies interval (e.g. 60m, 1h)');
		`,
	},
	{
		Name: "008_add_socks_protocol_settings",
		SQL: `
			-- Add SOCKS4/SOCKS5 protocol check settings
			INSERT OR IGNORE INTO settings (key, value, description) VALUES
				('check_socks4', 'true', 'Enable SOCKS4 protocol checking'),
				('check_socks5', 'true', 'Enable SOCKS5 protocol checking');
		`,
	},
}

// RunMigrations executes all pending migrations
func RunMigrations(db *sql.DB) error {
	// First, ensure migrations table exists
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS migrations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
	); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	for _, m := range migrations {
		// Skip the migrations table creation since we already did it
		if m.Name == "001_create_migrations_table" {
			continue
		}

		applied, err := isMigrationApplied(db, m.Name)
		if err != nil {
			return fmt.Errorf("failed to check migration %s: %w", m.Name, err)
		}

		if applied {
			log.Printf("Migration %s already applied, skipping", m.Name)
			continue
		}

		log.Printf("Running migration: %s", m.Name)

		// Start transaction
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("failed to start transaction for %s: %w", m.Name, err)
		}

		// Execute migration SQL
		if _, err := tx.Exec(m.SQL); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to execute migration %s: %w", m.Name, err)
		}

		// Record migration
		if _, err := tx.Exec(
			"INSERT INTO migrations (name) VALUES (?)",
			m.Name,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record migration %s: %w", m.Name, err)
		}

		// Commit transaction
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %s: %w", m.Name, err)
		}

		log.Printf("Migration %s completed successfully", m.Name)
	}

	return nil
}

// isMigrationApplied checks if a migration has been applied
func isMigrationApplied(db *sql.DB, name string) (bool, error) {
	var count int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM migrations WHERE name = ?",
		name,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
