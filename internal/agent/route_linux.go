//go:build linux

package agent

import (
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
)

func routeAddKernel(subnet string) {
	if _, err := os.Stat("/sys/class/net/tun0"); err != nil {
		return
	}
	cmd := exec.Command("ip", "route", "add", subnet, "dev", "tun0")
	if out, err := cmd.CombinedOutput(); err != nil {
		outStr := strings.TrimSpace(string(out))
		if !strings.Contains(outStr, "File exists") {
			log.Printf("route add %s dev tun0: %s", subnet, outStr)
		}
	}
}

func routeDelKernel(subnet string) {
	exec.Command("ip", "route", "del", subnet, "dev", "tun0").CombinedOutput()
}

func detectSubnets() ([]string, error) {
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

	if _, ipNet, err := net.ParseCIDR(cidr); err == nil {
		cidr = ipNet.String()
	}

	log.Printf("detected subnet: %s on %s", cidr, iface)
	return []string{cidr}, nil
}
