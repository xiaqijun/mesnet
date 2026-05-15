package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mesnet/mesnet/internal/server/logwatch"
)

// GetLogs returns recent backend log entries.
func GetLogs() gin.HandlerFunc {
	return func(c *gin.Context) {
		level := c.Query("level")
		since := time.Now().Add(-24 * time.Hour)
		logs := logwatch.GetLogs(since, level)
		if logs == nil {
			logs = []logwatch.Entry{}
		}
		errors := logwatch.GetErrors()
		if errors == nil {
			errors = []logwatch.Entry{}
		}
		c.JSON(http.StatusOK, gin.H{
			"logs":       logs,
			"errors":     errors,
			"error_count": len(errors),
		})
	}
}
