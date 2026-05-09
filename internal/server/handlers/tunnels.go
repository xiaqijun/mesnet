package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mesnet/mesnet/internal/server/models"
	"github.com/mesnet/mesnet/internal/server/ws"
	"gorm.io/gorm"
)

type createTunnelBody struct {
	Name         string `json:"name" binding:"required"`
	LeftNodeID   uint   `json:"left_node_id" binding:"required"`
	RightNodeID  uint   `json:"right_node_id" binding:"required"`
	LeftSubnet   string `json:"left_subnet"`
	RightSubnet  string `json:"right_subnet"`
}

func ListTunnels(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var tunnels []models.Tunnel
		db.Preload("LeftNode").Preload("RightNode").
			Order("created_at desc").Find(&tunnels)
		c.JSON(http.StatusOK, gin.H{"tunnels": tunnels})
	}
}

func GetTunnel(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		var tunnel models.Tunnel
		if err := db.Preload("LeftNode").Preload("RightNode").First(&tunnel, id).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "tunnel not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"tunnel": tunnel})
	}
}

func CreateTunnel(db *gorm.DB, registry *ws.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body createTunnelBody
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Validate nodes exist
		var leftNode, rightNode models.Node
		if err := db.First(&leftNode, body.LeftNodeID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "left node not found"})
			return
		}
		if err := db.First(&rightNode, body.RightNodeID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "right node not found"})
			return
		}

		tunnel := models.Tunnel{
			Name:        body.Name,
			LeftNodeID:  body.LeftNodeID,
			RightNodeID: body.RightNodeID,
			LeftSubnet:  body.LeftSubnet,
			RightSubnet: body.RightSubnet,
			Status:      "down",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		db.Create(&tunnel)

		// Orchestrate agent-to-agent connection
		if registry.IsOnline(body.LeftNodeID) {
			registry.SendCmd(body.LeftNodeID, "peer_connect", gin.H{
				"node_id":    body.RightNodeID,
				"peer_addr":  rightNode.ListenAddr,
				"peer_token": rightNode.AgentToken,
				"tunnel_id":  tunnel.ID,
			}, 10*time.Second)
		}

		if registry.IsOnline(body.RightNodeID) {
			registry.SendCmd(body.RightNodeID, "peer_connect", gin.H{
				"node_id":    body.LeftNodeID,
				"peer_addr":  leftNode.ListenAddr,
				"peer_token": leftNode.AgentToken,
				"tunnel_id":  tunnel.ID,
			}, 10*time.Second)
		}

		addAudit(db, "tunnel.create", "tunnel", tunnel.ID, "Created tunnel "+tunnel.Name)
		c.JSON(http.StatusCreated, gin.H{"tunnel": tunnel})
	}
}

func DeleteTunnel(db *gorm.DB, registry *ws.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		var tunnel models.Tunnel
		if err := db.First(&tunnel, id).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "tunnel not found"})
			return
		}

		// Notify agents to disconnect
		registry.SendCmd(tunnel.LeftNodeID, "peer_disconnect", gin.H{"tunnel_id": tunnel.ID}, 5*time.Second)
		registry.SendCmd(tunnel.RightNodeID, "peer_disconnect", gin.H{"tunnel_id": tunnel.ID}, 5*time.Second)

		db.Delete(&tunnel)
		addAudit(db, "tunnel.delete", "tunnel", tunnel.ID, "Deleted tunnel "+tunnel.Name)
		c.JSON(http.StatusOK, gin.H{"message": "deleted"})
	}
}

func TunnelUp(db *gorm.DB, registry *ws.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		var tunnel models.Tunnel
		if err := db.First(&tunnel, id).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "tunnel not found"})
			return
		}

		// Tell agents to add routes
		registry.SendCmd(tunnel.LeftNodeID, "route_add", gin.H{
			"subnet": tunnel.RightSubnet,
			"tunnel_id": tunnel.ID,
		}, 10*time.Second)

		registry.SendCmd(tunnel.RightNodeID, "route_add", gin.H{
			"subnet": tunnel.LeftSubnet,
			"tunnel_id": tunnel.ID,
		}, 10*time.Second)

		tunnel.Status = "up"
		tunnel.UpdatedAt = time.Now()
		db.Save(&tunnel)

		addAudit(db, "tunnel.up", "tunnel", tunnel.ID, "Started tunnel "+tunnel.Name)
		c.JSON(http.StatusOK, gin.H{"message": "tunnel started"})
	}
}

func TunnelDown(db *gorm.DB, registry *ws.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		var tunnel models.Tunnel
		if err := db.First(&tunnel, id).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "tunnel not found"})
			return
		}

		registry.SendCmd(tunnel.LeftNodeID, "route_del", gin.H{
			"subnet": tunnel.RightSubnet,
		}, 5*time.Second)
		registry.SendCmd(tunnel.RightNodeID, "route_del", gin.H{
			"subnet": tunnel.LeftSubnet,
		}, 5*time.Second)

		tunnel.Status = "down"
		tunnel.UpdatedAt = time.Now()
		db.Save(&tunnel)

		addAudit(db, "tunnel.down", "tunnel", tunnel.ID, "Stopped tunnel "+tunnel.Name)
		c.JSON(http.StatusOK, gin.H{"message": "tunnel stopped"})
	}
}
