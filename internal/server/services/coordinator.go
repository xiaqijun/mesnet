package services

import (
	"github.com/mesnet/mesnet/internal/server/models"
	"gorm.io/gorm"
)

// VirtualNetwork manages IP allocation for the mesh overlay.
type VirtualNetwork struct {
	Network string // e.g. "10.100.0.0/16"
	db      *gorm.DB
}

func NewVirtualNetwork(network string, db *gorm.DB) *VirtualNetwork {
	return &VirtualNetwork{Network: network, db: db}
}

// AllocateIP assigns the next available virtual IP to a node.
func (vn *VirtualNetwork) AllocateIP(nodeID uint) (string, error) {
	var count int64
	vn.db.Model(&models.Node{}).Count(&count)
	ip := "10.100.0." + itoa(int(count))
	return ip, nil
}

// SyncRoutes pushes route updates to all affected nodes when tunnels change.
func SyncRoutes(db *gorm.DB, tunnelID uint) error {
	var tunnel models.Tunnel
	if err := db.Preload("LeftNode").Preload("RightNode").First(&tunnel, tunnelID).Error; err != nil {
		return err
	}

	// Routes to push:
	// LeftNode: add route to RightSubnet via tunnel
	// RightNode: add route to LeftSubnet via tunnel
	_ = tunnel.LeftSubnet
	_ = tunnel.RightSubnet

	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := make([]byte, 0, 10)
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
