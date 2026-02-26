package services

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"proxy-checker/internal/database"
)

// Exporter handles exporting alive proxies to files
type Exporter struct {
	db       *sql.DB
	basePath string
}

// NewExporter creates a new exporter
func NewExporter(db *sql.DB, basePath string) *Exporter {
	return &Exporter{
		db:       db,
		basePath: basePath,
	}
}

// ExportAlive exports alive proxies to files
// Creates two files:
// - proxy_alive.txt: simple format (ip:port)
// - proxy_alive_full.txt: full format (ip:port:type latency)
func (e *Exporter) ExportAlive() error {
	proxies, err := e.getAliveProxies()
	if err != nil {
		return fmt.Errorf("failed to get alive proxies: %w", err)
	}

	// Ensure directory exists
	publicDir := filepath.Join(e.basePath, "public")
	if err := os.MkdirAll(publicDir, 0755); err != nil {
		return fmt.Errorf("failed to create public directory: %w", err)
	}

	// Export simple format (ip:port)
	if err := e.exportSimple(proxies, publicDir); err != nil {
		return err
	}

	// Export full format (ip:port:type latency)
	if err := e.exportFull(proxies, publicDir); err != nil {
		return err
	}

	return nil
}

// getAliveProxies retrieves all alive proxies sorted by latency
func (e *Exporter) getAliveProxies() ([]database.Proxy, error) {
	query := `
		SELECT id, address, type, status, latency, country, source_type, source_url,
		       check_count, fail_count, last_checked, created_at, updated_at
		FROM proxies
		WHERE status = ?
		ORDER BY latency ASC
	`

	rows, err := e.db.Query(query, database.StatusAlive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var proxies []database.Proxy
	for rows.Next() {
		var p database.Proxy
		var lastChecked sql.NullTime
		err := rows.Scan(
			&p.ID, &p.Address, &p.Type, &p.Status, &p.Latency, &p.Country,
			&p.SourceType, &p.SourceURL, &p.CheckCount, &p.FailCount,
			&lastChecked, &p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		if lastChecked.Valid {
			p.LastChecked = &lastChecked.Time
		}
		proxies = append(proxies, p)
	}

	return proxies, rows.Err()
}

// exportSimple creates proxy_alive.txt with simple format
func (e *Exporter) exportSimple(proxies []database.Proxy, dir string) error {
	path := filepath.Join(dir, "proxy_alive.txt")

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", path, err)
	}
	defer file.Close()

	// Add header comment
	fmt.Fprintf(file, "# Proxy Alive List\n")
	fmt.Fprintf(file, "# Generated: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(file, "# Total: %d proxies\n\n", len(proxies))

	for _, p := range proxies {
		fmt.Fprintln(file, p.Address)
	}

	return nil
}

// exportFull creates proxy_alive_full.txt with type and latency info
func (e *Exporter) exportFull(proxies []database.Proxy, dir string) error {
	path := filepath.Join(dir, "proxy_alive_full.txt")

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", path, err)
	}
	defer file.Close()

	// Add header comment
	fmt.Fprintf(file, "# Proxy Alive List (Full Format)\n")
	fmt.Fprintf(file, "# Format: ip:port:type latency_ms\n")
	fmt.Fprintf(file, "# Generated: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(file, "# Total: %d proxies\n\n", len(proxies))

	// Group by type for better organization
	byType := make(map[string][]database.Proxy)
	for _, p := range proxies {
		byType[p.Type] = append(byType[p.Type], p)
	}

	// Sort types for consistent output
	types := make([]string, 0, len(byType))
	for t := range byType {
		types = append(types, t)
	}
	sort.Strings(types)

	for _, t := range types {
		fmt.Fprintf(file, "# %s proxies (%d)\n", t, len(byType[t]))
		for _, p := range byType[t] {
			fmt.Fprintf(file, "%s:%s %d\n", p.Address, p.Type, p.Latency)
		}
		fmt.Fprintln(file)
	}

	return nil
}

// GetExportPath returns the path to the simple export file
func (e *Exporter) GetExportPath() string {
	return filepath.Join(e.basePath, "public", "proxy_alive.txt")
}

// GetFullExportPath returns the path to the full export file
func (e *Exporter) GetFullExportPath() string {
	return filepath.Join(e.basePath, "public", "proxy_alive_full.txt")
}

// ExportAll exports all proxies to files including type-specific files
func (e *Exporter) ExportAll() error {
	proxies, err := e.getAliveProxies()
	if err != nil {
		return fmt.Errorf("failed to get alive proxies: %w", err)
	}

	// Ensure directory exists
	publicDir := filepath.Join(e.basePath, "public")
	if err := os.MkdirAll(publicDir, 0755); err != nil {
		return fmt.Errorf("failed to create public directory: %w", err)
	}

	// Export simple format
	if err := e.exportSimple(proxies, publicDir); err != nil {
		return err
	}

	// Export full format
	if err := e.exportFull(proxies, publicDir); err != nil {
		return err
	}

	// Export by type
	types := []string{"http", "https", "socks4", "socks5"}
	for _, t := range types {
		if err := e.exportByType(proxies, t, publicDir); err != nil {
			return err
		}
	}

	return nil
}

// exportByType creates proxy_{type}.txt for a specific proxy type
func (e *Exporter) exportByType(proxies []database.Proxy, proxyType string, dir string) error {
	// Filter proxies by type
	var filtered []database.Proxy
	for _, p := range proxies {
		if p.Type == proxyType {
			filtered = append(filtered, p)
		}
	}

	path := filepath.Join(dir, fmt.Sprintf("proxy_%s.txt", proxyType))

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", path, err)
	}
	defer file.Close()

	// Add header comment
	fmt.Fprintf(file, "# Proxy %s List\n", proxyType)
	fmt.Fprintf(file, "# Generated: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(file, "# Total: %d proxies\n\n", len(filtered))

	for _, p := range filtered {
		fmt.Fprintln(file, p.Address)
	}

	return nil
}

// GetTypeExportPath returns the path to a type-specific export file
func (e *Exporter) GetTypeExportPath(proxyType string) string {
	return filepath.Join(e.basePath, "public", fmt.Sprintf("proxy_%s.txt", proxyType))
}
