package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mesnet/mesnet/internal/server/models"
	"github.com/mesnet/mesnet/internal/server/services"
	"gorm.io/gorm"
)

// AutoDeployNode tries SSH deployment for an existing node, falls back to script.
func AutoDeployNode(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		var node models.Node
		if err := db.First(&node, id).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
			return
		}

		var body struct {
			Host     string `json:"host"`
			Username string `json:"username"`
			Password string `json:"password"`
		}
		c.ShouldBindJSON(&body)

		host := body.Host
		if host == "" {
			host = node.Host
		}

		serverAddr := c.Request.Host

		if body.Username != "" {
			ssh := services.NewSSHClient(host, 22, body.Username, body.Password, nil)
			if _, err := ssh.TestConnection(); err == nil {
				steps, err := ssh.DeployAgent(serverAddr, node.AgentToken, node.Name, node.Backbone)
				if err == nil {
					node.ListenAddr = fmt.Sprintf("%s:443", host)
					node.UpdatedAt = time.Now()
					db.Save(&node)
					c.JSON(http.StatusOK, gin.H{"deployed": true, "host": host, "steps": steps})
					return
				}
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"deployed": false,
			"script":   onelinerDeploy(node.AgentToken, node.Name, node.Backbone),
			"error":    "SSH 不通，请手动执行",
		})
	}
}
