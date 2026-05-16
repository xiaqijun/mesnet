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

const maxMeshPeers = 3 // maximum backbone peers per node (not full mesh)

// AutoMesh connects a newly online node to the mesh network.
func AutoMesh(db *gorm.DB, registry *ws.Registry, nodeID uint) {
	var node models.Node
	if err := db.First(&node, nodeID).Error; err != nil {
		return
	}

	// Step 1: detect subnets from agent if not manually configured
	if node.Subnets == "" || node.Subnets == "-" {
		detectAndSaveSubnets(db, registry, &node)
	}

	// Step 2: find best backbone peers (max 3, sorted by latency or CPU*Memory)
	peers := findBestPeers(db, registry, nodeID)
	if len(peers) == 0 {
		log.Printf("mesh: no peers available for %s, creating self-tunnel", node.Name)
		createSelfTunnel(db, registry, &node)
		return
	}

	// Step 3: create tunnels based on node type
	if node.Backbone {
		for _, peer := range peers {
			createBackboneMesh(db, registry, &node, &peer)
		}
	} else {
		// Leaf connects to best 2 backbones
		best := findBestBackbone(peers)
		if best != nil {
			createLeafTunnel(db, registry, &node, best)
		}
		if len(peers) > 1 {
			second := findSecondBest(peers, best.ID)
			if second != nil {
				createLeafTunnel(db, registry, &node, second)
			}
		}
	}

	// Step 4: after tunnels up, push full routing table to all affected nodes
	time.Sleep(4 * time.Second)
	syncAllRoutes(db, registry)
}

// findBestPeers returns up to maxMeshPeers best backbone peers.
// Prefers peers with lower latency; falls back to CPU*Memory ranking.
func findBestPeers(db *gorm.DB, registry *ws.Registry, excludeID uint) []models.Node {
	var all []models.Node
	db.Where("id != ? AND backbone = ?", excludeID, true).Order("cpu * memory_mb DESC").Find(&all)

	// Filter to maxMeshPeers
	if len(all) > maxMeshPeers {
		all = all[:maxMeshPeers]
	}

	// Filter to online
	var online []models.Node
	for _, n := range all {
		if registry.IsOnline(n.ID) {
			online = append(online, n)
		}
	}
	return online
}

func findBestBackbone(nodes []models.Node) *models.Node {
	for i := range nodes {
		if nodes[i].Connected {
			return &nodes[i]
		}
	}
	return nil
}

func findSecondBest(nodes []models.Node, excludeID uint) *models.Node {
	for i := range nodes {
		if nodes[i].ID != excludeID && nodes[i].Connected {
			return &nodes[i]
		}
	}
	return nil
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

// createBackboneMesh connects two backbone nodes.
func createBackboneMesh(db *gorm.DB, registry *ws.Registry, a, b *models.Node) {
	var existing models.Tunnel
	err := db.Where(
		"(left_node_id = ? AND right_node_id = ?) OR (left_node_id = ? AND right_node_id = ?)",
		a.ID, b.ID, b.ID, a.ID,
	).First(&existing).Error
	if err == nil {
		return // tunnel already exists
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
		// Both sides connect to each other
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

		time.Sleep(2 * time.Second)

		// Push routes: A's subnets ↔ B, B's subnets ↔ A
		if a.Subnets != "" && a.Subnets != "-" {
			for _, s := range strings.Split(a.Subnets, ",") {
				s = strings.TrimSpace(s)
				if s != "" {
					registry.SendCmd(b.ID, "route_add", map[string]any{
						"subnet":   s,
						"node_id":  a.ID,
						"next_hop": a.ID,
					}, 5*time.Second)
				}
			}
		}
		if b.Subnets != "" && b.Subnets != "-" {
			for _, s := range strings.Split(b.Subnets, ",") {
				s = strings.TrimSpace(s)
				if s != "" {
					registry.SendCmd(a.ID, "route_add", map[string]any{
						"subnet":   s,
						"node_id":  b.ID,
						"next_hop": b.ID,
					}, 5*time.Second)
				}
			}
		}

		tunnel.Status = "up"
		tunnel.UpdatedAt = time.Now()
		db.Save(&tunnel)
	}()

	log.Printf("mesh: backbone tunnel %s ↔ %s", a.Name, b.Name)
}

// createLeafTunnel connects a leaf to a backbone.
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

		// Push backbone subnets to leaf
		if backbone.Subnets != "" && backbone.Subnets != "-" {
			for _, s := range strings.Split(backbone.Subnets, ",") {
				s = strings.TrimSpace(s)
				if s != "" {
					registry.SendCmd(leaf.ID, "route_add", map[string]any{
						"subnet":   s,
						"node_id":  backbone.ID,
						"next_hop": backbone.ID,
					}, 5*time.Second)
				}
			}
		}
		// Push leaf subnets to backbone
		if leaf.Subnets != "" && leaf.Subnets != "-" {
			for _, s := range strings.Split(leaf.Subnets, ",") {
				s = strings.TrimSpace(s)
				if s != "" {
					registry.SendCmd(backbone.ID, "route_add", map[string]any{
						"subnet":   s,
						"node_id":  leaf.ID,
						"next_hop": leaf.ID,
					}, 5*time.Second)
				}
			}
		}

		tunnel.Status = "up"
		tunnel.UpdatedAt = time.Now()
		db.Save(&tunnel)
	}()

	log.Printf("mesh: leaf tunnel %s → %s", leaf.Name, backbone.Name)
}

