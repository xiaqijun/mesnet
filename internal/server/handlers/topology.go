package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mesnet/mesnet/internal/server/models"
	"github.com/mesnet/mesnet/internal/server/ws"
	"gorm.io/gorm"
)

type topologyEdge struct {
	ID        uint   `json:"id"`
	LeftID    uint   `json:"left_id"`
	RightID   uint   `json:"right_id"`
	Status    string `json:"status"`
	RxBytes   int64  `json:"rx_bytes"`
	TxBytes   int64  `json:"tx_bytes"`
	LatencyMs float64 `json:"latency_ms"`
}

type topologyNode struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	VirtualIP string `json:"virtual_ip"`
	Online    bool   `json:"online"`
	Backbone  bool   `json:"backbone"`
}

func GetTopology(db *gorm.DB, registry *ws.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		var nodes []models.Node
		db.Find(&nodes)

		var tunnels []models.Tunnel
		db.Find(&tunnels)

		onlineSet := make(map[uint]bool)
		for _, id := range registry.ListOnline() {
			onlineSet[id] = true
		}

		graphNodes := make([]topologyNode, len(nodes))
		for i, n := range nodes {
			graphNodes[i] = topologyNode{
				ID:        n.ID,
				Name:      n.Name,
				VirtualIP: n.VirtualIP,
				Online:    onlineSet[n.ID],
				Backbone:  n.Backbone,
			}
		}

		graphEdges := make([]topologyEdge, len(tunnels))
		for i, t := range tunnels {
			graphEdges[i] = topologyEdge{
				ID:        t.ID,
				LeftID:    t.LeftNodeID,
				RightID:   t.RightNodeID,
				Status:    t.Status,
				RxBytes:   t.RxBytes,
				TxBytes:   t.TxBytes,
				LatencyMs: t.LatencyMs,
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"nodes": graphNodes,
			"edges": graphEdges,
		})
	}
}
