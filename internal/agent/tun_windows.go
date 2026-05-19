//go:build windows

package agent

import (
	"log"
	"os"
)

// Create is a no-op on Windows. TUN device is not available.
func (t *TUNDevice) Create(ip string) error {
	log.Printf("TUN device not available on Windows, agent running in relay mode")
	return nil
}

// Read returns ErrClosed. TUN device not available on Windows.
func (t *TUNDevice) Read() ([]byte, error) {
	return nil, os.ErrClosed
}

// Write is a no-op on Windows. TUN device not available on Windows.
func (t *TUNDevice) Write(data []byte) error {
	return nil
}