// syncAllRoutes pushes the full routing table to every online node.
// For multi-hop routes, computes the correct next_hop based on tunnel topology.
func syncAllRoutes(db *gorm.DB, registry *ws.Registry) {
	var nodes []models.Node
	db.Find(&nodes)

	// Collect all subnets with their owner node IDs
	type subnetInfo struct {
		subnet string
		nodeID uint
	}
	var allSubnets []subnetInfo
	for _, n := range nodes {
		if n.Subnets == "" || n.Subnets == "-" {
			continue
		}
		for _, s := range strings.Split(n.Subnets, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				allSubnets = append(allSubnets, subnetInfo{s, n.ID})
			}
		}
	}

	// Get active tunnels for computing next-hops
	var tunnels []models.Tunnel
	db.Where("status = ?", "up").Find(&tunnels)

	// Build adjacency: nodeID → direct peers
	adj := make(map[uint]map[uint]bool)
	for _, t := range tunnels {
		if adj[t.LeftNodeID] == nil {
			adj[t.LeftNodeID] = make(map[uint]bool)
		}
		if adj[t.RightNodeID] == nil {
			adj[t.RightNodeID] = make(map[uint]bool)
		}
		adj[t.LeftNodeID][t.RightNodeID] = true
		adj[t.RightNodeID][t.LeftNodeID] = true
	}

	// For each online node, push routes with correct next-hop
	for _, node := range nodes {
		if !registry.IsOnline(node.ID) {
			continue
		}
		for _, si := range allSubnets {
			if si.nodeID == node.ID {
				continue // don't route to self
			}
			// Find next-hop: if directly connected, use si.nodeID; otherwise find relay
			nextHop := findNextHop(node.ID, si.nodeID, adj)
			if nextHop == 0 {
				continue
			}
			registry.SendCmd(node.ID, "route_add", map[string]any{
				"subnet":   si.subnet,
				"node_id":  si.nodeID,
				"next_hop": nextHop,
			}, 5*time.Second)
		}
	}

	log.Printf("mesh: synced %d subnets to all nodes", len(allSubnets))
}

// findNextHop finds the direct peer to forward traffic to reach the target node.
// Uses BFS for shortest path through the mesh.
func findNextHop(from, to uint, adj map[uint]map[uint]bool) uint {
	if adj[from][to] {
		return to // directly connected
	}
	// BFS
	visited := map[uint]bool{from: true}
	type step struct {
		node uint
		next uint // first hop from source
	}
	queue := []step{}
	for peer := range adj[from] {
		queue = append(queue, step{peer, peer})
	}
	for len(queue) > 0 {
		s := queue[0]
		queue = queue[1:]
		if s.node == to {
			return s.next
		}
		if visited[s.node] {
			continue
		}
		visited[s.node] = true
		for peer := range adj[s.node] {
			if !visited[peer] {
				queue = append(queue, step{peer, s.next})
			}
		}
	}
	return 0
}

func createSelfTunnel(db *gorm.DB, registry *ws.Registry, node *models.Node) {
	if node.Subnets == "" || node.Subnets == "-" {
		return
	}
	if registry.IsOnline(node.ID) {
		registry.SendCmd(node.ID, "tun_setup", map[string]any{
			"ip": node.VirtualIP,
		}, 5*time.Second)
	}
}
