# giling-proxy

24/7 Proxy Scraper & Checker Web Application - Automatically scrapes, checks, and exports working proxies from multiple sources.

## Features

- **Dual-Schedule Architecture**
  - Every 15 min: Re-check alive proxies
  - Every 60 min: Scrape sources + check all proxies (alive + new)
- **Automatic Dead Proxy Removal** - Delete failed proxies after each check
- **Multi-Protocol Support** - HTTP, HTTPS, SOCKS4, SOCKS5
- **Export by Type** - Separate export files for each proxy type
- **Settings Management** - Configure intervals, workers, timeout via web UI
- **Progress Tracking** - Real-time progress indicator during checks

## Tech Stack

- **Backend:** Go 1.21+ with Gin framework
- **Database:** SQLite with WAL mode
- **Frontend:** HTML templates with Tailwind CSS
- **Scheduler:** robfig/cron v3

## Quick Start (Windows)

### Prerequisites

- Go 1.21+ installed
- Or use the pre-built `proxy-checker.exe`

### Option 1: Using Pre-built Binary

```powershell
# Extract and navigate to the directory
cd proxy-checker

# Create .env file
copy .env.example .env

# Edit .env and update:
# - ADMIN_PASSWORD (minimum 12 characters)
# - SESSION_SECRET (minimum 32 characters)

# Run the application
.\proxy-checker.exe

# Access at http://localhost:8080/admin
```

### Option 2: Build from Source

```powershell
# Install dependencies
go mod download

# Build
go build -o proxy-checker.exe

# Run
.\proxy-checker.exe
```

## Quick Start (Linux)

### Option 1: Using Deployment Script (Recommended)

```bash
# Clone or download source
git clone <your-repo-url>
cd giling-proxy/proxy-checker

# Run deployment script
sudo bash deploy/install.sh

# Edit configuration
sudo nano /var/www/proxy-checker/.env

# Restart service
sudo systemctl restart proxy-checker

# Check status
sudo systemctl status proxy-checker
```

### Option 2: Manual Build

```bash
# Install Go 1.21+
wget https://go.dev/dl/go1.21.6.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.6.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc

# Clone and build
git clone <your-repo-url>
cd giling-proxy/proxy-checker
go mod download
go build -o proxy-checker main.go

# Create .env
cp .env.example .env
nano .env  # Update ADMIN_PASSWORD and SESSION_SECRET

# Run
./proxy-checker
```

### Systemd Service

```bash
# Create service file
sudo nano /etc/systemd/system/proxy-checker.service
```

```ini
[Unit]
Description=Proxy Checker Service
After=network.target

[Service]
Type=simple
User=www-data
WorkingDirectory=/var/www/proxy-checker
ExecStart=/var/www/proxy-checker/proxy-checker
Restart=always
RestartSec=10
Environment="PORT=8080"

[Install]
WantedBy=multi-user.target
```

```bash
# Enable and start
sudo systemctl daemon-reload
sudo systemctl enable proxy-checker
sudo systemctl start proxy-checker
```

## RunCloud Deployment

### Prerequisites

- RunCloud Cloud Panel (Ubuntu server)
- SSH access to your server

### Step 1: Create Web App

1. Go to RunCloud Panel → Your Server → Web App
2. Click "Create Web App"
3. Configure:
   - **Name:** `proxy-checker`
   - **Domain:** Your domain (or use server IP)
   - **User:** `runcloud` (or create new user)
   - **PHP Version:** Non-PHP (Static)
   - **Websocket Support:** Enable

### Step 2: Deploy Application

```bash
# SSH to your server
ssh runcloud@your-server-ip

# Navigate to app directory
cd /home/runcloud/webapps/proxy-checker

# Upload source (option 1: git clone)
git clone <your-repo-url> temp
mv temp/* temp/.* . 2>/dev/null || true
rm -rf temp

# Upload source (option 2: manual upload)
# Upload via SCP/SFTP then extract

# Install Go (if not installed)
wget -q https://go.dev/dl/go1.21.6.linux-amd64.tar.gz
tar -C /usr/local -xzf go1.21.6.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# Build application
go mod download
go build -o proxy-checker main.go

# Create data directory
mkdir -p data/public

# Create .env file
cat > .env << 'EOF'
PORT=8080
DB_PATH=data/proxy.db
WORKER_COUNT=100
CHECK_INTERVAL=15m
SCRAPE_INTERVAL=60m
CHECK_TIMEOUT=10s
ADMIN_PASSWORD=your-secure-password-here
SESSION_SECRET=change-this-to-32-char-random-string
EOF

# Set permissions
chmod 600 .env
chmod +x proxy-checker
```

### Step 3: Setup Supervisor (RunCloud)

1. Go to RunCloud Panel → Your Server → Supervisor
2. Click "Create Supervisor"
3. Configure:
   - **Name:** `proxy-checker`
   - **User:** `runcloud`
   - **Command:** `/home/runcloud/webapps/proxy-checker/proxy-checker`
   - **Directory:** `/home/runcloud/webapps/proxy-checker`
   - **Autostart:** Enable
   - **Auto Restart:** Enable
