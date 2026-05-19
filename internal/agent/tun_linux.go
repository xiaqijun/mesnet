//go:build linux

package agent

import (
	"log"
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

const (
	cIFFTUN    = 0x0001
	cIFFNOPI   = 0x1000
	cTUNSETIFF = 0x400454ca
	cMTU       = 1500
)

// Create opens /dev/net/tun, creates the interface, assigns IP, and brings it up.
func (t *TUNDevice) Create(ip string) error {
	if t.up {
		return nil // already created
	}

	// Clean up any leftover TUN interface from previous run
	exec.Command("ip", "link", "del", "dev", t.name).Run()

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
