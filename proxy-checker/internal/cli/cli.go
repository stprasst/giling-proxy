package cli

import (
	"bufio"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"proxy-checker/internal/config"
	"proxy-checker/internal/database"
	"proxy-checker/internal/scheduler"
	"proxy-checker/internal/services"
)

// Commands represents CLI command operations
type Commands struct {
	db            *sql.DB
	cfg           *config.Config
	scraper       *services.Scraper
	exporter      *services.Exporter
	checker       *services.ProxyChecker
	scheduler     *scheduler.Scheduler
	workerCount   int
	checkTimeout  time.Duration
}

// NewCommands creates a new CLI commands instance
func NewCommands(dbPath, configPath string, workerCount int, checkTimeout time.Duration) (*Commands, error) {
	// Load config
	cfg, err := loadConfig(configPath)
	if err != nil {
		return nil, err
	}

	// Override CLI settings
	if workerCount > 0 {
		cfg.WorkerCount = workerCount
	}
	if checkTimeout > 0 {
		cfg.CheckTimeout = checkTimeout
	}

	// Initialize database
	db, err := database.NewDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Run migrations
	if err := database.RunMigrations(db); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	// Initialize services
	checker := services.NewProxyChecker(cfg)
	pool := services.NewWorkerPool(cfg.WorkerCount, checker)
	scraper := services.NewScraper()
	exporter := services.NewExporter(db, "data")

	// Initialize scheduler
	sched := scheduler.NewScheduler(db, cfg, checker, pool, scraper, exporter)
	if err := sched.SetupJobs(); err != nil {
		return nil, fmt.Errorf("failed to setup scheduler: %w", err)
	}

	return &Commands{
		db:           db,
		cfg:          cfg,
		scraper:      scraper,
		exporter:     exporter,
		checker:      checker,
		scheduler:    sched,
		workerCount:  cfg.WorkerCount,
		checkTimeout: cfg.CheckTimeout,
	}, nil
}

// loadConfig loads configuration from file or environment
func loadConfig(configPath string) (*config.Config, error) {
	// Try to load from .env file first
	if _, err := os.Stat(configPath); err == nil {
		// File exists, load it
		// Set environment variables from file
		file, err := os.Open(configPath)
		if err == nil {
			defer file.Close()
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line != "" && !strings.HasPrefix(line, "#") {
					parts := strings.SplitN(line, "=", 2)
					if len(parts) == 2 {
						os.Setenv(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
					}
				}
			}
		}
	}

	return config.Load()
}

// ParseFlags parses CLI flags and returns the command to execute
func ParseFlags() (command string, args map[string]string, workers int, timeout time.Duration, dbPath, configPath string) {
	// Define flags
	addSources := flag.String("add-sources", "", "File containing source URLs to add")
	listSources := flag.Bool("list-sources", false, "List all sources")
	refreshSources := flag.Bool("refresh-sources", false, "Refresh all sources")
	getSettings := flag.Bool("get-settings", false, "Get all settings")
	setSetting := flag.String("set", "", "Set a setting (key=value)")
	checkAlive := flag.Bool("check-alive", false, "Re-check alive proxies only")
	checkAll := flag.Bool("check-all", false, "Full scrape and check cycle")
	workersFlag := flag.Int("workers", 0, "Worker count (override)")
	timeoutFlag := flag.Duration("timeout", 0, "Check timeout (override)")
	dbPathFlag := flag.String("db", "", "Database path (override)")
	configPathFlag := flag.String("config", ".env", "Config file path")

	flag.Parse()

	// Build args map
	args = make(map[string]string)
	if *addSources != "" {
		command = "add-sources"
		args["file"] = *addSources
	}
	if *listSources {
		command = "list-sources"
	}
	if *refreshSources {
		command = "refresh-sources"
	}
	if *getSettings {
		command = "get-settings"
	}
	if *setSetting != "" {
		command = "set-setting"
		parts := strings.SplitN(*setSetting, "=", 2)
		if len(parts) == 2 {
			args["key"] = parts[0]
			args["value"] = parts[1]
		}
	}
	if *checkAlive {
		command = "check-alive"
	}
	if *checkAll {
		command = "check-all"
	}

	workers = *workersFlag
	timeout = *timeoutFlag
	dbPath = *dbPathFlag
	configPath = *configPathFlag

	return
}

