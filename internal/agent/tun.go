package agent

import (
	"log"
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

// TUNDevice manages a TUN virtual network interface using Linux /dev/net/tun.
type TUNDevice struct {
	name string
	ip   string
	up   bool
	fd   int // raw fd, avoids Go epoll issues with /dev/net/tun
}

const (
	cIFFTUN    = 0x0001
	cIFFNOPI   = 0x1000
	cTUNSETIFF = 0x400454ca
)

func NewTUNDevice() *TUNDevice {
	return &TUNDevice{name: "tun0", fd: -1}
}

// Create opens /dev/net/tun, creates the interface, assigns IP, and brings it up.
func (t *TUNDevice) Create(ip string) error {
	if t.up {
		return nil
	}

	exec.Command("ip", "link", "del", "dev", t.name).Run()

	// Use raw syscall fd to avoid Go runtime epoll issues with /dev/net/tun
	fd, err := syscall.Open("/dev/net/tun", syscall.O_RDWR, 0)
	if err != nil {
		return err
	}

	var ifr [40]byte
	copy(ifr[:16], t.name)
	flags := uint16(cIFFTUN | cIFFNOPI)
	*(*uint16)(unsafe.Pointer(&ifr[16])) = flags

	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), cTUNSETIFF, uintptr(unsafe.Pointer(&ifr)))
	if errno != 0 {
		syscall.Close(fd)
		return errno
	}

	cmd := exec.Command("ip", "addr", "add", ip+"/16", "dev", t.name)
	if out, err := cmd.CombinedOutput(); err != nil {
		syscall.Close(fd)
		log.Printf("tun addr: %s, %v", string(out), err)
		return err
	}

	cmd = exec.Command("ip", "link", "set", "dev", t.name, "mtu", "1500", "up")
	if out, err := cmd.CombinedOutput(); err != nil {
		syscall.Close(fd)
		log.Printf("tun up: %s, %v", string(out), err)
		return err
	}

	exec.Command("sh", "-c", "echo 1 > /proc/sys/net/ipv4/ip_forward").Run()
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
	if t.fd >= 0 {
		syscall.Close(t.fd)
		t.fd = -1
	}
	exec.Command("ip", "link", "del", "dev", t.name).Run()
	return nil
}

// Read reads a raw IP packet from the TUN device using raw syscall.
func (t *TUNDevice) Read() ([]byte, error) {
	if t.fd < 0 {
		return nil, os.ErrClosed
	}
	buf := make([]byte, 2048)
	n, err := syscall.Read(t.fd, buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

// Write writes a raw IP packet to the TUN device using raw syscall.
func (t *TUNDevice) Write(data []byte) error {
	if t.fd < 0 {
		return os.ErrClosed
	}
	_, err := syscall.Write(t.fd, data)
	return err
}

func (t *TUNDevice) IsUp() bool  { return t.up }
func (t *TUNDevice) IP() string  { return t.ip }