4. Click "Create"

### Step 4: Setup Nginx Reverse Proxy (RunCloud)

1. Go to RunCloud Panel → Web App → proxy-checker → NGINX
2. Click "Create NGINX Config"
3. Configure:

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection 'upgrade';
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_cache_bypass $http_upgrade;
}
```

4. Save and reload NGINX

### Step 5: Access Application

```
URL: https://your-domain.com/admin
Default credentials (change .env first):
- Password: (your ADMIN_PASSWORD from .env)
```

## Configuration

### Environment Variables (.env)

| Variable | Default | Description |
|----------|---------|-------------|
| PORT | 8080 | Server port |
| DB_PATH | data/proxy.db | SQLite database path |
| SESSION_SECRET | - | Session secret (32+ chars) **REQUIRED** |
| ADMIN_PASSWORD | - | Admin password (12+ chars) **REQUIRED** |
| WORKER_COUNT | 100 | Concurrent workers (database setting override) |
| CHECK_INTERVAL | 15m | Re-check alive proxies interval |
| SCRAPE_INTERVAL | 60m | Scrape + check all proxies interval |
| CHECK_TIMEOUT | 10s | Per proxy check timeout |
| TEST_URLS | - | Test URLs (fallback) |

### Database Settings (via Web UI)

Access Settings tab to configure:
- **Check Interval (Alive Proxies):** How often to re-check alive proxies (default: 15m)
- **Scrape Interval:** How often to scrape + check all proxies (default: 60m)
- **Worker Count:** Number of concurrent workers (default: 100)
- **Check Timeout:** Timeout per proxy check (default: 10s)
- **Test URLs:** Comma-separated test URLs

> **Note:** Settings changes require application restart to take effect.

## Usage

### Adding Sources

1. Go to **Sources** tab
2. Add proxy list URLs (one per line)
3. Click **Add Sources**
4. Sources will be auto-scraped based on Scrape Interval

### Manual Actions

- **Refresh All Sources:** Scrape all sources immediately
- **Check All Proxies Now:** Run full scrape + check cycle
- **View Exports:** Access proxy lists by type

### Export Files

Located in `data/public/`:
- `proxy_alive.txt` - All alive proxies
- `proxy_http.txt` - HTTP proxies only
- `proxy_https.txt` - HTTPS proxies only
- `proxy_socks4.txt` - SOCKS4 proxies only
- `proxy_socks5.txt` - SOCKS5 proxies only
- `proxy_alive_full.txt` - Full export with details

Access via: `http://your-server:8080/public/proxy_alive.txt`

## Project Structure

```
proxy-checker/
├── main.go                 # Entry point
├── internal/
│   ├── config/            # Configuration loader
│   ├── database/          # Database layer (SQLite, migrations)
│   ├── handlers/          # HTTP handlers
│   │   ├── auth.go        # Authentication
│   │   ├── check.go       # Check endpoints
│   │   ├── proxy.go       # Proxy CRUD
│   │   ├── settings.go    # Settings management
│   │   └── source.go      # Source management
│   ├── scheduler/         # Cron scheduler (dual-schedule)
│   └── services/          # Business logic
│       ├── checker.go     # Proxy validation
│       ├── exporter.go    # File export
│       ├── scraper.go     # Source scraping
│       └── worker_pool.go # Concurrent checking
├── static/                # CSS assets
├── templates/             # HTML templates
├── deploy/                # Deployment scripts
│   ├── install.sh         # Linux deployment script
│   └── proxy-checker.service  # Systemd service
└── data/                  # Runtime data (database, exports)
```

## Troubleshooting

### Application won't start

```bash
# Check logs
tail -f server.log

# Verify .env exists and is valid
cat .env

# Check database permissions
ls -la data/proxy.db

# Verify port is available
netstat -an | grep 8080
```

### Database locked error

```bash
# Stop application
sudo systemctl stop proxy-checker

# Remove WAL files
rm data/proxy.db-shm data/proxy.db-wal

# Restart
sudo systemctl start proxy-checker
```

### Workers not processing

1. Check Settings tab → Worker Count (default: 100)
2. Restart application after changing settings
3. Check logs for "WorkerPool: Starting X workers"

### Settings not being applied

Settings are read **only at startup**. Restart the application after changing:
- Check Interval
- Scrape Interval
- Worker Count

## Development

### Running in Development Mode

```bash
# Install dependencies
go mod download

# Run with file watcher (install air first)
go install github.com/air-verse/air@latest
air

# Or run directly
go run main.go
```

### Building for Production

```bash
# Windows
set GOOS=windows
set GOARCH=amd64
go build -o proxy-checker.exe .

# Linux
GOOS=linux GOARCH=amd64 go build -o proxy-checker .

# With optimizations
go build -ldflags="-s -w" -o proxy-checker .
```

## License

MIT

## Credits

Built with Go, Gin, SQLite, and robfig/cron.
