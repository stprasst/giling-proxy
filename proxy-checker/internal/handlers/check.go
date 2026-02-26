package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"proxy-checker/internal/scheduler"
)

// CheckHandler handles check trigger routes
type CheckHandler struct {
	scheduler *scheduler.Scheduler
}

// NewCheckHandler creates a new check handler
func NewCheckHandler(scheduler *scheduler.Scheduler) *CheckHandler {
	return &CheckHandler{scheduler: scheduler}
}

// Trigger manually triggers a proxy check
func (h *CheckHandler) Trigger(c *gin.Context) {
	h.scheduler.TriggerCheck()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Check triggered",
	})
}

// Status returns the scheduler status including next check time
func (h *CheckHandler) Status(c *gin.Context) {
	status := h.scheduler.GetStatus()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    status,
	})
}
