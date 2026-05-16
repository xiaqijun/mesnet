package services

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/mesnet/mesnet/internal/server/models"
	"github.com/mesnet/mesnet/internal/server/ws"
	"gorm.io/gorm"
)

// AutoMesh connects a newly online node to the mesh network.
func AutoMesh(db *gorm.DB, registry *ws.Registry, nodeID uint) {
	var node models.Node
	if err := db.First(&node, nodeID).Error; err != nil {
		return
	}

	// Auto-detect subnets from agent if not manually configured
	if node.Subnets == "" || node.Subnets == "-" {
		detectAndSaveSubnets(db, registry, &node)
	}

	// Find all other online backbone nodes
	var backbones []models.Node
	db.Where("id != ? AND backbone = ?", nodeID, true).Find(&backbones)

	if len(backbones) == 0 {
		log.Printf("mesh: no backbone nodes available for %s", node.Name)
		createSelfTunnel(db, registry, &node)
		return
	}

	if node.Backbone {
		// Backbone ↔ Backbone: full mesh, both sides initiate
		for _, peer := range backbones {
			createBackboneMesh(db, registry, &node, &peer)
		}
	} else {
		// Leaf → Backbone: leaf connects to best backbone(s). Backbone just accepts.
		best := selectBestBackbone(&backbones)
		if best != nil {
			createLeafTunnel(db, registry, &node, best)
		}
		if len(backbones) > 1 {
			second := selectSecondBest(&backbones, best.ID)
			if second != nil {
				createLeafTunnel(db, registry, &node, second)
			}
		}
	}
}

// detectAndSaveSubnets sends subnet_detect to agent and saves result to DB.
func detectAndSaveSubnets(db *gorm.DB, registry *ws.Registry, node *models.Node) {
	result, err := registry.SendCmd(node.ID, "subnet_detect", nil, 10*time.Second)
	if err != nil {
		log.Printf("mesh: subnet detect failed for %s: %v", node.Name, err)
		return
	}

	var data struct {
		Subnets []string `json:"subnets"`
	}
	if result.Data != nil {
		if err := json.Unmarshal(result.Data, &data); err != nil {
			log.Printf("mesh: subnet detect parse failed for %s: %v", node.Name, err)
			return
		}
	}

	if len(data.Subnets) > 0 {
		subnetsStr := strings.Join(data.Subnets, ",")
		db.Model(node).Update("subnets", subnetsStr)
		node.Subnets = subnetsStr
		log.Printf("mesh: auto-detected subnets for %s: %s", node.Name, subnetsStr)
	}
}

// createSelfTunnel creates a tunnel to itself when no other backbone exists (single node).
func createSelfTunnel(db *gorm.DB, registry *ws.Registry, node *models.Node) {
	if node.Subnets == "" {
		return
	}
	if !registry.IsOnline(node.ID) {
		registry.SendCmd(node.ID, "tun_setup", map[string]any{
			"ip": node.VirtualIP,
		}, 5*time.Second)
	}
}

// createBackboneMesh connects two backbone nodes bidirectionally.
func createBackboneMesh(db *gorm.DB, registry *ws.Registry, a, b *models.Node) {
	var existing models.Tunnel
	err := db.Where(
		"(left_node_id = ? AND right_node_id = ?) OR (left_node_id = ? AND right_node_id = ?)",
		a.ID, b.ID, b.ID, a.ID,
	).First(&existing).Error
	if err == nil {
		return
	}

	tunnel := models.Tunnel{
		Name:        fmt.Sprintf("%s ↔ %s", a.Name, b.Name),
		LeftNodeID:  a.ID,
		RightNodeID: b.ID,
		LeftSubnet:  a.Subnets,
		RightSubnet: b.Subnets,
		Status:      "down",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	db.Create(&tunnel)

	go func() {
		// Both backbones connect to each other
		registry.SendCmd(a.ID, "peer_connect", map[string]any{
			"node_id":    b.ID,
			"peer_addr":  b.ListenAddr,
			"peer_token": b.AgentToken,
			"tunnel_id":  tunnel.ID,
		}, 10*time.Second)

		registry.SendCmd(b.ID, "peer_connect", map[string]any{
			"node_id":    a.ID,
			"peer_addr":  a.ListenAddr,
			"peer_token": a.AgentToken,
			"tunnel_id":  tunnel.ID,
		}, 10*time.Second)

		time.Sleep(3 * time.Second)

		registry.SendCmd(a.ID, "route_add", map[string]any{
			"subnet":    b.Subnets,
			"tunnel_id": tunnel.ID,
		}, 5*time.Second)
		registry.SendCmd(b.ID, "route_add", map[string]any{
			"subnet":    a.Subnets,
			"tunnel_id": tunnel.ID,
		}, 5*time.Second)

		tunnel.Status = "up"
		tunnel.UpdatedAt = time.Now()
		db.Save(&tunnel)
	}()

	log.Printf("mesh: backbone tunnel %s ↔ %s", a.Name, b.Name)
}

// createLeafTunnel connects a leaf to a backbone.
// Only the leaf dials out; backbone accepts the incoming connection.
func createLeafTunnel(db *gorm.DB, registry *ws.Registry, leaf, backbone *models.Node) {
	var existing models.Tunnel
	err := db.Where(
		"(left_node_id = ? AND right_node_id = ?) OR (left_node_id = ? AND right_node_id = ?)",
		leaf.ID, backbone.ID, backbone.ID, leaf.ID,
	).First(&existing).Error
	if err == nil {
		return
	}

	tunnel := models.Tunnel{
		Name:        fmt.Sprintf("%s → %s", leaf.Name, backbone.Name),
		LeftNodeID:  leaf.ID,
		RightNodeID: backbone.ID,
		LeftSubnet:  leaf.Subnets,
		RightSubnet: backbone.Subnets,
		Status:      "down",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	db.Create(&tunnel)

	go func() {
		// Only leaf connects outbound to backbone
		registry.SendCmd(leaf.ID, "peer_connect", map[string]any{
			"node_id":    backbone.ID,
			"peer_addr":  backbone.ListenAddr,
			"peer_token": backbone.AgentToken,
			"tunnel_id":  tunnel.ID,
		}, 10*time.Second)

		time.Sleep(2 * time.Second)

		// Push routes to both sides
		registry.SendCmd(leaf.ID, "route_add", map[string]any{
			"subnet":    backbone.Subnets,
			"tunnel_id": tunnel.ID,
		}, 5*time.Second)
		registry.SendCmd(backbone.ID, "route_add", map[string]any{
			"subnet":    leaf.Subnets,
			"tunnel_id": tunnel.ID,
		}, 5*time.Second)

		tunnel.Status = "up"
		tunnel.UpdatedAt = time.Now()
		db.Save(&tunnel)
	}()

	log.Printf("mesh: leaf tunnel %s → %s", leaf.Name, backbone.Name)
}

func selectBestBackbone(nodes *[]models.Node) *models.Node {
	if len(*nodes) == 0 {
		return nil
	}
	best := &(*nodes)[0]
	for i := range *nodes {
		n := &(*nodes)[i]
		if n.Connected && n.CPU*n.MemoryMB > best.CPU*best.MemoryMB {
			best = n
		}
	}
	return best
}

func selectSecondBest(nodes *[]models.Node, excludeID uint) *models.Node {
	for i := range *nodes {
		n := &(*nodes)[i]
		if n.ID != excludeID && n.Connected {
			return n
		}
	}
	return nil
}
