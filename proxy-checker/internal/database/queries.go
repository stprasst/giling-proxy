package database

import (
	"database/sql"
	"fmt"
	"time"
)

// InsertProxiesBatch inserts multiple proxies, ignoring duplicates
func InsertProxiesBatch(db *sql.DB, proxies []Proxy, sourceType, sourceURL string) (int, error) {
	if len(proxies) == 0 {
		return 0, nil
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO proxies (address, source_type, source_url, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return 0, fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	now := time.Now()
	inserted := 0

	for _, p := range proxies {
		result, err := stmt.Exec(p.Address, sourceType, sourceURL, StatusUnchecked, now, now)
		if err != nil {
			continue // Skip errors (duplicates handled by INSERT OR IGNORE)
		}

		if rows, _ := result.RowsAffected(); rows > 0 {
			inserted++
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return inserted, nil
}

// GetProxies retrieves proxies with pagination
func GetProxies(db *sql.DB, page, limit int, status string) ([]Proxy, int, error) {
	offset := (page - 1) * limit
	if offset < 0 {
		offset = 0
	}

	// Count query
	countQuery := "SELECT COUNT(*) FROM proxies"
	args := []interface{}{}
	if status != "" {
		countQuery += " WHERE status = ?"
		args = append(args, status)
	}

	var total int
	if err := db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Data query
	query := `
		SELECT id, address, type, status, latency, country, source_type, source_url,
		       check_count, fail_count, last_checked, created_at, updated_at
		FROM proxies
	`
	queryArgs := []interface{}{}

	if status != "" {
		query += " WHERE status = ?"
		queryArgs = append(queryArgs, status)
	}

	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	queryArgs = append(queryArgs, limit, offset)

	rows, err := db.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var proxies []Proxy
	for rows.Next() {
		var p Proxy
		var lastChecked sql.NullTime
		err := rows.Scan(
			&p.ID, &p.Address, &p.Type, &p.Status, &p.Latency, &p.Country,
			&p.SourceType, &p.SourceURL, &p.CheckCount, &p.FailCount,
			&lastChecked, &p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		if lastChecked.Valid {
			p.LastChecked = &lastChecked.Time
		}
		proxies = append(proxies, p)
	}

	return proxies, total, rows.Err()
}

// GetUncheckedProxies retrieves all unchecked proxies
func GetUncheckedProxies(db *sql.DB) ([]Proxy, error) {
	query := `
		SELECT id, address, type, status, latency, country, source_type, source_url,
		       check_count, fail_count, last_checked, created_at, updated_at
		FROM proxies
		WHERE status = ? OR status = ?
	`

	rows, err := db.Query(query, StatusUnchecked, StatusDead)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var proxies []Proxy
	for rows.Next() {
		var p Proxy
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

// UpdateProxyStatus updates a proxy's status and related fields
func UpdateProxyStatus(db *sql.DB, id int64, status, proxyType string, latency int) error {
	now := time.Now()

	// Calculate fail count increment
	failCount := 0
	if status != StatusAlive {
		failCount = 1
	}

	query := `
		UPDATE proxies SET
			status = ?,
			type = ?,
			latency = ?,
			last_checked = ?,
			check_count = check_count + 1,
			fail_count = fail_count + ?,
			updated_at = ?
		WHERE id = ?
	`

	_, err := db.Exec(query, status, proxyType, latency, now, failCount, now, id)
	return err
}

// DeleteProxy deletes a proxy by ID
func DeleteProxy(db *sql.DB, id int64) error {
	_, err := db.Exec("DELETE FROM proxies WHERE id = ?", id)
	return err
}

// GetStats returns proxy statistics
func GetStats(db *sql.DB) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Count by status
	rows, err := db.Query("SELECT status, COUNT(*) FROM proxies GROUP BY status")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	total := 0
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		stats[status] = count
		total += count
	}
	stats["total"] = total

	// Average latency of alive proxies
	var avgLatency sql.NullFloat64
	err = db.QueryRow("SELECT AVG(latency) FROM proxies WHERE status = ?", StatusAlive).Scan(&avgLatency)
	if err == nil && avgLatency.Valid {
		stats["avg_latency"] = int(avgLatency.Float64)
	}

	// Last check time
	var lastCheck sql.NullTime
	err = db.QueryRow("SELECT MAX(last_checked) FROM proxies").Scan(&lastCheck)
	if err == nil && lastCheck.Valid {
		stats["last_check"] = lastCheck.Time
	}

	return stats, nil
}

// InsertCheckLog adds a check log entry
func InsertCheckLog(db *sql.DB, log CheckLog) error {
	_, err := db.Exec(`
		INSERT INTO check_logs (proxy_id, status, latency, error_msg, checked_at)
		VALUES (?, ?, ?, ?, ?)
	`, log.ProxyID, log.Status, log.Latency, log.ErrorMsg, time.Now())
	return err
}

// GetCheckLogs retrieves check logs with pagination
func GetCheckLogs(db *sql.DB, proxyID int64, limit int) ([]CheckLog, error) {
	query := `
		SELECT id, proxy_id, status, latency, error_msg, checked_at
		FROM check_logs
	`
	args := []interface{}{}

	if proxyID > 0 {
		query += " WHERE proxy_id = ?"
		args = append(args, proxyID)
	}

	query += " ORDER BY checked_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []CheckLog
	for rows.Next() {
		var l CheckLog
		if err := rows.Scan(&l.ID, &l.ProxyID, &l.Status, &l.Latency, &l.ErrorMsg, &l.CheckedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}

	return logs, nil
}

// GetUser retrieves the admin user
func GetUser(db *sql.DB) (*User, error) {
	var u User
	err := db.QueryRow("SELECT id, password_hash, created_at FROM users WHERE id = 1").Scan(&u.ID, &u.PasswordHash, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// CreateUser creates the admin user
func CreateUser(db *sql.DB, passwordHash string) error {
	_, err := db.Exec("INSERT OR IGNORE INTO users (id, password_hash) VALUES (1, ?)", passwordHash)
	return err
}

// UpdateUserPassword updates the admin password
func UpdateUserPassword(db *sql.DB, passwordHash string) error {
	_, err := db.Exec("UPDATE users SET password_hash = ? WHERE id = 1", passwordHash)
	return err
}

// GetSources retrieves all sources
func GetSources(db *sql.DB) ([]Source, error) {
	query := `
		SELECT id, url, auto_refresh, interval_minutes, last_fetched, last_status, proxy_count, created_at
		FROM sources
		ORDER BY created_at DESC
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sources []Source
	for rows.Next() {
		var s Source
		var lastFetched sql.NullTime
		err := rows.Scan(
			&s.ID, &s.URL, &s.AutoRefresh, &s.IntervalMin,
			&lastFetched, &s.LastStatus, &s.ProxyCount, &s.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		if lastFetched.Valid {
			s.LastFetched = &lastFetched.Time
		}
		sources = append(sources, s)
	}

	return sources, nil
}

// InsertSource adds a new source
func InsertSource(db *sql.DB, url string, autoRefresh bool, intervalMin int) (int64, error) {
	result, err := db.Exec(
		"INSERT OR IGNORE INTO sources (url, auto_refresh, interval_minutes) VALUES (?, ?, ?)",
		url, autoRefresh, intervalMin,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// UpdateSourceStatus updates source fetch status
func UpdateSourceStatus(db *sql.DB, id int64, status string, proxyCount int) error {
	_, err := db.Exec(
		"UPDATE sources SET last_fetched = ?, last_status = ?, proxy_count = ? WHERE id = ?",
		time.Now(), status, proxyCount, id,
	)
	return err
}

// DeleteSource removes a source
func DeleteSource(db *sql.DB, id int64) error {
	_, err := db.Exec("DELETE FROM sources WHERE id = ?", id)
	return err
}