// AddSources adds sources from a file
func (c *Commands) AddSources(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	var urls []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		url := strings.TrimSpace(scanner.Text())
		if url != "" && !strings.HasPrefix(url, "#") {
			urls = append(urls, url)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	if len(urls) == 0 {
		return fmt.Errorf("no URLs found in file")
	}

	// Add sources
	added := 0
	for _, url := range urls {
		_, err := database.InsertSource(c.db, url, true, 60)
		if err == nil {
			added++
		}
	}

	fmt.Printf("Added %d of %d sources\n", added, len(urls))
	return nil
}

// ListSources lists all sources
func (c *Commands) ListSources() error {
	sources, err := database.GetSources(c.db)
	if err != nil {
		return fmt.Errorf("failed to get sources: %w", err)
	}

	fmt.Printf("Total: %d sources\n\n", len(sources))
	for _, s := range sources {
		fmt.Printf("ID: %d\n", s.ID)
		fmt.Printf("URL: %s\n", s.URL)
		fmt.Printf("Status: %s\n", s.LastStatus)
		fmt.Printf("Proxies: %d\n", s.ProxyCount)
		fmt.Printf("Last Fetched: %s\n\n", s.LastFetched.Format("2006-01-02 15:04:05"))
	}

	return nil
}

// RefreshSources refreshes all sources
func (c *Commands) RefreshSources() error {
	sources, err := database.GetSources(c.db)
	if err != nil {
		return fmt.Errorf("failed to get sources: %w", err)
	}

	fmt.Printf("Refreshing %d sources...\n", len(sources))

	for _, source := range sources {
		proxies, err := c.scraper.Fetch(source.URL)
		if err != nil {
			database.UpdateSourceStatus(c.db, source.ID, "error", 0)
			log.Printf("[ERROR] %s - %v", source.URL, err)
			continue
		}

		var proxyList []database.Proxy
		for _, addr := range proxies {
			proxyList = append(proxyList, database.Proxy{Address: addr})
		}

		inserted, duplicates, err := database.InsertProxiesBatch(c.db, proxyList, database.SourceTypeURL, source.URL)
		if err != nil {
			database.UpdateSourceStatus(c.db, source.ID, "error", 0)
			log.Printf("[ERROR] %s - DB insert failed: %v", source.URL, err)
			continue
		}

		database.UpdateSourceStatus(c.db, source.ID, "success", inserted)
		log.Printf("[OK] %s | Fetched: %d | Inserted: %d | Duplicates: %d",
			source.URL, len(proxies), inserted, duplicates)
	}

	fmt.Println("Refresh complete")
	return nil
}

// GetSettings gets all settings
func (c *Commands) GetSettings() error {
	settings, err := database.GetAllSettings(c.db)
	if err != nil {
		return fmt.Errorf("failed to get settings: %w", err)
	}

	fmt.Println("Current Settings:")
	fmt.Println("=================")
	for _, s := range settings {
		fmt.Printf("%s: %s\n", s.Key, s.Value)
	}

	return nil
}

// SetSetting sets a single setting
func (c *Commands) SetSetting(key, value string) error {
	if err := database.UpdateSettingsBatch(c.db, map[string]string{key: value}); err != nil {
		return fmt.Errorf("failed to update setting: %w", err)
	}

	fmt.Printf("Setting updated: %s = %s\n", key, value)
	fmt.Println("Note: Some settings require application restart to take effect")
	return nil
}

// CheckAlive re-checks alive proxies only
func (c *Commands) CheckAlive() error {
	fmt.Println("Starting alive proxy re-check...")
	c.scheduler.TriggerAliveCheck()

	// Wait a bit for check to start
	time.Sleep(2 * time.Second)

	// Monitor progress
	c.monitorProgress()

	return nil
}

// CheckAll runs full scrape and check cycle
func (c *Commands) CheckAll() error {
	fmt.Println("Starting full scrape and check cycle...")
	c.scheduler.TriggerCheck()

	// Wait a bit for check to start
	time.Sleep(2 * time.Second)

	// Monitor progress
	c.monitorProgress()

	return nil
}

// monitorProgress monitors check progress
func (c *Commands) monitorProgress() {
	for {
		status := c.scheduler.GetStatus()
		if running, ok := status["running"].(bool); ok && running {
			if total, ok := status["progress_total"].(int); ok && total > 0 {
				processed := status["progress_processed"].(int)
				alive := status["progress_alive"].(int)
				dead := status["progress_dead"].(int)
				percent := float64(processed) / float64(total) * 100

				fmt.Printf("\rProgress: %d/%d (%.1f%%) | Alive: %d | Dead: %d",
					processed, total, percent, alive, dead)
			}
			time.Sleep(2 * time.Second)
		} else {
			fmt.Println("\nCheck complete")
			break
		}
	}
}

// Close closes the CLI commands
func (c *Commands) Close() {
	if c.db != nil {
		c.db.Close()
	}
}

// WorkerCount returns the worker count
func (c *Commands) WorkerCount() int {
	return c.workerCount
}

// CheckTimeout returns the check timeout
func (c *Commands) CheckTimeout() time.Duration {
	return c.checkTimeout
}
