package agent

import (
	"log"
	"os/exec"
	"strings"
)

// TUNDevice manages a TUN virtual network interface.
type TUNDevice struct {
	name string
	ip   string
	up   bool
}

func NewTUNDevice() *TUNDevice {
	return &TUNDevice{name: "tun0"}
}

func (t *TUNDevice) Create(ip string) error {
	// Create TUN device (Linux)
	cmd := exec.Command("ip", "tuntap", "add", "dev", t.name, "mode", "tun")
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("tun create: %s, %v", string(out), err)
		return err
	}

	// Assign IP
	cmd = exec.Command("ip", "addr", "add", ip+"/16", "dev", t.name)
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("tun addr: %s, %v", string(out), err)
		return err
	}

	// Bring up
	cmd = exec.Command("ip", "link", "set", "dev", t.name, "up")
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("tun up: %s, %v", string(out), err)
		return err
	}

	t.ip = ip
	t.up = true
	log.Printf("tun %s created with IP %s", t.name, ip)
	return nil
}

func (t *TUNDevice) Destroy() error {
	cmd := exec.Command("ip", "link", "del", "dev", t.name)
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("tun destroy: %s, %v", string(out), err)
		return err
	}
	t.up = false
	return nil
}

func (t *TUNDevice) IsUp() bool {
	return t.up
}

func (t *TUNDevice) IP() string {
	return t.ip
}

func (t *TUNDevice) Read() ([]byte, error) {
	// Read from TUN device file descriptor
	// In production, this would use golang.org/x/net or open /dev/net/tun directly
	cmd := exec.Command("cat", "/dev/net/tun")
	out, err := cmd.Output()
	return out, err
}

func (t *TUNDevice) Write(data []byte) error {
	// Write to TUN device
	return nil
}

func (t *TUNDevice) execCmd(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("cmd %s %s: %s, %v", name, strings.Join(args, " "), string(out), err)
	}
	return strings.TrimSpace(string(out))
}
