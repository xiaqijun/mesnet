package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mesnet/mesnet/internal/server/models"
	"github.com/mesnet/mesnet/internal/server/ws"
	"gorm.io/gorm"
)

type createNodeBody struct {
	Name         string `json:"name" binding:"required"`
	Subnets      string `json:"subnets"`
	LocalSubnets string `json:"local_subnets"`
	Backbone     bool   `json:"backbone"`
}

func ListNodes(db *gorm.DB, registry *ws.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		var nodes []models.Node
		db.Order("created_at desc").Find(&nodes)

		// Inject online status from registry
		online := registry.ListOnline()
		onlineSet := make(map[uint]bool)
		for _, id := range online {
			onlineSet[id] = true
		}

		type nodeWithStatus struct {
			models.Node
			Online bool `json:"online"`
		}
		result := make([]nodeWithStatus, len(nodes))
		for i, n := range nodes {
			result[i] = nodeWithStatus{Node: n, Online: onlineSet[n.ID]}
		}

		c.JSON(http.StatusOK, gin.H{"nodes": result})
	}
}

func GetNode(db *gorm.DB, registry *ws.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		var node models.Node
		if err := db.First(&node, id).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
			return
		}

		var tunnels []models.Tunnel
		db.Where("left_node_id = ? OR right_node_id = ?", id, id).
			Preload("LeftNode").Preload("RightNode").
			Find(&tunnels)

		c.JSON(http.StatusOK, gin.H{
			"node":    node,
			"tunnels": tunnels,
			"online":  registry.IsOnline(uint(id)),
		})
	}
}

func CreateNode(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body createNodeBody
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		token := generateToken()

		node := models.Node{
			Name:       body.Name,
			Subnets:    body.Subnets,
			Backbone:   body.Backbone,
			AgentToken: token,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		db.Create(&node)

		// Auto-assign virtual IP
		var count int64
		db.Model(&models.Node{}).Count(&count)
		node.VirtualIP = "10.100.0." + strconv.Itoa(int(count))
		db.Save(&node)

		addAudit(db, "node.create", "node", node.ID, "Created node "+node.Name)
		c.JSON(http.StatusCreated, gin.H{"node": node})
	}
}

func UpdateNode(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		var node models.Node
		if err := db.First(&node, id).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
			return
		}

		var body createNodeBody
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		node.Name = body.Name
		node.Subnets = body.Subnets
		node.LocalSubnets = body.LocalSubnets
		node.Backbone = body.Backbone
		node.UpdatedAt = time.Now()

		db.Save(&node)
		addAudit(db, "node.update", "node", node.ID, "Updated node "+node.Name)
		c.JSON(http.StatusOK, gin.H{"node": node})
	}
}

func DeleteNode(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		var node models.Node
		if err := db.First(&node, id).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
			return
		}

		// Delete related tunnels
		db.Where("left_node_id = ? OR right_node_id = ?", id, id).Delete(&models.Tunnel{})
		db.Delete(&node)

		addAudit(db, "node.delete", "node", node.ID, "Deleted node "+node.Name)
		c.JSON(http.StatusOK, gin.H{"message": "deleted"})
	}
}

func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func addAudit(db *gorm.DB, action, targetType string, targetID uint, detail string) {
	db.Create(&models.AuditLog{
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Detail:     detail,
		CreatedAt:  time.Now(),
	})
}
