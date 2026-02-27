package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"

	"proxy-checker/internal/cli"
	"proxy-checker/internal/config"
	"proxy-checker/internal/database"
	"proxy-checker/internal/handlers"
	"proxy-checker/internal/middleware"
	"proxy-checker/internal/scheduler"
	"proxy-checker/internal/services"
)

func main() {
	// Parse CLI flags
	command, args, workersFlag, timeoutFlag, dbPathFlag, configPath, _ := cli.ParseFlags()

	// Execute CLI command if provided
	if command != "" {
		if err := runCLI(command, args, workersFlag, timeoutFlag, dbPathFlag, configPath); err != nil {
			log.Fatalf("Error: %v", err)
		}
		return
	}

	// Default: Run GUI server
	runServer(dbPathFlag, configPath, workersFlag, timeoutFlag)
}

// runCLI executes CLI commands
func runCLI(command string, args map[string]string, workers int, timeout time.Duration, dbPath, configPath string) error {
	// Default paths
	if dbPath == "" {
		dbPath = "data/proxy.db"
	}

	// Initialize CLI commands
	cmd, err := cli.NewCommands(dbPath, configPath, workers, timeout)
	if err != nil {
		return fmt.Errorf("failed to initialize: %w", err)
	}
	defer cmd.Close()

	// Execute command
	switch command {
	case "add-sources":
		file, ok := args["file"]
		if !ok {
			return fmt.Errorf("--add-sources requires a file path")
		}
		return cmd.AddSources(file)

	case "list-sources":
		return cmd.ListSources()

	case "refresh-sources":
		return cmd.RefreshSources()

	case "get-settings":
		return cmd.GetSettings()

	case "set-setting":
		key, ok1 := args["key"]
		value, ok2 := args["value"]
		if !ok1 || !ok2 {
			return fmt.Errorf("--set requires key=value")
		}
		return cmd.SetSetting(key, value)

	case "check-alive":
		return cmd.CheckAlive()

	case "check-all":
		return cmd.CheckAll()

	case "daemon":
		return cmd.RunDaemon()

	default:
		return fmt.Errorf("unknown command: %s", command)
	}
}

// runServer starts the web server
func runServer(dbPathFlag, configPath string, workersFlag int, timeoutFlag time.Duration) {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Starting Proxy Checker...")
	log.Printf("Port: %s, DB: %s, Workers: %d", cfg.Port, cfg.DBPath, cfg.WorkerCount)

	// Initialize database
	db, err := database.NewDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	log.Printf("Database initialized with WAL mode")

	// Run migrations
	if err := database.RunMigrations(db); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	log.Printf("Migrations completed")

	// Create initial admin user
	if err := handlers.SetupInitialUser(db, cfg.AdminPassword); err != nil {
		log.Fatalf("Failed to create admin user: %v", err)
	}

	log.Printf("Admin user initialized")

	// Read worker count from database settings (or use config default)
	workerCountStr := database.GetSettingWithDefault(db, "worker_count", "100")
	workerCount := cfg.WorkerCount
	if wc, err := strconv.Atoi(workerCountStr); err == nil && wc > 0 {
		workerCount = wc
	}
	log.Printf("Using worker count: %d (from settings: %s)", workerCount, workerCountStr)

	// Initialize services
	checker := services.NewProxyChecker(cfg)
	pool := services.NewWorkerPool(workerCount, checker)
	scraper := services.NewScraper()
	exporter := services.NewExporter(db, "data")

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(db)
	dashboardHandler := handlers.NewDashboardHandler(db)
	proxyHandler := handlers.NewProxyHandler(db, scraper, exporter)
	sourceHandler := handlers.NewSourceHandler(db, scraper)
	statsHandler := handlers.NewStatsHandler(db)
	settingsHandler := handlers.NewSettingsHandler(db)

	// Initialize scheduler
	sched := scheduler.NewScheduler(db, cfg, checker, pool, scraper, exporter)
	if err := sched.SetupJobs(); err != nil {
		log.Fatalf("Failed to setup scheduler: %v", err)
	}

	checkHandler := handlers.NewCheckHandler(sched)

	// Setup Gin router
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	// Session middleware
	store := cookie.NewStore([]byte(cfg.SessionSecret))
	store.Options(sessions.Options{
		MaxAge:   86400,
		HttpOnly: true,
		Path:     "/",
	})
	r.Use(sessions.Sessions("proxy-checker", store))

	// Load templates
	r.LoadHTMLGlob("templates/*")

	// Static files
	r.Static("/static", "./static")
	r.Static("/public", "./data/public")

	// Public routes
	r.GET("/login", authHandler.ShowLogin)
	r.POST("/login", authHandler.Login)
	r.GET("/logout", authHandler.Logout)

	// Protected routes
	admin := r.Group("/admin")
	admin.Use(middleware.AuthRequired())
	{
		admin.GET("/dashboard", dashboardHandler.ShowDashboard)
	}

	api := r.Group("/api")
	api.Use(middleware.AuthRequired())
	{
		// Proxy routes
		api.POST("/proxies/bulk", proxyHandler.BulkAdd)
		api.POST("/proxies/fetch", proxyHandler.FetchFromURL)
		api.GET("/proxies", proxyHandler.List)
		api.DELETE("/proxies/:id", proxyHandler.Delete)
		api.POST("/export", proxyHandler.Export)

		// Source routes
		api.POST("/sources", sourceHandler.Create)
		api.GET("/sources", sourceHandler.List)
		api.DELETE("/sources/:id", sourceHandler.Delete)
		api.POST("/sources/:id/refresh", sourceHandler.Refresh)
		api.POST("/sources/refresh-all", sourceHandler.RefreshAll)

		// Stats routes
		api.GET("/stats", statsHandler.GetStats)
		api.GET("/logs", statsHandler.GetLogs)

		// Check routes
		api.POST("/check/trigger", checkHandler.Trigger)
		api.POST("/check/alive", checkHandler.TriggerAlive)
		api.GET("/check/status", checkHandler.Status)

		// Settings routes
		api.GET("/settings", settingsHandler.GetAll)
		api.POST("/settings", settingsHandler.Update)
		api.POST("/settings/password", settingsHandler.UpdatePassword)
	}

	// Root redirect
	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/login")
	})

	// Ensure data directories exist
	if err := os.MkdirAll("data/public", 0755); err != nil {
		log.Fatalf("Failed to create public directory: %v", err)
	}

	// Start scheduler
	sched.Start()
	defer sched.Stop()

	// Start server
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	// Graceful shutdown - Windows compatible
	go func() {
		log.Printf("Server starting on port %s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal (Windows + Unix compatible)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt) // Ctrl+C on all platforms
	<-quit

	log.Println("Shutting down server...")

	// Stop scheduler first
	sched.Stop()

	// Shutdown HTTP server with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}
