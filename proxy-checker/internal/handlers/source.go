package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

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

// Create adds a new source
func (h *SourceHandler) Create(c *gin.Context) {
	var req struct {
		URL          string `json:"url"`
		AutoRefresh  bool   `json:"auto_refresh"`
		IntervalMin  int    `json:"interval_minutes"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	if req.URL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "URL is required"})
		return
	}

	if req.IntervalMin == 0 {
		req.IntervalMin = 20
	}

	id, err := database.InsertSource(h.db, req.URL, req.AutoRefresh, req.IntervalMin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"id":      id,
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

	// Fetch proxies
	proxies, err := h.scraper.Fetch(source.URL)
	if err != nil {
		database.UpdateSourceStatus(h.db, id, "error", 0)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Convert to Proxy structs
	var proxyList []database.Proxy
	for _, addr := range proxies {
		proxyList = append(proxyList, database.Proxy{Address: addr})
	}

	// Insert proxies
	inserted, err := database.InsertProxiesBatch(h.db, proxyList, database.SourceTypeURL, source.URL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Update source status
	database.UpdateSourceStatus(h.db, id, "success", inserted)

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"fetched":  len(proxies),
		"inserted": inserted,
	})
}
