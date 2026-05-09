package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mesnet/mesnet/internal/server/models"
	"github.com/mesnet/mesnet/internal/server/ws"
	"gorm.io/gorm"
)

func GetDashboardStats(db *gorm.DB, registry *ws.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		var nodeCount, tunnelCount, onlineCount int64
		var totalRx, totalTx int64

		db.Model(&models.Node{}).Count(&nodeCount)
		db.Model(&models.Tunnel{}).Count(&tunnelCount)
		db.Model(&models.Tunnel{}).Where("status = ?", "up").Count(&onlineCount)
		db.Model(&models.Tunnel{}).Select("COALESCE(SUM(rx_bytes), 0), COALESCE(SUM(tx_bytes), 0)").Row().
			Scan(&totalRx, &totalTx)

		onlineAgentCount := len(registry.ListOnline())

		c.JSON(http.StatusOK, gin.H{
			"nodes":   nodeCount,
			"tunnels": tunnelCount,
			"online_tunnels": onlineCount,
			"online_agents":  onlineAgentCount,
			"total_rx": totalRx,
			"total_tx": totalTx,
		})
	}
}
