package services

import (
	"encoding/json"
	"log"
	"net"
	"time"

	"github.com/mesnet/mesnet/internal/server/models"
	"github.com/mesnet/mesnet/internal/server/ws"
	"gorm.io/gorm"
)

// CheckAndFailover periodically checks leaf nodes and switches
// to a better backbone if the current one fails or has high latency.
func CheckAndFailover(db *gorm.DB, registry *ws.Registry) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	highLatencyCount := make(map[uint]int)

	for range ticker.C {
		var leaves []models.Node
		db.Where("backbone = ?", false).Find(&leaves)

		for _, leaf := range leaves {
			if !registry.IsOnline(leaf.ID) {
				continue
			}

			var tunnel models.Tunnel
			err := db.Where(
				"(left_node_id = ? OR right_node_id = ?) AND status = ?",
				leaf.ID, leaf.ID, "up",
			).First(&tunnel).Error
			if err != nil {
				continue
			}

			currentBB := tunnel.LeftNodeID
			if currentBB == leaf.ID {
				currentBB = tunnel.RightNodeID
			}

			if !registry.IsOnline(currentBB) {
				log.Printf("failover: backbone %d offline for leaf %d, switching", currentBB, leaf.ID)
				switchBackbone(db, registry, &leaf, currentBB)
				highLatencyCount[leaf.ID] = 0
				continue
			}

			result, err := registry.SendCmd(leaf.ID, "tunnel_test",
				map[string]any{"node_id": currentBB}, 5*time.Second)
			if err != nil {
				continue
			}

			var data struct {
				RTTMs float64 `json:"rtt_ms"`
			}
			if result.Data != nil {
				json.Unmarshal(result.Data, &data)
			}

			if data.RTTMs > 200 {
				highLatencyCount[leaf.ID]++
				if highLatencyCount[leaf.ID] >= 3 {
					log.Printf("failover: high latency %.0fms for leaf %d, switching", data.RTTMs, leaf.ID)
					switchBackbone(db, registry, &leaf, currentBB)
					highLatencyCount[leaf.ID] = 0
				}
			} else {
				highLatencyCount[leaf.ID] = 0
			}
		}
	}
}

func switchBackbone(db *gorm.DB, registry *ws.Registry, leaf *models.Node, oldBB uint) {
	newBB := SelectBestBackbone(db, registry, oldBB)
	if newBB == 0 {
		log.Printf("failover: no alternative backbone for leaf %d", leaf.ID)
		return
	}

	var newBBNode models.Node
	if db.First(&newBBNode, newBB).Error != nil {
		return
	}

	registry.SendCmd(leaf.ID, "peer_disconnect", map[string]any{"peer_id": oldBB}, 3*time.Second)

	registry.SendCmd(newBB, "peer_accept", map[string]any{
		"node_id": leaf.ID, "token": newBBNode.AgentToken, "public_key": leaf.PublicKey,
	}, 5*time.Second)

	_, err := registry.SendCmd(leaf.ID, "peer_connect", map[string]any{
		"node_id":    newBB,
		"peer_addr":  newBBNode.ListenAddr,
		"peer_token": newBBNode.AgentToken,
		"public_key": newBBNode.PublicKey,
	}, 10*time.Second)
	if err != nil {
		log.Printf("failover: peer_connect failed for leaf %d -> bb %d: %v", leaf.ID, newBB, err)
		return
	}

	db.Model(&models.Tunnel{}).
		Where("(left_node_id = ? AND right_node_id = ?) OR (left_node_id = ? AND right_node_id = ?)",
			leaf.ID, oldBB, oldBB, leaf.ID).
		Update("status", "down")

	log.Printf("failover: leaf %d switched backbone %d -> %d", leaf.ID, oldBB, newBB)
}

// SelectBestBackbone picks the backbone with the lowest TCP latency.
// excludeID is a backbone to skip.
func SelectBestBackbone(db *gorm.DB, registry *ws.Registry, excludeID uint) uint {
	var backbones []models.Node
	db.Where("backbone = ? AND id != ?", true, excludeID).Find(&backbones)

	var bestID uint
	var bestLatency time.Duration = 999 * time.Second

	for _, bb := range backbones {
		if !registry.IsOnline(bb.ID) || bb.ListenAddr == "" {
			continue
		}
		// TCP dial to measure latency (2s timeout)
		start := time.Now()
		conn, err := net.DialTimeout("tcp", bb.ListenAddr, 2*time.Second)
		if err != nil {
			continue
		}
		rtt := time.Since(start)
		conn.Close()

		if rtt < bestLatency {
			bestLatency = rtt
			bestID = bb.ID
		}
		log.Printf("backbone %s (%s) latency: %dms", bb.Name, bb.ListenAddr, rtt.Milliseconds())
	}

	if bestID > 0 {
		log.Printf("selected backbone %d (latency: %dms)", bestID, bestLatency.Milliseconds())
	}
	return bestID
}
