package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mesnet/mesnet/internal/server/models"
	"github.com/mesnet/mesnet/internal/server/ws"
	"gorm.io/gorm"
)

func GetNodeStats(db *gorm.DB, registry *ws.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		var totalRx, totalTx int64

		db.Model(&models.Tunnel{}).
			Where("left_node_id = ? OR right_node_id = ?", id, id).
			Select("COALESCE(SUM(rx_bytes), 0), COALESCE(SUM(tx_bytes), 0)").
			Row().Scan(&totalRx, &totalTx)

		var tunnelCount, upCount int64
		db.Model(&models.Tunnel{}).
			Where("left_node_id = ? OR right_node_id = ?", id, id).
			Count(&tunnelCount)
		db.Model(&models.Tunnel{}).
			Where("(left_node_id = ? OR right_node_id = ?) AND status = ?", id, id, "up").
			Count(&upCount)

		c.JSON(http.StatusOK, gin.H{
			"rx_bytes":    totalRx,
			"tx_bytes":    totalTx,
			"tunnel_count": tunnelCount,
			"up_count":    upCount,
		})
	}
}

func GetTunnelStats(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		var tunnel models.Tunnel
		if err := db.First(&tunnel, id).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "tunnel not found"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"rx_bytes":   tunnel.RxBytes,
			"tx_bytes":   tunnel.TxBytes,
			"latency_ms": tunnel.LatencyMs,
			"status":     tunnel.Status,
		})
	}
}

func GetTunnelStatsHistory(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		rangeParam := c.DefaultQuery("range", "1h")

		// Determine cutoff time
		cutoff := "datetime('now', '-1 hour')"
		switch rangeParam {
		case "6h":
			cutoff = "datetime('now', '-6 hours')"
		case "24h":
			cutoff = "datetime('now', '-24 hours')"
		case "7d":
			cutoff = "datetime('now', '-7 days')"
		}

		var snapshots []models.TrafficSnapshot
		db.Where("tunnel_id = ? AND created_at > "+cutoff, id).
			Order("created_at asc").
			Limit(1000).
			Find(&snapshots)

		c.JSON(http.StatusOK, gin.H{"snapshots": snapshots})
	}
}

func GetTotalTraffic(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var totalRx, totalTx int64
		db.Model(&models.Tunnel{}).
			Select("COALESCE(SUM(rx_bytes), 0), COALESCE(SUM(tx_bytes), 0)").
			Row().Scan(&totalRx, &totalTx)

		var topTunnels []models.Tunnel
		db.Preload("LeftNode").Preload("RightNode").
			Order("rx_bytes + tx_bytes desc").
			Limit(10).
			Find(&topTunnels)

		c.JSON(http.StatusOK, gin.H{
			"total_rx":    totalRx,
			"total_tx":    totalTx,
			"top_tunnels": topTunnels,
		})
	}
}
