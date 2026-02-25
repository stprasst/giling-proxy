package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"proxy-checker/internal/database"
	"proxy-checker/internal/services"
)

// ProxyHandler handles proxy routes
type ProxyHandler struct {
	db       *sql.DB
	scraper  *services.Scraper
	exporter *services.Exporter
}

// NewProxyHandler creates a new proxy handler
func NewProxyHandler(db *sql.DB, scraper *services.Scraper, exporter *services.Exporter) *ProxyHandler {
	return &ProxyHandler{
		db:       db,
		scraper:  scraper,
		exporter: exporter,
	}
}

// BulkAdd handles bulk proxy addition
func (h *ProxyHandler) BulkAdd(c *gin.Context) {
	var req struct {
		Proxies    string `json:"proxies"`
		SourceType string `json:"source_type"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	if req.SourceType == "" {
		req.SourceType = database.SourceTypeManual
	}

	// Parse proxies
	lines := strings.Split(req.Proxies, "\n")
	var proxies []database.Proxy
	for _, line := range lines {
		addr, ok := services.ParseAddress(line)
		if ok {
			proxies = append(proxies, database.Proxy{Address: addr})
		}
	}

	if len(proxies) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No valid proxies found"})
		return
	}

	// Insert proxies
	inserted, err := database.InsertProxiesBatch(h.db, proxies, req.SourceType, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"inserted": inserted,
		"total":    len(proxies),
	})
}

// FetchFromURL handles fetching proxies from a URL
func (h *ProxyHandler) FetchFromURL(c *gin.Context) {
	var req struct {
		URL         string `json:"url"`
		IncludeSelf bool   `json:"include_self"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	if req.URL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "URL is required"})
		return
	}

	// Fetch proxies from URL
	proxies, err := h.scraper.Fetch(req.URL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Optionally include self proxy_alive.txt
	var selfProxies []string
	if req.IncludeSelf {
		selfProxies, _ = h.scraper.Fetch("/public/proxy_alive.txt")
		proxies = append(proxies, selfProxies...)
	}

	// Convert to Proxy structs
	var proxyList []database.Proxy
	for _, addr := range proxies {
		proxyList = append(proxyList, database.Proxy{Address: addr})
	}

	// Insert proxies
	inserted, err := database.InsertProxiesBatch(h.db, proxyList, database.SourceTypeURL, req.URL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"fetched":      len(proxies),
		"inserted":     inserted,
		"self_included": len(selfProxies),
	})
}

// List returns paginated proxy list
func (h *ProxyHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	status := c.Query("status")

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}

	proxies, total, err := database.GetProxies(h.db, page, limit, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    proxies,
		"total":   total,
		"page":    page,
		"limit":   limit,
	})
}

// Delete removes a proxy
func (h *ProxyHandler) Delete(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	if err := database.DeleteProxy(h.db, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Export triggers manual export
func (h *ProxyHandler) Export(c *gin.Context) {
	if err := h.exporter.ExportAlive(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Export completed",
	})
}
