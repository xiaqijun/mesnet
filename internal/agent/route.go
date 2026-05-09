package agent

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// RouteManager manages local routing rules for tunnel subnets.
type RouteManager struct {
	routes map[string]bool // subnet → active
	mu     sync.RWMutex
}

func NewRouteManager() *RouteManager {
	return &RouteManager{
		routes: make(map[string]bool),
	}
}

// Add adds a route for subnet via the TUN device.
func (r *RouteManager) Add(subnet string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if already exists
	if r.routes[subnet] {
		return nil
	}

	// ip route add <subnet> dev tun0
	cmd := exec.Command("ip", "route", "add", subnet, "dev", "tun0")
	if out, err := cmd.CombinedOutput(); err != nil {
		outStr := strings.TrimSpace(string(out))
		// "already exists" is not an error
		if strings.Contains(outStr, "File exists") {
			r.routes[subnet] = true
			return nil
		}
		return fmt.Errorf("route add %s: %s, %w", subnet, outStr, err)
	}

	r.routes[subnet] = true
	return nil
}

// Del removes a route for subnet.
func (r *RouteManager) Del(subnet string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.routes[subnet] {
		return nil
	}

	cmd := exec.Command("ip", "route", "del", subnet, "dev", "tun0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("route del %s: %s, %w", subnet, string(out), err)
	}

	delete(r.routes, subnet)
	return nil
}

// List returns all active routes.
func (r *RouteManager) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]string, 0, len(r.routes))
	for subnet := range r.routes {
		list = append(list, subnet)
	}
	return list
}

// Lookup finds which node should handle traffic to a given IP.
func (r *RouteManager) Lookup(dstIP string) uint {
	// In production, use longest prefix match against route table
	// Simplified: return 0 (no route) for now
	return 0
}

// DetectSubnets discovers local subnets on this machine.
func (r *RouteManager) DetectSubnets() ([]string, error) {
	cmd := exec.Command("ip", "-o", "route", "show", "scope", "link")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(out), "\n")
	subnets := make([]string, 0)
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 1 {
			continue
		}
		cidr := fields[0]
		if strings.Contains(cidr, "/") && !strings.Contains(cidr, "docker") && !strings.Contains(cidr, "br-") {
			subnets = append(subnets, cidr)
		}
	}

	if len(subnets) == 0 {
		// Fallback: use default route interface
		cmd2 := exec.Command("sh", "-c",
			"ip route show default | awk '{print $5}' | head -1")
		out2, _ := cmd2.CombinedOutput()
		iface := strings.TrimSpace(string(out2))
		if iface != "" {
			cmd3 := exec.Command("sh", "-c",
				"ip -o addr show "+iface+" | awk '{print $4}' | head -1")
			out3, _ := cmd3.CombinedOutput()
			cidr := strings.TrimSpace(string(out3))
			if cidr != "" {
				subnets = append(subnets, cidr)
			}
		}
	}

	return subnets, nil
}
