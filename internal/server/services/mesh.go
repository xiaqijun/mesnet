package services

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sort"
	"sync"
	"strings"
	"time"

	"github.com/mesnet/mesnet/internal/server/logwatch"
	"github.com/mesnet/mesnet/internal/server/models"
	"github.com/mesnet/mesnet/internal/server/ws"
	"gorm.io/gorm"
)

var autoMeshMu sync.Mutex
const maxMeshPeers = 3

// Weighted scoring constants for backbone selection
const (
	weightLatency = 0.5
	weightCPU     = 0.25
	weightMemory  = 0.25
	memRefMB      = 8192  // memory score reference point (8 GB)
	latencyRefMs  = 100.0 // latency score midpoint: 100ms → 0.5
)

// scoreBackbone computes a weighted score (0.0–1.0) for a backbone candidate.
// Pass latencyMs=-1 to skip latency (re-normalizes over CPU+memory only).
func scoreBackbone(cpu int, memoryMB int, latencyMs int64) float64 {
	cpuScore := 1.0 - float64(cpu)/100.0
	if cpuScore < 0 {
		cpuScore = 0
	}

	memScore := float64(memoryMB) / memRefMB
	if memScore > 1.0 {
		memScore = 1.0
	}

	if latencyMs < 0 {
		denom := weightCPU + weightMemory
		if denom == 0 {
			return 0
		}
		return (weightCPU*cpuScore + weightMemory*memScore) / denom
	}

	latencyScore := latencyRefMs / (float64(latencyMs) + latencyRefMs)
	return weightLatency*latencyScore + weightCPU*cpuScore + weightMemory*memScore
}

func AutoMesh(db *gorm.DB, registry *ws.Registry, nodeID uint) {
	autoMeshMu.Lock()
	defer autoMeshMu.Unlock()
	var node models.Node
	if err := db.First(&node, nodeID).Error; err != nil {
		return
	}

	if node.VirtualIP != "" {
		registry.SendCmd(nodeID, "tun_setup", map[string]any{"ip": node.VirtualIP}, 5*time.Second)
	}

	if node.LocalSubnets == "" || node.Subnets == "" {
		DetectAndSaveSubnets(db, registry, &node)
	}

	peers := findBestPeers(db, registry, nodeID)
	if len(peers) == 0 {
		log.Printf("mesh: no peers for %s", node.Name)
		createSelfTunnel(db, registry, &node)
		return
	}

	if node.Backbone {
		for _, peer := range peers {
			createBackboneMesh(db, registry, &node, &peer)
		}
	} else {
		// Check if leaf already has an active tunnel
		var existingTunnel models.Tunnel
		if db.Where(
			"(left_node_id = ? OR right_node_id = ?) AND status = ?",
			node.ID, node.ID, "up",
		).First(&existingTunnel).Error != nil {
			// No active tunnel — create one
			bestID := SelectBestBackbone(db, registry, node.ID, 0)
			if bestID > 0 {
				var best models.Node
				if db.First(&best, bestID).Error == nil {
					createLeafTunnel(db, registry, &node, &best)
				}
			}
		}
	}

	time.Sleep(4 * time.Second)
	syncAllRoutes(db, registry)
}

func createBackboneMesh(db *gorm.DB, registry *ws.Registry, a, b *models.Node) {
	var existing models.Tunnel
	err := db.Where(
		"(left_node_id = ? AND right_node_id = ?) OR (left_node_id = ? AND right_node_id = ?)",
		a.ID, b.ID, b.ID, a.ID,
	).First(&existing).Error

	tunnel := existing
	if err != nil {
		tunnel = models.Tunnel{
			Name:        fmt.Sprintf("%s <-> %s", a.Name, b.Name),
			LeftNodeID:  a.ID,
			RightNodeID: b.ID,
			LeftSubnet:  a.Subnets,
			RightSubnet: b.Subnets,
			Status:      "down",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		db.Create(&tunnel)
	}

	go func() {
		// Pre-register public keys and expected tokens on both sides first,
		// so incoming handshake frames can be processed regardless of
		// which side dials first.
		registry.SendCmd(a.ID, "peer_accept", map[string]any{
			"node_id": b.ID, "token": a.AgentToken, "public_key": b.PublicKey,
		}, 5*time.Second)
		registry.SendCmd(b.ID, "peer_accept", map[string]any{
			"node_id": a.ID, "token": b.AgentToken, "public_key": a.PublicKey,
		}, 5*time.Second)

		_, errA := registry.SendCmd(a.ID, "peer_connect", map[string]any{
			"node_id": b.ID, "peer_addr": b.ListenAddr, "peer_token": b.AgentToken, "tunnel_id": tunnel.ID, "public_key": b.PublicKey,
		}, 10*time.Second)
		_, errB := registry.SendCmd(b.ID, "peer_connect", map[string]any{
			"node_id": a.ID, "peer_addr": a.ListenAddr, "peer_token": a.AgentToken, "tunnel_id": tunnel.ID, "public_key": a.PublicKey,
		}, 10*time.Second)

		if errA != nil || errB != nil {
			logwatch.Warn("mesh", fmt.Sprintf("peer_connect failed %s<->%s: A=%v B=%v", a.Name, b.Name, errA, errB))
			return
		}

		time.Sleep(2 * time.Second)

		for _, s := range splitSubnets(a.Subnets) {
			registry.SendCmd(b.ID, "route_add", map[string]any{
				"subnet": s, "node_id": a.ID, "next_hop": a.ID,
			}, 5*time.Second)
		}
		if a.VirtualIP != "" {
			registry.SendCmd(b.ID, "route_add", map[string]any{
				"subnet": a.VirtualIP + "/32", "node_id": a.ID, "next_hop": a.ID,
			}, 5*time.Second)
		}
		for _, s := range splitSubnets(b.Subnets) {
			registry.SendCmd(a.ID, "route_add", map[string]any{
				"subnet": s, "node_id": b.ID, "next_hop": b.ID,
			}, 5*time.Second)
		}
		if b.VirtualIP != "" {
			registry.SendCmd(a.ID, "route_add", map[string]any{
				"subnet": b.VirtualIP + "/32", "node_id": b.ID, "next_hop": b.ID,
			}, 5*time.Second)
		}

		tunnel.Status = "up"
		tunnel.UpdatedAt = time.Now()
		db.Save(&tunnel)
		logwatch.Info("mesh", fmt.Sprintf("backbone tunnel up %s <-> %s", a.Name, b.Name))
	}()
}

func splitSubnets(s string) []string {
	if s == "" || s == "-" {
		return nil
	}
	var r []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			r = append(r, p)
		}
	}
	return r
}

