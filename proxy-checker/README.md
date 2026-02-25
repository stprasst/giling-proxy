# Proxy Checker

24/7 proxy scraper and checker web application built with Go + Gin.

## Features

- **Scrape proxies** from URLs (GitHub, pastebin, etc.)
- **Auto-detect protocol**: HTTP, HTTPS, SOCKS4, SOCKS5
- **Concurrent checking**: 100 workers with rate limiting
- **Scheduled checks**: Auto-refresh every 20 minutes
- **Deduplication**: SQLite with UNIQUE constraint
- **Export**: Public `proxy_alive.txt` file for other apps
- **Web dashboard**: Modern UI with Tailwind CSS

## Quick Start

```bash
# 1. Copy config
cp .env.example .env

# 2. Edit .env (IMPORTANT: Change SESSION_SECRET and ADMIN_PASSWORD)
nano .env

# 3. Run
go run main.go

# 4. Open http://localhost:8080
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| PORT | 8080 | Server port |
| DB_PATH | data/proxy.db | SQLite database path |
| SESSION_SECRET | (required) | Session secret (32+ chars) |
| ADMIN_PASSWORD | (required) | Admin password (12+ chars) |
| CHECK_INTERVAL | 20m | Auto-check interval |
| WORKER_COUNT | 100 | Concurrent workers |
| CHECK_TIMEOUT | 10s | Proxy check timeout |
| TEST_URLS | httpbin.org,... | Fallback test URLs |

## API Endpoints

### Public
- `GET /public/proxy_alive.txt` - Download alive proxies

### Auth
- `GET /login` - Login page
- `POST /login` - Login
- `GET /logout` - Logout

### Admin (requires auth)
- `GET /admin/dashboard` - Dashboard page

### API (requires auth)
- `POST /api/proxies/bulk` - Bulk add proxies
- `POST /api/proxies/fetch` - Fetch from URL
- `GET /api/proxies` - List proxies (paginated)
- `DELETE /api/proxies/:id` - Delete proxy
- `POST /api/export` - Trigger export
- `POST /api/sources` - Add source URL
- `GET /api/sources` - List sources
- `DELETE /api/sources/:id` - Delete source
- `POST /api/sources/:id/refresh` - Refresh source
- `GET /api/stats` - Get statistics
- `GET /api/logs` - Get check logs
- `POST /api/check/trigger` - Trigger manual check

## Project Structure

```
proxy-checker/
├── main.go                    # Entry point
├── .env                       # Config (gitignored)
├── .env.example               # Config template
├── go.mod                     # Dependencies
├── data/
│   ├── proxy.db               # SQLite database
│   └── public/
│       ├── proxy_alive.txt    # Simple export
│       └── proxy_alive_full.txt # Full export
├── internal/
│   ├── config/                # Config loader
│   ├── database/              # DB, models, migrations
│   ├── handlers/              # HTTP handlers
│   ├── middleware/            # Auth middleware
│   ├── services/              # Business logic
│   └── scheduler/             # Cron jobs
├── templates/                 # HTML templates
└── static/css/                # Stylesheets
```

## Database Schema

- **users**: Admin authentication
- **proxies**: Proxy entries with status tracking
- **sources**: URL sources for scraping
- **check_logs**: Check history

## Building

```bash
# Development
go run main.go

# Production build
go build -o proxy-checker main.go
./proxy-checker
```

## Requirements

- Go 1.21+
- GCC (for SQLite CGO)

## License

MIT
