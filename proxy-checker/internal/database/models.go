package database

import "time"

// User represents admin user for authentication
type User struct {
	ID           int64     `json:"id"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

// Proxy represents a proxy server entry
type Proxy struct {
	ID          int64      `json:"id"`
	Address     string     `json:"address"`     // ip:port (UNIQUE)
	Type        string     `json:"type"`        // http, https, socks4, socks5
	Status      string     `json:"status"`      // alive, dead, unchecked
	Latency     int        `json:"latency"`     // response time in ms
	Country     string     `json:"country"`     // geoip country code (optional)
	SourceType  string     `json:"source_type"` // manual, url
	SourceURL   string     `json:"source_url"`  // URL where proxy was found
	CheckCount  int        `json:"check_count"` // number of times checked
	FailCount   int        `json:"fail_count"`  // number of failed checks
	LastChecked *time.Time `json:"last_checked"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Source represents a URL source for proxy scraping
type Source struct {
	ID             int64      `json:"id"`
	URL            string     `json:"url"` // URL to scrape for proxies (UNIQUE)
	AutoRefresh    bool       `json:"auto_refresh"`
	IntervalMin    int        `json:"interval_minutes"` // refresh interval in minutes
	LastFetched    *time.Time `json:"last_fetched"`
	LastStatus     string     `json:"last_status"` // success, error
	ProxyCount     int        `json:"proxy_count"` // proxies found last fetch
	CreatedAt      time.Time  `json:"created_at"`
}

// CheckLog represents a proxy check history entry
type CheckLog struct {
	ID        int64     `json:"id"`
	ProxyID   int64     `json:"proxy_id"`
	Status    string    `json:"status"` // alive, dead
	Latency   int       `json:"latency"`
	ErrorMsg  string    `json:"error_msg"`
	CheckedAt time.Time `json:"checked_at"`
}

// ProxyStatus constants
const (
	StatusAlive     = "alive"
	StatusDead      = "dead"
	StatusUnchecked = "unchecked"
	StatusChecking  = "checking"
)

// ProxyType constants
const (
	TypeHTTP   = "http"
	TypeHTTPS  = "https"
	TypeSOCKS4 = "socks4"
	TypeSOCKS5 = "socks5"
	TypeUnknown = "unknown"
)

// SourceType constants
const (
	SourceTypeManual = "manual"
	SourceTypeURL    = "url"
)