func createLeafTunnel(db *gorm.DB, registry *ws.Registry, leaf, backbone *models.Node) {
	var existing models.Tunnel
	err := db.Where(
		"(left_node_id = ? AND right_node_id = ?) OR (left_node_id = ? AND right_node_id = ?)",
		leaf.ID, backbone.ID, backbone.ID, leaf.ID,
	).First(&existing).Error

	tunnel := existing
	if err != nil {
		tunnel = models.Tunnel{
		Name:        fmt.Sprintf("%s -> %s", leaf.Name, backbone.Name),
		LeftNodeID:  leaf.ID,
		RightNodeID: backbone.ID,
		LeftSubnet:  leaf.Subnets,
		RightSubnet: backbone.Subnets,
		Status:      "up",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	db.Create(&tunnel)
}
	go func() {
		// Tell backbone to expect leaf's incoming connection and store leaf's public key
		if leaf.PublicKey == "" {
			logwatch.Warn("mesh", fmt.Sprintf("leaf %s (id=%d) has no public key in DB", leaf.Name, leaf.ID))
		}
		if backbone.PublicKey == "" {
			logwatch.Warn("mesh", fmt.Sprintf("backbone %s (id=%d) has no public key in DB", backbone.Name, backbone.ID))
		}
		_, errAccept := registry.SendCmd(backbone.ID, "peer_accept", map[string]any{
			"node_id": leaf.ID, "token": backbone.AgentToken, "public_key": leaf.PublicKey,
		}, 5*time.Second)
		if errAccept != nil {
			logwatch.Warn("mesh", fmt.Sprintf("peer_accept to backbone failed %s->%s: %v", leaf.Name, backbone.Name, errAccept))
		}

		_, err := registry.SendCmd(leaf.ID, "peer_connect", map[string]any{
			"node_id": backbone.ID, "peer_addr": backbone.ListenAddr, "peer_token": backbone.AgentToken, "tunnel_id": tunnel.ID, "public_key": backbone.PublicKey,
		}, 10*time.Second)
		if err != nil {
			logwatch.Warn("mesh", fmt.Sprintf("leaf peer_connect failed %s->%s: %v", leaf.Name, backbone.Name, err))
			return
		}

		time.Sleep(2 * time.Second)

		for _, s := range splitSubnets(backbone.Subnets) {
			registry.SendCmd(leaf.ID, "route_add", map[string]any{
				"subnet": s, "node_id": backbone.ID, "next_hop": backbone.ID,
			}, 5*time.Second)
		}
		if backbone.VirtualIP != "" {
			registry.SendCmd(leaf.ID, "route_add", map[string]any{
				"subnet": backbone.VirtualIP + "/32", "node_id": backbone.ID, "next_hop": backbone.ID,
			}, 5*time.Second)
		}
		for _, s := range splitSubnets(leaf.Subnets) {
			registry.SendCmd(backbone.ID, "route_add", map[string]any{
				"subnet": s, "node_id": leaf.ID, "next_hop": leaf.ID,
			}, 5*time.Second)
		}
		if leaf.VirtualIP != "" {
			registry.SendCmd(backbone.ID, "route_add", map[string]any{
				"subnet": leaf.VirtualIP + "/32", "node_id": leaf.ID, "next_hop": leaf.ID,
			}, 5*time.Second)
		}

		tunnel.Status = "up"
		tunnel.UpdatedAt = time.Now()
		db.Save(&tunnel)
		logwatch.Info("mesh", fmt.Sprintf("leaf tunnel up %s -> %s", leaf.Name, backbone.Name))
	}()
}

func findBestPeers(db *gorm.DB, registry *ws.Registry, excludeID uint) []models.Node {
	var all []models.Node
	db.Where("id != ? AND backbone = ?", excludeID, true).Find(&all)

	var candidates []models.Node
	for _, n := range all {
		if registry.IsOnline(n.ID) {
			candidates = append(candidates, n)
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		si := scoreBackbone(candidates[i].CPU, candidates[i].MemoryMB, -1)
		sj := scoreBackbone(candidates[j].CPU, candidates[j].MemoryMB, -1)
		return si > sj
	})

	if len(candidates) > maxMeshPeers {
		candidates = candidates[:maxMeshPeers]
	}
	return candidates
}

func DetectAndSaveSubnets(db *gorm.DB, registry *ws.Registry, node *models.Node) {
	result, err := registry.SendCmd(node.ID, "subnet_detect", nil, 10*time.Second)
	if err != nil {
		logwatch.Warn("mesh", fmt.Sprintf("subnet detect failed for %s: %v", node.Name, err))
		return
	}
	var data struct {
		Subnets []string `json:"subnets"`
	}
	if result.Data != nil {
		if err := json.Unmarshal(result.Data, &data); err != nil {
			logwatch.Warn("mesh", fmt.Sprintf("subnet detect parse failed for %s: %v", node.Name, err))
			return
		}
	}
	if len(data.Subnets) > 0 {
		localStr := strings.Join(data.Subnets, ",")
		db.Model(node).Updates(map[string]any{"local_subnets": localStr})
		node.LocalSubnets = localStr
		advertised := resolveSubnetConflicts(db, node.ID, data.Subnets)
		if len(advertised) > 0 {
			advStr := strings.Join(advertised, ",")
			db.Model(node).Update("subnets", advStr)
			node.Subnets = advStr
		}
		logwatch.Info("mesh", fmt.Sprintf("subnets for %s: local=%s advertised=%s", node.Name, localStr, node.Subnets))
	}
}

func resolveSubnetConflicts(db *gorm.DB, nodeID uint, subnets []string) []string {
	var others []models.Node
	db.Where("id != ? AND subnets != '' AND subnets != '-'", nodeID).Find(&others)
	existing := make(map[string]bool)
	for _, n := range others {
		for _, s := range strings.Split(n.Subnets, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				existing[s] = true
			}
		}
	}
	var clean []string
	for _, s := range subnets {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if overlaps(s, existing) {
			logwatch.Warn("mesh", fmt.Sprintf("subnet conflict: %s for node %d", s, nodeID))
			continue
		}
		clean = append(clean, s)
	}
	return clean
}

