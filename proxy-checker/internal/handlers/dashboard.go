package handlers

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"proxy-checker/internal/database"
)

// DashboardHandler handles dashboard routes
type DashboardHandler struct {
	db *sql.DB
}

// NewDashboardHandler creates a new dashboard handler
func NewDashboardHandler(db *sql.DB) *DashboardHandler {
	return &DashboardHandler{db: db}
}

// ShowDashboard renders the dashboard page
func (h *DashboardHandler) ShowDashboard(c *gin.Context) {
	// Get stats
	stats, err := database.GetStats(h.db)
	if err != nil {
		stats = make(map[string]interface{})
	}

	// Get recent proxies (limit 10)
	proxies, _, err := database.GetProxies(h.db, 1, 10, "")
	if err != nil {
		proxies = []database.Proxy{}
	}

	// Get sources
	sources, err := database.GetSources(h.db)
	if err != nil {
		sources = []database.Source{}
	}

	c.HTML(http.StatusOK, "dashboard.html", gin.H{
		"title":   "Dashboard",
		"stats":   stats,
		"proxies": proxies,
		"sources": sources,
	})
}
