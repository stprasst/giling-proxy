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
	cron          *cron.Cron
	db            *sql.DB
	checker       *services.ProxyChecker
	pool          *services.WorkerPool
	scraper       *services.Scraper
	exporter      *services.Exporter
	config        *config.Config
	mu            sync.Mutex
	running       bool
	lastCheck     time.Time
	nextCheck     time.Time // Next alive re-check
	nextScrape    time.Time // Next scrape+check
	checkInterval time.Duration
	scrapeInterval time.Duration
	checkTimeout  time.Duration
	// Progress tracking
	progressMu    sync.RWMutex
	progress      CheckProgress
}

// CheckProgress holds the current check progress
type CheckProgress struct {
	Total      int
	Processed  int
	Alive      int
	Dead       int
	StartedAt  time.Time
}

// NewScheduler creates a new scheduler
func NewScheduler(db *sql.DB, cfg *config.Config, checker *services.ProxyChecker, pool *services.WorkerPool, scraper *services.Scraper, exporter *services.Exporter) *Scheduler {
	return &Scheduler{
		cron: cron.New(cron.WithChain(
			cron.Recover(cron.DefaultLogger),
		)),
		db:            db,
		checker:       checker,
		pool:          pool,
		scraper:       scraper,
		exporter:      exporter,
		config:        cfg,
		checkInterval: cfg.CheckInterval,
		scrapeInterval: time.Hour, // Default: scrape every 60 min
		checkTimeout:  cfg.CheckTimeout,
		nextCheck:     time.Now().Add(cfg.CheckInterval),
	}
}

