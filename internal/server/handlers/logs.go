package handlers

import (
	"net/http"
	"time"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/mesnet/mesnet/internal/server/logwatch"
)

// GetLogs returns recent backend log entries.
func GetLogs() gin.HandlerFunc {
	return func(c *gin.Context) {
		level := c.Query("level")
		source := c.Query("source")
		limit := 200
		if l := c.Query("limit"); l != "" {
			if n, err := parseInt(l); err == nil && n > 0 && n <= 2000 {
				limit = n
			}
		}
		since := time.Now().Add(-24 * time.Hour)
		if s := c.Query("since"); s != "" {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				since = t
			}
		}
		logs := logwatch.GetLogs(since, level, source, limit)
		if logs == nil {
			logs = []logwatch.Entry{}
		}
		errors := logwatch.GetErrors()
		if errors == nil {
			errors = []logwatch.Entry{}
		}
		c.JSON(http.StatusOK, gin.H{
			"logs":        logs,
			"errors":      errors,
			"error_count": len(errors),
		})
	}
}

func parseInt(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a number")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
