package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mesnet/mesnet/internal/server/services"
)

// TestSSH tests SSH connectivity and returns the result.
func TestSSH() gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			Host     string `json:"host" binding:"required"`
			Username string `json:"username" binding:"required"`
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		ssh := services.NewSSHClient(body.Host, 22, body.Username, body.Password, nil)
		out, err := ssh.TestConnection()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "output": out})
	}
}