// SetupJobs configures all scheduled jobs
func (s *Scheduler) SetupJobs() error {
	// Read intervals from database settings
	checkIntervalStr := database.GetSettingWithDefault(s.db, "check_interval", "15m")
	if interval, err := time.ParseDuration(checkIntervalStr); err == nil {
		s.checkInterval = interval
	}
	scrapeIntervalStr := database.GetSettingWithDefault(s.db, "scrape_interval", "60m")
	if interval, err := time.ParseDuration(scrapeIntervalStr); err == nil {
		s.scrapeInterval = interval
	}
	checkTimeoutStr := database.GetSettingWithDefault(s.db, "check_timeout", "10s")
	if timeout, err := time.ParseDuration(checkTimeoutStr); err == nil {
		s.checkTimeout = timeout
	}

	// Read protocol settings (SOCKS4/SOCKS5)
	checkSOCKS4Str := database.GetSettingWithDefault(s.db, "check_socks4", "true")
	checkSOCKS5Str := database.GetSettingWithDefault(s.db, "check_socks5", "true")
	checkSOCKS4 := checkSOCKS4Str == "true" || checkSOCKS4Str == "1"
	checkSOCKS5 := checkSOCKS5Str == "true" || checkSOCKS5Str == "1"
	s.checker.SetProtocolSettings(checkSOCKS4, checkSOCKS5)

	// Update initial next_check times so countdowns display correctly
	s.mu.Lock()
	s.nextCheck = time.Now().Add(s.checkInterval)
	s.nextScrape = time.Now().Add(s.scrapeInterval)
	s.mu.Unlock()

	// Job 1: Re-check alive proxies every check_interval (default 15 min)
	checkCronExpr := "@every " + s.checkInterval.String()
	_, err := s.cron.AddFunc(checkCronExpr, func() {
		s.checkAliveProxies()
	})
	if err != nil {
		return err
	}

	// Job 2: Scrape sources + check new proxies every scrape_interval (default 60 min)
	scrapeCronExpr := "@every " + s.scrapeInterval.String()
	_, err = s.cron.AddFunc(scrapeCronExpr, func() {
		s.scrapeAndCheck()
	})
	if err != nil {
		return err
	}

	log.Printf("Scheduler: Jobs configured (re-check alive: %s, scrape+check: %s, timeout: %s)",
		s.checkInterval, s.scrapeInterval, s.checkTimeout)
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

// checkAliveProxies re-checks all alive proxies every check_interval
func (s *Scheduler) checkAliveProxies() {
	s.mu.Lock()
	if s.running {
		log.Println("Scheduler: Check already in progress, skipping")
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	// Reset progress
	s.progressMu.Lock()
	s.progress = CheckProgress{StartedAt: time.Now()}
	s.progressMu.Unlock()

	defer func() {
		s.mu.Lock()
		s.running = false
		s.lastCheck = time.Now()
		s.nextCheck = time.Now().Add(s.checkInterval) // Next alive re-check
		s.mu.Unlock()

		// Clear progress after a delay
		time.AfterFunc(30*time.Second, func() {
			s.progressMu.Lock()
			s.progress = CheckProgress{}
			s.progressMu.Unlock()
		})
	}()

	log.Println("Scheduler: Starting alive proxy re-check")

	// Get alive proxies for re-checking
	proxies, err := database.GetAliveProxies(s.db)
	if err != nil {
		log.Printf("Scheduler: Error getting proxies: %v", err)
		return
	}

	if len(proxies) == 0 {
		log.Println("Scheduler: No alive proxies to re-check")
		return
	}

	// Update progress total
	s.progressMu.Lock()
	s.progress.Total = len(proxies)
	s.progressMu.Unlock()

	log.Printf("Scheduler: Re-checking %d alive proxies", len(proxies))

	s.runProxyCheck(proxies, "alive")
}

// scrapeAndCheck scrapes sources and checks alive + new proxies every scrape_interval
func (s *Scheduler) scrapeAndCheck() {
	s.mu.Lock()
	if s.running {
		log.Println("Scheduler: Check already in progress, skipping")
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	// Reset progress
	s.progressMu.Lock()
	s.progress = CheckProgress{StartedAt: time.Now()}
	s.progressMu.Unlock()

	defer func() {
		s.mu.Lock()
		s.running = false
		s.lastCheck = time.Now()
		s.nextScrape = time.Now().Add(s.scrapeInterval) // Next scrape+check
		s.mu.Unlock()

		// Clear progress after a delay
		time.AfterFunc(30*time.Second, func() {
			s.progressMu.Lock()
			s.progress = CheckProgress{}
			s.progressMu.Unlock()
		})
	}()

	log.Println("Scheduler: Starting scrape and check cycle")

	// First, scrape all sources
	s.scrapeAllSources()

	// Get alive proxies for re-checking
	aliveProxies, err := database.GetAliveProxies(s.db)
	if err != nil {
		log.Printf("Scheduler: Error getting alive proxies: %v", err)
		return
	}

	// Get new/unchecked proxies
	newProxies, err := database.GetUncheckedProxies(s.db)
	if err != nil {
		log.Printf("Scheduler: Error getting new proxies: %v", err)
		return
	}

	// Combine both lists
	var allProxies []database.Proxy
	allProxies = append(allProxies, aliveProxies...)
	allProxies = append(allProxies, newProxies...)

	if len(allProxies) == 0 {
		log.Println("Scheduler: No proxies to check")
		return
	}

	// Update progress total
	s.progressMu.Lock()
	s.progress.Total = len(allProxies)
	s.progressMu.Unlock()

	log.Printf("Scheduler: Checking %d proxies (alive: %d, new: %d)", len(allProxies), len(aliveProxies), len(newProxies))

	s.runProxyCheck(allProxies, "all")
}

// BatchResult holds results for batch DB writes
type BatchResult struct {
	ID      int64
	Address string
	Alive   bool
	Type    string
	Latency int
	Error   string
}

// runProxyCheck executes the actual proxy checking logic
func (s *Scheduler) runProxyCheck(proxies []database.Proxy, checkType string) {
	// Start worker pool
	s.pool.Start()

	// Channels for async batch processing
	resultChan := make(chan services.ProxyResult, 1000)
	batchWriterDone := make(chan struct{})

	// Start async batch writer goroutine
	go func() {
		defer close(batchWriterDone)
		batchSize := 500
		batch := make([]BatchResult, 0, batchSize)
		batchTimer := time.NewTicker(2 * time.Second)
		defer batchTimer.Stop()

		flushBatch := func() {
			if len(batch) == 0 {
				return
			}
			// Convert to database types
			statusBatch := make([]database.ProxyResultBatch, len(batch))
			logBatch := make([]database.CheckLogBatch, len(batch))
			for i, r := range batch {
				status := database.StatusAlive
				if !r.Alive {
					status = database.StatusDead
				}
				statusBatch[i] = database.ProxyResultBatch{
					ID:      r.ID,
					Alive:   r.Alive,
					Type:    r.Type,
					Latency: r.Latency,
				}
				logBatch[i] = database.CheckLogBatch{
					ProxyID:  r.ID,
					Status:   status,
					Latency:  r.Latency,
					ErrorMsg: r.Error,
				}
			}
			// Batch update proxies
			if err := database.UpdateProxyStatusBatch(s.db, statusBatch); err != nil {
				log.Printf("Scheduler: Batch update error: %v", err)
			}
			// Batch insert check logs
			if err := database.InsertCheckLogBatch(s.db, logBatch); err != nil {
				log.Printf("Scheduler: Batch log insert error: %v", err)
			}
			log.Printf("Scheduler: Batch write completed - %d records", len(batch))
			batch = batch[:0] // Clear batch
		}

		for {
			select {
			case result, ok := <-resultChan:
				if !ok {
					// Channel closed, flush remaining
					flushBatch()
					return
				}
				// Add to batch
				batch = append(batch, BatchResult{
					ID:      result.ID,
					Address: result.Address,
					Alive:   result.Alive,
					Type:    result.Type,
					Latency: result.Latency,
					Error:   result.Error,
				})
				// Flush when batch is full
				if len(batch) >= batchSize {
					flushBatch()
				}
			case <-batchTimer.C:
				// Flush every 2 seconds (for progress visibility)
				flushBatch()
			}
		}
	}()

	// Result collector for UI updates (non-blocking)
	resultDone := make(chan struct{})
	var alive, dead, processed int
	var mu sync.Mutex

	go func() {
		defer close(resultDone)
		defer close(resultChan) // Signal batch writer to finish
		progressInterval := 100
		lastProgress := time.Now()

		for result := range s.pool.Results() {
			mu.Lock()
			processed++
			if result.Alive {
				alive++
			} else {
				dead++
			}
			currentProcessed := processed
			currentAlive := alive
			currentDead := dead
			mu.Unlock()

			// Send to batch writer (non-blocking - channel is buffered)
			resultChan <- result

			// Update UI progress instantly
			s.progressMu.Lock()
			s.progress.Processed = currentProcessed
			s.progress.Alive = currentAlive
			s.progress.Dead = currentDead
			s.progressMu.Unlock()

			// Log progress every N proxies or every 30 seconds
			if currentProcessed%progressInterval == 0 || time.Since(lastProgress) > 30*time.Second {
				s.progressMu.RLock()
				total := s.progress.Total
				s.progressMu.RUnlock()
				percent := float64(currentProcessed) / float64(total) * 100
				log.Printf("Scheduler: Progress %d/%d (%.1f%%) - Alive: %d, Dead: %d",
					currentProcessed, total, percent, currentAlive, currentDead)
				lastProgress = time.Now()
			}
		}
	}()

	// Submit jobs - now safe because result collector is running
	submitted := 0
	for _, p := range proxies {
		job := services.ProxyJob{
			ID:      p.ID,
			Address: p.Address,
			Timeout: s.checkTimeout, // Use from settings
		}
		if s.pool.Submit(job) {
			submitted++
		} else {
			log.Printf("Scheduler: Failed to submit job for proxy %s", p.Address)
		}
	}

	log.Printf("Scheduler: Submitted %d jobs, waiting for results...", submitted)

	if submitted < len(proxies) {
		log.Printf("Scheduler: Warning - Only submitted %d of %d proxies", submitted, len(proxies))
	}

	// Stop the pool (closes result channel, ending the collector goroutine)
	s.pool.Stop()

	// Wait for result collector to finish
	<-resultDone

	// Wait for batch writer to finish
	<-batchWriterDone

	mu.Lock()
	finalAlive := alive
	finalDead := dead
	finalProcessed := processed
	mu.Unlock()

	log.Printf("Scheduler: %s proxy check complete - Processed: %d, Alive: %d, Dead: %d", checkType, finalProcessed, finalAlive, finalDead)

	// Delete dead proxies
	deleted, err := database.DeleteDeadProxies(s.db)
	if err != nil {
		log.Printf("Scheduler: Error deleting dead proxies: %v", err)
	} else if deleted > 0 {
		log.Printf("Scheduler: Deleted %d dead proxies", deleted)
	}

	// Export alive proxies
	if err := s.exporter.ExportAll(); err != nil {
		log.Printf("Scheduler: Export error: %v", err)
	} else {
		log.Println("Scheduler: Exported alive proxies (all formats)")
	}
}

// scrapeAllSources scrapes all sources regardless of auto_refresh setting
func (s *Scheduler) scrapeAllSources() {
	sources, err := database.GetSources(s.db)
	if err != nil {
		log.Printf("Scheduler: Error getting sources: %v", err)
		return
	}

	log.Printf("Scheduler: Starting scrape cycle for %d sources...", len(sources))

	for _, source := range sources {
		// Fetch proxies
		proxies, err := s.scraper.Fetch(source.URL)
		if err != nil {
			database.UpdateSourceStatus(s.db, source.ID, "error", 0)
			log.Printf("Scheduler: [ERROR] %s - %v", source.URL, err)
			continue
		}

		fetchedCount := len(proxies)

		// Convert to Proxy structs
		var proxyList []database.Proxy
		for _, addr := range proxies {
			proxyList = append(proxyList, database.Proxy{Address: addr})
		}

		// Insert proxies
		inserted, duplicates, err := database.InsertProxiesBatch(s.db, proxyList, database.SourceTypeURL, source.URL)
		if err != nil {
			database.UpdateSourceStatus(s.db, source.ID, "error", 0)
			log.Printf("Scheduler: [ERROR] %s - DB insert failed: %v", source.URL, err)
			continue
		}

		database.UpdateSourceStatus(s.db, source.ID, "success", inserted)
		log.Printf("Scheduler: [OK] %s | Fetched: %d | Inserted: %d | Duplicates: %d",
			source.URL, fetchedCount, inserted, duplicates)
	}
}

// TriggerCheck manually triggers a full scrape and check cycle
func (s *Scheduler) TriggerCheck() {
	// Manual trigger does NOT update countdown - only scheduled jobs update countdown
	go s.scrapeAndCheck()
}

// TriggerAliveCheck manually triggers an alive-only re-check
func (s *Scheduler) TriggerAliveCheck() {
	// Manual trigger does NOT update countdown - only scheduled jobs update countdown
	go s.checkAliveProxies()
}

// GetStatus returns the current scheduler status
func (s *Scheduler) GetStatus() map[string]interface{} {
	s.mu.Lock()
	running := s.running
	lastCheck := s.lastCheck
	nextCheck := s.nextCheck
	nextScrape := s.nextScrape
	checkInterval := s.checkInterval
	s.mu.Unlock()

	s.progressMu.RLock()
	progress := s.progress
	s.progressMu.RUnlock()

	// Read current settings from database
	intervalStr := database.GetSettingWithDefault(s.db, "check_interval", "20m")
	workerCountStr := database.GetSettingWithDefault(s.db, "worker_count", "100")

	return map[string]interface{}{
		"running":        running,
		"last_check":     lastCheck,
		"next_check":     nextCheck,
		"next_scrape":    nextScrape,
		"check_interval": checkInterval.Seconds(),
		// Database settings
		"settings_interval": intervalStr,
		"settings_workers":  workerCountStr,
		// Progress
		"progress_total":     progress.Total,
		"progress_processed": progress.Processed,
		"progress_alive":     progress.Alive,
		"progress_dead":      progress.Dead,
		"progress_percent":   calculatePercent(progress.Processed, progress.Total),
		"progress_started":   progress.StartedAt,
	}
}

func calculatePercent(processed, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(processed) / float64(total) * 100
}
