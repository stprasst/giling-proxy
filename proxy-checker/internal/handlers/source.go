package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"proxy-checker/internal/database"
	"proxy-checker/internal/services"
)

// SourceHandler handles source routes
type SourceHandler struct {
	db      *sql.DB
	scraper *services.Scraper
}

// NewSourceHandler creates a new source handler
func NewSourceHandler(db *sql.DB, scraper *services.Scraper) *SourceHandler {
	return &SourceHandler{db: db, scraper: scraper}
}

// Create adds new sources (supports multiline input)
func (h *SourceHandler) Create(c *gin.Context) {
	var req struct {
		URLs         string `json:"urls"` // Multiline input
		AutoRefresh  bool   `json:"auto_refresh"`
		IntervalMin  int    `json:"interval_minutes"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	if req.URLs == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "URLs are required"})
		return
	}

	if req.IntervalMin == 0 {
		req.IntervalMin = 20
	}

	// Parse multiline URLs
	lines := strings.Split(req.URLs, "\n")
	var urls []string
	for _, line := range lines {
		url := strings.TrimSpace(line)
		if url != "" && strings.HasPrefix(url, "http") {
			urls = append(urls, url)
		}
	}

	if len(urls) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No valid URLs found"})
		return
	}

	// Insert each URL as a source
	inserted := 0
	var lastID int64
	for _, url := range urls {
		id, err := database.InsertSource(h.db, url, req.AutoRefresh, req.IntervalMin)
		if err == nil && id > 0 {
			inserted++
			lastID = id
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"inserted": inserted,
		"total":    len(urls),
		"last_id":  lastID,
	})
}

// List returns all sources
func (h *SourceHandler) List(c *gin.Context) {
	sources, err := database.GetSources(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    sources,
	})
}

// Delete removes a source
func (h *SourceHandler) Delete(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	if err := database.DeleteSource(h.db, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Refresh manually refreshes a source
func (h *SourceHandler) Refresh(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	// Get source
	sources, err := database.GetSources(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var source *database.Source
	for _, s := range sources {
		if s.ID == id {
			source = &s
			break
		}
	}

	if source == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Source not found"})
		return
	}

	log.Printf("Source: Refreshing %s...", source.URL)

	// Fetch proxies
	proxies, err := h.scraper.Fetch(source.URL)
	if err != nil {
		database.UpdateSourceStatus(h.db, id, "error", 0)
		log.Printf("Source: [ERROR] %s - %v", source.URL, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	fetchedCount := len(proxies)

	// Convert to Proxy structs
	var proxyList []database.Proxy
	for _, addr := range proxies {
		proxyList = append(proxyList, database.Proxy{Address: addr})
	}

	// Insert proxies
	inserted, duplicates, err := database.InsertProxiesBatch(h.db, proxyList, database.SourceTypeURL, source.URL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Update source status
	database.UpdateSourceStatus(h.db, id, "success", inserted)

	log.Printf("Source: [OK] %s | Fetched: %d | Inserted: %d | Duplicates: %d",
		source.URL, fetchedCount, inserted, duplicates)

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"fetched":    fetchedCount,
		"inserted":   inserted,
		"duplicates": duplicates,
	})
}

// RefreshAll refreshes all sources
func (h *SourceHandler) RefreshAll(c *gin.Context) {
	sources, err := database.GetSources(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Println("Source: Starting refresh all sources...")

	refreshed := 0
	totalFetched := 0
	totalInserted := 0
	totalDuplicates := 0
	errors := 0

	for _, source := range sources {
		proxies, err := h.scraper.Fetch(source.URL)
		if err != nil {
			database.UpdateSourceStatus(h.db, source.ID, "error", 0)
			log.Printf("Source: [ERROR] %s - %v", source.URL, err)
			errors++
			continue
		}

		fetchedCount := len(proxies)

		var proxyList []database.Proxy
		for _, addr := range proxies {
			proxyList = append(proxyList, database.Proxy{Address: addr})
		}

		inserted, duplicates, err := database.InsertProxiesBatch(h.db, proxyList, database.SourceTypeURL, source.URL)
		if err != nil {
			database.UpdateSourceStatus(h.db, source.ID, "error", 0)
			log.Printf("Source: [ERROR] %s - DB insert failed: %v", source.URL, err)
			errors++
			continue
		}

		database.UpdateSourceStatus(h.db, source.ID, "success", inserted)
		log.Printf("Source: [OK] %s | Fetched: %d | Inserted: %d | Duplicates: %d",
			source.URL, fetchedCount, inserted, duplicates)

		refreshed++
		totalFetched += fetchedCount
		totalInserted += inserted
		totalDuplicates += duplicates
	}

	log.Printf("Source: Refresh complete | Sources: %d | Total Fetched: %d | Total Inserted: %d | Total Duplicates: %d | Errors: %d",
		refreshed, totalFetched, totalInserted, totalDuplicates, errors)

	c.JSON(http.StatusOK, gin.H{
		"success":         true,
		"message":         fmt.Sprintf("Refreshed %d sources | Fetched: %d | Inserted: %d | Duplicates: %d | Errors: %d", refreshed, totalFetched, totalInserted, totalDuplicates, errors),
		"refreshed":       refreshed,
		"total_fetched":   totalFetched,
		"total_inserted":  totalInserted,
		"total_duplicates": totalDuplicates,
		"errors":          errors,
	})
}
