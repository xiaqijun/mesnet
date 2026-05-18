package agent

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// RouteEntry represents a route in the mesh routing table.
type RouteEntry struct {
	Subnet  string // e.g. "172.16.0.0/16"
	NodeID  uint   // the owner node of this subnet
	NextHop uint   // directly connected peer to forward to (may equal NodeID)
}

// RouteManager manages local routing rules for tunnel subnets.
type RouteManager struct {
	routes map[string]*RouteEntry // subnet → entry
	mu     sync.RWMutex
}

func NewRouteManager() *RouteManager {
	return &RouteManager{
		routes: make(map[string]*RouteEntry),
	}
}

// Add adds a route for subnet via the TUN device, with optional next-hop.
// If the TUN device is not ready yet, the route is stored in-memory only
// and will be synced to the kernel when FlushKernel is called.
func (r *RouteManager) Add(subnet string, nodeID, nextHop uint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry := &RouteEntry{Subnet: subnet, NodeID: nodeID, NextHop: nextHop}
	if nextHop == 0 {
		entry.NextHop = nodeID
	}

	r.routes[subnet] = entry

	// Only sync to kernel if TUN device exists
	if _, err := os.Stat("/sys/class/net/tun0"); err == nil {
		cmd := exec.Command("ip", "route", "replace", subnet, "dev", "tun0")
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("route replace %s dev tun0: %s", subnet, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

// FlushKernel syncs all stored routes to the kernel routing table.
// Called after TUN device is created.
func (r *RouteManager) FlushKernel() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for subnet := range r.routes {
		cmd := exec.Command("ip", "route", "replace", subnet, "dev", "tun0")
		out, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("route flush %s dev tun0: %s", subnet, strings.TrimSpace(string(out)))
		}
	}
}

// Del removes a route for subnet.
func (r *RouteManager) Del(subnet string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.routes[subnet]; !ok {
		return nil
	}
	delete(r.routes, subnet)

	cmd := exec.Command("ip", "route", "del", subnet, "dev", "tun0")
	cmd.CombinedOutput()
	return nil
}

// List returns all active routes.
func (r *RouteManager) List() []*RouteEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]*RouteEntry, 0, len(r.routes))
	for _, e := range r.routes {
		list = append(list, e)
	}
	return list
}

// Lookup finds the next-hop node for a given destination IP using longest prefix match.
func (r *RouteManager) Lookup(dstIP string) (nodeID, nextHop uint) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	dst := net.ParseIP(dstIP)
	if dst == nil {
		return 0, 0
	}

	var bestMatch *RouteEntry
	var bestLen int
	for _, entry := range r.routes {
		_, cidr, err := net.ParseCIDR(entry.Subnet)
		if err != nil {
			continue
		}
		ones, _ := cidr.Mask.Size()
		if cidr.Contains(dst) && ones > bestLen {
			bestLen = ones
			bestMatch = entry
		}
	}

	if bestMatch == nil {
		return 0, 0
	}
	return bestMatch.NodeID, bestMatch.NextHop
}

// Flood propagates all local routes to connected peers via a callback.
func (r *RouteManager) Flood(peerIDs []uint, sendFn func(peerID uint, route *RouteEntry)) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, entry := range r.routes {
		for _, pid := range peerIDs {
			sendFn(pid, entry)
		}
	}
}

// getSubnets returns all subnet strings (for status reporting).
func (r *RouteManager) getSubnets() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]string, 0, len(r.routes))
	for _, e := range r.routes {
		list = append(list, e.Subnet)
	}
	return list
}

// DetectSubnets discovers the real physical NIC subnet (default route interface).
// Returns the network address (e.g. 10.1.0.0/22), not the host IP (10.1.0.2/22).
func (r *RouteManager) DetectSubnets() ([]string, error) {
	out, err := exec.Command("sh", "-c",
		"ip route show default | awk '{print $5}' | head -1").CombinedOutput()
	if err != nil {
		return nil, err
	}
	iface := strings.TrimSpace(string(out))
	if iface == "" {
		return nil, nil
	}

	out, err = exec.Command("sh", "-c",
		"ip -4 -o addr show "+iface+" | awk '{print $4}' | head -1").CombinedOutput()
	if err != nil {
		return nil, err
	}
	cidr := strings.TrimSpace(string(out))
	if cidr == "" {
		return nil, nil
	}

	// Convert host CIDR to network address: 10.1.0.2/22 → 10.1.0.0/22
	if _, ipNet, err := net.ParseCIDR(cidr); err == nil {
		cidr = ipNet.String()
	}

	log.Printf("detected subnet: %s on %s", cidr, iface)
	return []string{cidr}, nil
}

// extractDstIP extracts the destination IP from an IPv4 packet header.
func extractDstIP(packet []byte) string {
	if len(packet) < 20 {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.%d", packet[16], packet[17], packet[18], packet[19])
}

// extractSrcIP extracts the source IP from an IPv4 packet header.
func extractSrcIP(packet []byte) string {
	if len(packet) < 16 {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.%d", packet[12], packet[13], packet[14], packet[15])
}

// buildIPPacket constructs a minimal IP packet for probe data.
func buildIPPacket(src, dst string, id uint16, payload []byte) []byte {
	srcIP := net.ParseIP(src).To4()
	dstIP := net.ParseIP(dst).To4()
	if srcIP == nil || dstIP == nil {
		return nil
	}

	totalLen := 20 + len(payload)
	pkt := make([]byte, totalLen)

	pkt[0] = 0x45
	pkt[1] = 0
	binary.BigEndian.PutUint16(pkt[2:4], uint16(totalLen))
	binary.BigEndian.PutUint16(pkt[4:6], id)
	pkt[6] = 0
	pkt[7] = 0
	pkt[8] = 64
	pkt[9] = 253
	copy(pkt[12:16], srcIP)
	copy(pkt[16:20], dstIP)
	copy(pkt[20:], payload)

	return pkt
}
