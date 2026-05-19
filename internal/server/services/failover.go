package services

import (
	"encoding/json"
	"log"
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

	// Track consecutive high-latency samples per leaf
	highLatencyCount := make(map[uint]int)

	for range ticker.C {
		var leaves []models.Node
		db.Where("backbone = ?", false).Find(&leaves)

		for _, leaf := range leaves {
			if !registry.IsOnline(leaf.ID) {
				continue
			}

			// Find this leaf's current backbone tunnel
			var tunnel models.Tunnel
			err := db.Where(
				"(left_node_id = ? OR right_node_id = ?) AND status = ?",
				leaf.ID, leaf.ID, "up",
			).First(&tunnel).Error
			if err != nil {
				// No tunnel — create one
				bestID := SelectBestBackbone(db, registry, 0)
				if bestID > 0 {
				var bb models.Node
				bb.ID = bestID
				go createLeafTunnel(db, registry, &leaf, &bb)
			}
				continue
			}

			currentBB := tunnel.LeftNodeID
			if currentBB == leaf.ID {
				currentBB = tunnel.RightNodeID
			}

			// Check if backbone is online
			if !registry.IsOnline(currentBB) {
				log.Printf("failover: backbone %d offline for leaf %d, switching", currentBB, leaf.ID)
				switchBackbone(db, registry, &leaf, currentBB)
				highLatencyCount[leaf.ID] = 0
				continue
			}

			// Check latency via probe result
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
				if highLatencyCount[leaf.ID] >= 3 { // 30s sustained
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
	// Find best alternative backbone
	newBB := SelectBestBackbone(db, registry, oldBB)
	if newBB == 0 {
		log.Printf("failover: no alternative backbone for leaf %d", leaf.ID)
		return
	}

	var newBBNode models.Node
	if db.First(&newBBNode, newBB).Error != nil {
		return
	}

	// Disconnect from old backbone
	registry.SendCmd(leaf.ID, "peer_disconnect", map[string]any{"peer_id": oldBB}, 3*time.Second)

	// Connect to new backbone
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

	// Mark old tunnel as down
	db.Model(&models.Tunnel{}).
		Where("(left_node_id = ? AND right_node_id = ?) OR (left_node_id = ? AND right_node_id = ?)",
			leaf.ID, oldBB, oldBB, leaf.ID).
		Update("status", "down")

	log.Printf("failover: leaf %d switched backbone %d -> %d", leaf.ID, oldBB, newBB)
}

// SelectBestBackbone picks the optimal backbone node.
// excludeID is a backbone to skip (e.g., the current failing one).
func SelectBestBackbone(db *gorm.DB, registry *ws.Registry, excludeID uint) uint {
	var backbones []models.Node
	db.Where("backbone = ? AND id != ?", true, excludeID).Find(&backbones)

	var bestID uint
	for _, bb := range backbones {
		if !registry.IsOnline(bb.ID) {
			continue
		}
		// Simple: pick first available. TODO: use probe RTT from nearby leaves.
		bestID = bb.ID
		break
	}
	return bestID
}
