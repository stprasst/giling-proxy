package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"proxy-checker/internal/database"
)

// StatsHandler handles stats routes
type StatsHandler struct {
	db *sql.DB
}

// NewStatsHandler creates a new stats handler
func NewStatsHandler(db *sql.DB) *StatsHandler {
	return &StatsHandler{db: db}
}

// GetStats returns proxy statistics
func (h *StatsHandler) GetStats(c *gin.Context) {
	stats, err := database.GetStats(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}

// GetLogs returns check logs
func (h *StatsHandler) GetLogs(c *gin.Context) {
	proxyIDStr := c.Query("proxy_id")
	limitStr := c.DefaultQuery("limit", "100")

	limit, _ := strconv.Atoi(limitStr)
	if limit < 1 || limit > 1000 {
		limit = 100
	}

	var proxyID int64
	if proxyIDStr != "" {
		proxyID, _ = strconv.ParseInt(proxyIDStr, 10, 64)
	}

	logs, err := database.GetCheckLogs(h.db, proxyID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    logs,
	})
}
