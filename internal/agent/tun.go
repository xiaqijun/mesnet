package agent

import (
	"log"
	"os"
	"os/exec"
	"strings"
)

// TUNDevice manages a TUN virtual network interface.
// On Linux this uses /dev/net/tun. On Windows this is a stub (relay-only mode).
type TUNDevice struct {
	name string
	ip   string
	up   bool
	fd   *os.File
}

func NewTUNDevice() *TUNDevice {
	return &TUNDevice{name: "tun0"}
}

// Destroy removes the TUN interface and closes the file descriptor.
func (t *TUNDevice) Destroy() error {
	t.up = false
	if t.fd != nil {
		t.fd.Close()
		t.fd = nil
	}
	exec.Command("ip", "link", "del", "dev", t.name).Run()
	return nil
}

func (t *TUNDevice) IsUp() bool {
	return t.up
}

func (t *TUNDevice) IP() string {
	return t.ip
}

func (t *TUNDevice) execCmd(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("cmd %s %s: %s, %v", name, strings.Join(args, " "), string(out), err)
	}
	return strings.TrimSpace(string(out))
}