func overlaps(subnet string, existing map[string]bool) bool {
	_, cidr, err := net.ParseCIDR(subnet)
	if err != nil {
		return existing[subnet]
	}
	for es := range existing {
		_, eCidr, eErr := net.ParseCIDR(es)
		if eErr != nil {
			continue
		}
		if cidr.Contains(eCidr.IP) || eCidr.Contains(cidr.IP) {
			return true
		}
	}
	return existing[subnet]
}

func syncAllRoutes(db *gorm.DB, registry *ws.Registry) {
	var nodes []models.Node
	db.Find(&nodes)
	type si struct {
		subnet string
		nodeID uint
	}
	var allSubnets []si
	for _, n := range nodes {
		for _, s := range splitSubnets(n.Subnets) {
			allSubnets = append(allSubnets, si{s, n.ID})
		}
		if n.VirtualIP != "" {
			allSubnets = append(allSubnets, si{n.VirtualIP + "/32", n.ID})
		}
	}
	var tunnels []models.Tunnel
	db.Where("status = ?", "up").Find(&tunnels)
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
	for _, node := range nodes {
		if !registry.IsOnline(node.ID) {
			continue
		}
		for _, s := range allSubnets {
			if s.nodeID == node.ID {
				continue
			}
			nextHop := findNextHop(node.ID, s.nodeID, adj)
			if nextHop == 0 {
				continue
			}
			registry.SendCmd(node.ID, "route_add", map[string]any{
				"subnet": s.subnet, "node_id": s.nodeID, "next_hop": nextHop,
			}, 5*time.Second)
		}
	}
	logwatch.Info("mesh", fmt.Sprintf("synced %d subnets to all nodes", len(allSubnets)))
}

func findNextHop(from, to uint, adj map[uint]map[uint]bool) uint {
	if adj[from][to] {
		return to
	}
	visited := map[uint]bool{from: true}
	type step struct{ node, next uint }
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
	if node.VirtualIP != "" {
		registry.SendCmd(node.ID, "tun_setup", map[string]any{"ip": node.VirtualIP}, 5*time.Second)
	}
}
