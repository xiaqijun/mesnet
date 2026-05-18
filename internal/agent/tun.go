package agent

import (
	"log"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"
)

// TUNDevice manages a TUN virtual network interface using Linux /dev/net/tun.
type TUNDevice struct {
	name string
	ip   string
	up   bool
	fd   *os.File
}

const (
	cIFFTUN      = 0x0001
	cIFFNOPI     = 0x1000
	cTUNSETIFF   = 0x400454ca
	cMTU         = 1500
)

func NewTUNDevice() *TUNDevice {
	return &TUNDevice{name: "tun0"}
}

// Create opens /dev/net/tun, creates the interface, assigns IP, and brings it up.
func (t *TUNDevice) Create(ip string) error {
	if t.up {
		return nil // already created
	}
	fd, err := os.OpenFile("/dev/net/tun", os.O_RDWR, 0)
	if err != nil {
		return err
	}

	var ifr [40]byte
	copy(ifr[:16], t.name)
	flags := uint16(cIFFTUN | cIFFNOPI)
	*(*uint16)(unsafe.Pointer(&ifr[16])) = flags

	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd.Fd(), cTUNSETIFF, uintptr(unsafe.Pointer(&ifr)))
	if errno != 0 {
		fd.Close()
		return errno
	}

	// Assign IP
	cmd := exec.Command("ip", "addr", "add", ip+"/16", "dev", t.name)
	if out, err := cmd.CombinedOutput(); err != nil {
		fd.Close()
		log.Printf("tun addr: %s, %v", string(out), err)
		return err
	}

	// Bring up
	cmd = exec.Command("ip", "link", "set", "dev", t.name, "mtu", "1500", "up")
	if out, err := cmd.CombinedOutput(); err != nil {
		fd.Close()
		log.Printf("tun up: %s, %v", string(out), err)
		return err
	}

	// Enable IP forwarding on the host
	exec.Command("sh", "-c", "echo 1 > /proc/sys/net/ipv4/ip_forward").Run()
	// NAT masquerade for TUN traffic so return packets route correctly
	exec.Command("sh", "-c", "iptables -t nat -C POSTROUTING -o "+t.name+" -j MASQUERADE 2>/dev/null || iptables -t nat -A POSTROUTING -o "+t.name+" -j MASQUERADE").Run()

	t.fd = fd
	t.ip = ip
	t.up = true
	log.Printf("tun %s created with IP %s", t.name, ip)
	return nil
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

// Read reads a raw IP packet from the TUN device.
func (t *TUNDevice) Read() ([]byte, error) {
	if t.fd == nil {
		return nil, os.ErrClosed
	}
	buf := make([]byte, 2048)
	n, err := t.fd.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

// Write writes a raw IP packet to the TUN device.
func (t *TUNDevice) Write(data []byte) error {
	if t.fd == nil {
		return os.ErrClosed
	}
	_, err := t.fd.Write(data)
	return err
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
