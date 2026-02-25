package scheduler

import (
	"database/sql"
	"log"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"proxy-checker/internal/config"
	"proxy-checker/internal/database"
	"proxy-checker/internal/services"
)

// Scheduler handles scheduled tasks
type Scheduler struct {
	cron     *cron.Cron
	db       *sql.DB
	checker  *services.ProxyChecker
	pool     *services.WorkerPool
	scraper  *services.Scraper
	exporter *services.Exporter
	config   *config.Config
	mu       sync.Mutex
	running  bool
}

// NewScheduler creates a new scheduler
func NewScheduler(db *sql.DB, cfg *config.Config, checker *services.ProxyChecker, pool *services.WorkerPool, scraper *services.Scraper, exporter *services.Exporter) *Scheduler {
	return &Scheduler{
		cron: cron.New(cron.WithChain(
			cron.Recover(cron.DefaultLogger),
		)),
		db:       db,
		checker:  checker,
		pool:     pool,
		scraper:  scraper,
		exporter: exporter,
		config:   cfg,
	}
}

// SetupJobs configures all scheduled jobs
func (s *Scheduler) SetupJobs() error {
	// Job 1: Check proxies periodically
	interval := s.config.CheckInterval
	cronExpr := "@every " + interval.String()

	_, err := s.cron.AddFunc(cronExpr, func() {
		s.checkAllProxies()
	})
	if err != nil {
		return err
	}

	// Job 2: Refresh sources every 5 minutes
	_, err = s.cron.AddFunc("*/5 * * * *", func() {
		s.refreshSources()
	})
	if err != nil {
		return err
	}

	log.Printf("Scheduler: Jobs configured (check interval: %s)", interval)
	return nil
}

// Start begins the scheduler
func (s *Scheduler) Start() {
	s.cron.Start()
	log.Println("Scheduler: Started")
}

// Stop halts the scheduler
func (s *Scheduler) Stop() {
	s.cron.Stop()
	log.Println("Scheduler: Stopped")
}

// checkAllProxies checks all unchecked/dead proxies
func (s *Scheduler) checkAllProxies() {
	s.mu.Lock()
	if s.running {
		log.Println("Scheduler: Check already in progress, skipping")
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	log.Println("Scheduler: Starting proxy check")

	// Get unchecked proxies
	proxies, err := database.GetUncheckedProxies(s.db)
	if err != nil {
		log.Printf("Scheduler: Error getting proxies: %v", err)
		return
	}

	if len(proxies) == 0 {
		log.Println("Scheduler: No proxies to check")
		return
	}

	log.Printf("Scheduler: Checking %d proxies", len(proxies))

	// Start worker pool
	s.pool.Start()

	// Submit jobs
	submitted := 0
	for _, p := range proxies {
		job := services.ProxyJob{
			ID:      p.ID,
			Address: p.Address,
			Timeout: s.config.CheckTimeout,
		}
		if s.pool.Submit(job) {
			submitted++
		}
	}

	// Collect results
	alive := 0
	dead := 0
	timeout := time.After(5 * time.Minute)

	for i := 0; i < submitted; i++ {
		select {
		case result := <-s.pool.Results():
			status := database.StatusDead
			if result.Alive {
				status = database.StatusAlive
				alive++
			} else {
				dead++
			}

			// Update proxy status
			database.UpdateProxyStatus(s.db, result.ID, status, result.Type, result.Latency)

			// Log check
			logEntry := database.CheckLog{
				ProxyID: result.ID,
				Status:  status,
				Latency: result.Latency,
				ErrorMsg: result.Error,
			}
			database.InsertCheckLog(s.db, logEntry)

		case <-timeout:
			log.Println("Scheduler: Check timeout")
			break
		}
	}

	// Stop worker pool
	s.pool.Stop()

	log.Printf("Scheduler: Check complete - Alive: %d, Dead: %d", alive, dead)

	// Export alive proxies
	if err := s.exporter.ExportAlive(); err != nil {
		log.Printf("Scheduler: Export error: %v", err)
	} else {
		log.Println("Scheduler: Exported alive proxies")
	}
}

// refreshSources refreshes all auto-refresh sources
func (s *Scheduler) refreshSources() {
	sources, err := database.GetSources(s.db)
	if err != nil {
		log.Printf("Scheduler: Error getting sources: %v", err)
		return
	}

	for _, source := range sources {
		if !source.AutoRefresh {
			continue
		}

		// Check if refresh is due
		if source.LastFetched != nil {
			nextRefresh := source.LastFetched.Add(time.Duration(source.IntervalMin) * time.Minute)
			if time.Now().Before(nextRefresh) {
				continue
			}
		}

		// Fetch proxies
		proxies, err := s.scraper.Fetch(source.URL)
		if err != nil {
			database.UpdateSourceStatus(s.db, source.ID, "error", 0)
			log.Printf("Scheduler: Error fetching source %s: %v", source.URL, err)
			continue
		}

		// Convert to Proxy structs
		var proxyList []database.Proxy
		for _, addr := range proxies {
			proxyList = append(proxyList, database.Proxy{Address: addr})
		}

		// Insert proxies
		inserted, err := database.InsertProxiesBatch(s.db, proxyList, database.SourceTypeURL, source.URL)
		if err != nil {
			database.UpdateSourceStatus(s.db, source.ID, "error", 0)
			continue
		}

		database.UpdateSourceStatus(s.db, source.ID, "success", inserted)
		log.Printf("Scheduler: Refreshed source %s - %d proxies inserted", source.URL, inserted)
	}
}

// TriggerCheck manually triggers a proxy check
func (s *Scheduler) TriggerCheck() {
	go s.checkAllProxies()
}
