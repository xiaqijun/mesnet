package services

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/mesnet/mesnet/internal/server/logwatch"
	"github.com/mesnet/mesnet/internal/server/models"
	"github.com/mesnet/mesnet/internal/server/ws"
	"gorm.io/gorm"
)

const maxMeshPeers = 3

func AutoMesh(db *gorm.DB, registry *ws.Registry, nodeID uint) {
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
		return
	}

	if node.Backbone {
		for _, peer := range peers {
			createRelayTunnel(db, registry, &node, &peer)
		}
	} else {
		best := findBestBackbone(peers)
		if best != nil {
			createRelayTunnel(db, registry, &node, best)
		}
		if len(peers) > 1 {
			if s := findSecondBest(peers, best.ID); s != nil {
				createRelayTunnel(db, registry, &node, s)
			}
		}
	}

	time.Sleep(3 * time.Second)
	syncAllRoutes(db, registry)
}

func createRelayTunnel(db *gorm.DB, registry *ws.Registry, a, b *models.Node) {
	var existing models.Tunnel
	if db.Where("(left_node_id = ? AND right_node_id = ?) OR (left_node_id = ? AND right_node_id = ?)",
		a.ID, b.ID, b.ID, a.ID).First(&existing).Error == nil {
		return
	}

	t := models.Tunnel{
		Name: fmt.Sprintf("%s <-> %s", a.Name, b.Name),
		LeftNodeID: a.ID, RightNodeID: b.ID,
		LeftSubnet: a.Subnets, RightSubnet: b.Subnets,
		Status: "up", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	db.Create(&t)

	go func() {
		for _, s := range splitSubnets(a.Subnets) {
			registry.SendCmd(b.ID, "route_add", map[string]any{
				"subnet": s, "node_id": a.ID, "next_hop": a.ID,
			}, 5*time.Second)
		}
		for _, s := range splitSubnets(b.Subnets) {
			registry.SendCmd(a.ID, "route_add", map[string]any{
				"subnet": s, "node_id": b.ID, "next_hop": b.ID,
			}, 5*time.Second)
		}
		logwatch.Info("mesh", fmt.Sprintf("tunnel up %s <-> %s", a.Name, b.Name))
	}()
}

func splitSubnets(s string) []string {
	if s == "" || s == "-" {
		return nil
	}
	var r []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			r = append(r, p)
		}
	}
	return r
}

func findBestPeers(db *gorm.DB, registry *ws.Registry, excludeID uint) []models.Node {
	var all []models.Node
	db.Where("id != ? AND backbone = ?", excludeID, true).Order("cpu * memory_mb DESC").Find(&all)
	if len(all) > maxMeshPeers { all = all[:maxMeshPeers] }
	var online []models.Node
	for _, n := range all {
		if registry.IsOnline(n.ID) { online = append(online, n) }
	}
	return online
}

func findBestBackbone(nodes []models.Node) *models.Node {
	for i := range nodes {
		if nodes[i].Connected { return &nodes[i] }
	}
	return nil
}
func findSecondBest(nodes []models.Node, excludeID uint) *models.Node {
	for i := range nodes {
		if nodes[i].ID != excludeID && nodes[i].Connected { return &nodes[i] }
	}
	return nil
}

func DetectAndSaveSubnets(db *gorm.DB, registry *ws.Registry, node *models.Node) {
	r, err := registry.SendCmd(node.ID, "subnet_detect", nil, 10*time.Second)
	if err != nil {
		logwatch.Warn("mesh", fmt.Sprintf("subnet detect failed %s: %v", node.Name, err))
		return
	}
	var d struct{ Subnets []string `json:"subnets"` }
	if r.Data != nil { json.Unmarshal(r.Data, &d) }
	if len(d.Subnets) > 0 {
		ls := strings.Join(d.Subnets, ",")
		db.Model(node).Updates(map[string]any{"local_subnets": ls})
		node.LocalSubnets = ls
		if adv := resolveSubnetConflicts(db, node.ID, d.Subnets); len(adv) > 0 {
			as := strings.Join(adv, ",")
			db.Model(node).Update("subnets", as)
			node.Subnets = as
		}
		logwatch.Info("mesh", fmt.Sprintf("subnets %s: %s", node.Name, node.Subnets))
	}
}

func resolveSubnetConflicts(db *gorm.DB, nodeID uint, subnets []string) []string {
	var others []models.Node
	db.Where("id != ? AND subnets != '' AND subnets != '-'", nodeID).Find(&others)
	ex := make(map[string]bool)
	for _, n := range others {
		for _, s := range strings.Split(n.Subnets, ",") {
			if s = strings.TrimSpace(s); s != "" { ex[s] = true }
		}
	}
	var clean []string
	for _, s := range subnets {
		if s = strings.TrimSpace(s); s == "" { continue }
		if overlaps(s, ex) {
			logwatch.Warn("mesh", fmt.Sprintf("conflict: %s for %d", s, nodeID))
			continue
		}
		clean = append(clean, s)
	}
	return clean
}

func overlaps(s string, ex map[string]bool) bool {
	_, c, e := net.ParseCIDR(s)
	if e != nil { return ex[s] }
	for es := range ex {
		if _, ec, ee := net.ParseCIDR(es); ee == nil {
			if c.Contains(ec.IP) || ec.Contains(c.IP) { return true }
		}
	}
	return ex[s]
}

func syncAllRoutes(db *gorm.DB, registry *ws.Registry) {
	var nodes []models.Node; db.Find(&nodes)
	type si struct{ s string; id uint }
	var all []si
	for _, n := range nodes {
		for _, s := range splitSubnets(n.Subnets) { all = append(all, si{s, n.ID}) }
	}
	var tunnels []models.Tunnel; db.Where("status = ?", "up").Find(&tunnels)
	adj := make(map[uint]map[uint]bool)
	for _, t := range tunnels {
		if adj[t.LeftNodeID] == nil { adj[t.LeftNodeID] = make(map[uint]bool) }
		if adj[t.RightNodeID] == nil { adj[t.RightNodeID] = make(map[uint]bool) }
		adj[t.LeftNodeID][t.RightNodeID] = true
		adj[t.RightNodeID][t.LeftNodeID] = true
	}
	for _, n := range nodes {
		if !registry.IsOnline(n.ID) { continue }
		for _, s := range all {
			if s.id == n.ID { continue }
			if nh := findNextHop(n.ID, s.id, adj); nh != 0 {
				registry.SendCmd(n.ID, "route_add", map[string]any{
					"subnet": s.s, "node_id": s.id, "next_hop": nh,
				}, 5*time.Second)
			}
		}
	}
}

func findNextHop(from, to uint, adj map[uint]map[uint]bool) uint {
	if adj[from][to] { return to }
	v := map[uint]bool{from: true}
	type st struct{ n, nx uint }
	var q []st
	for p := range adj[from] { q = append(q, st{p, p}) }
	for len(q) > 0 {
		x := q[0]; q = q[1:]
		if x.n == to { return x.nx }
		if v[x.n] { continue }
		v[x.n] = true
		for p := range adj[x.n] {
			if !v[p] { q = append(q, st{p, x.nx}) }
		}
	}
	return 0
}

func createSelfTunnel(db *gorm.DB, registry *ws.Registry, node *models.Node) {
	if node.VirtualIP != "" {
		registry.SendCmd(node.ID, "tun_setup", map[string]any{"ip": node.VirtualIP}, 5*time.Second)
	}
}
