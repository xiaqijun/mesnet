package services

import (
	"encoding/json"
	"log"
	"sort"
	"time"

	"github.com/mesnet/mesnet/internal/server/models"
	"github.com/mesnet/mesnet/internal/server/ws"
	"gorm.io/gorm"
)

// CheckAndFailover periodically checks leaf nodes and switches
// to a better backbone if the current one fails or has high latency.
func CheckAndFailover(db *gorm.DB, registry *ws.Registry) {
	ticker := time.NewTicker(5 * time.Second)
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
				// No tunnel — trigger AutoMesh to create one
				log.Printf("failover: leaf %d has no tunnel, triggering AutoMesh", leaf.ID)
				go AutoMesh(db, registry, leaf.ID)
				continue
			}

			currentBB := tunnel.LeftNodeID
			if currentBB == leaf.ID {
				currentBB = tunnel.RightNodeID
			}

			if !registry.IsOnline(currentBB) {
				log.Printf("failover: backbone %d offline for leaf %d, switching", currentBB, leaf.ID)
				SwitchBackbone(db, registry, &leaf, currentBB)
				highLatencyCount[leaf.ID] = 0
				continue
			}

			result, err := registry.SendCmd(leaf.ID, "tunnel_test",
				map[string]any{"node_id": currentBB}, 2*time.Second)
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
					SwitchBackbone(db, registry, &leaf, currentBB)
					highLatencyCount[leaf.ID] = 0
				}
			} else {
				highLatencyCount[leaf.ID] = 0
			}
		}
	}
}

func SwitchBackbone(db *gorm.DB, registry *ws.Registry, leaf *models.Node, oldBB uint) {
	newBB := SelectBestBackbone(db, registry, leaf.ID, oldBB)
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

// SelectBestBackbone asks the leaf agent to probe all backbones and
// returns the one with the lowest TCP latency. If leafID is 0, falls back
// to returning first available backbone.
func SelectBestBackbone(db *gorm.DB, registry *ws.Registry, leafID uint, excludeID uint) uint {
	var backbones []models.Node
	db.Where("backbone = ? AND id != ?", true, excludeID).Find(&backbones)

	type probeAddr struct {
		ID   uint   `json:"id"`
		Addr string `json:"addr"`
	}
	var addrs []probeAddr
	for _, bb := range backbones {
		if registry.IsOnline(bb.ID) && bb.ListenAddr != "" {
			addrs = append(addrs, probeAddr{ID: bb.ID, Addr: bb.ListenAddr})
		}
	}
	if len(addrs) == 0 {
		return 0
	}

	// If leaf is online, ask it to probe. Otherwise fall back.
	if leafID > 0 && registry.IsOnline(leafID) {
		result, err := registry.SendCmd(leafID, "backbone_probe",
			map[string]any{"addrs": addrs}, 4*time.Second)
		if err == nil && result.Data != nil {
			var data struct {
				Results []struct {
					ID    uint   `json:"id"`
					Addr  string `json:"addr"`
					RTTMS int64  `json:"rtt_ms"`
				} `json:"results"`
			}
			json.Unmarshal(result.Data, &data)
			sort.Slice(data.Results, func(i, j int) bool {
				return data.Results[i].RTTMS >= 0 && data.Results[i].RTTMS < data.Results[j].RTTMS
			})
			for _, r := range data.Results {
				if r.RTTMS >= 0 {
					log.Printf("backbone %d latency from leaf %d: %dms", r.ID, leafID, r.RTTMS)
					return r.ID
				}
			}
		}
	}

	return addrs[0].ID
}
