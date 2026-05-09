package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mesnet/mesnet/internal/server/models"
	"github.com/mesnet/mesnet/internal/server/ws"
	"gorm.io/gorm"
)

const CurrentAgentVersion = "v1.0.0"

// GetAgentVersions returns version info for all nodes.
func GetAgentVersions(db *gorm.DB, registry *ws.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		var nodes []models.Node
		db.Order("name asc").Find(&nodes)

		type versionInfo struct {
			ID       uint   `json:"id"`
			Name     string `json:"name"`
			Version  string `json:"version"`
			Online   bool   `json:"online"`
			Latest   bool   `json:"latest"`
		}

		result := make([]versionInfo, 0, len(nodes))
		for _, n := range nodes {
			result = append(result, versionInfo{
				ID:      n.ID,
				Name:    n.Name,
				Version: n.AgentVersion,
				Online:  registry.IsOnline(n.ID),
				Latest:  n.AgentVersion == "" || n.AgentVersion == CurrentAgentVersion,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"current_version": CurrentAgentVersion,
			"nodes":           result,
		})
	}
}

// UpdateAgent triggers a self-update on a specific agent.
func UpdateAgent(registry *ws.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		type req struct {
			NodeID uint `json:"node_id" binding:"required"`
		}
		var body req
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if !registry.IsOnline(body.NodeID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "agent offline"})
			return
		}

		_, err := registry.SendCmd(body.NodeID, "agent_update", nil, 30*time.Second)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "updating"})
	}
}

// UpdateAllAgents triggers self-update on all online agents.
func UpdateAllAgents(registry *ws.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		online := registry.ListOnline()
		updated := 0
		for _, nodeID := range online {
			registry.SendCmd(nodeID, "agent_update", nil, 30*time.Second)
			updated++
		}

		c.JSON(http.StatusOK, gin.H{
			"updated": updated,
			"total":   len(online),
		})
	}
}
