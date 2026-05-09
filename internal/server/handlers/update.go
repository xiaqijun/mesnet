package handlers

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mesnet/mesnet/internal/server/models"
	"github.com/mesnet/mesnet/internal/server/ws"
	"gorm.io/gorm"
)

const CurrentVersion = "v1.0.0"

const ReleaseBase = "https://github.com/xiaqijun/mesnet/releases/latest/download"

// GetAgentVersions returns version info for all nodes and the server itself.
func GetAgentVersions(db *gorm.DB, registry *ws.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		var nodes []models.Node
		db.Order("name asc").Find(&nodes)

		type versionInfo struct {
			ID      uint   `json:"id"`
			Name    string `json:"name"`
			Version string `json:"version"`
			Online  bool   `json:"online"`
			Latest  bool   `json:"latest"`
		}

		result := make([]versionInfo, 0, len(nodes))
		for _, n := range nodes {
			result = append(result, versionInfo{
				ID:      n.ID,
				Name:    n.Name,
				Version: n.AgentVersion,
				Online:  registry.IsOnline(n.ID),
				Latest:  n.AgentVersion == "" || n.AgentVersion == CurrentVersion,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"current_version": CurrentVersion,
			"server_version":  CurrentVersion,
			"nodes":           result,
		})
	}
}

// UpdateServer triggers a full update: server binary + frontend.
func UpdateServer() gin.HandlerFunc {
	return func(c *gin.Context) {
		go func() {
			// 1. Update server binary
			resp, err := http.Get(fmt.Sprintf("%s/mesnet-server", ReleaseBase))
			if err != nil {
				return
			}

			tmpPath := "/tmp/mesnet-server.new"
			f, _ := os.Create(tmpPath)
			if f != nil {
				io.Copy(f, resp.Body)
				f.Close()
				resp.Body.Close()
				os.Chmod(tmpPath, 0755)

				exePath, _ := os.Executable()
				os.Rename(tmpPath, exePath)
			}

			// 2. Update frontend
			webResp, err := http.Get(fmt.Sprintf("%s/mesnet-web.tar.gz", ReleaseBase))
			if err == nil && webResp.StatusCode == 200 {
				defer webResp.Body.Close()
				extractTarGz(webResp.Body, "/etc/mesnet/web")
			}

			os.Exit(0)
		}()

		c.JSON(http.StatusOK, gin.H{"status": "restarting"})
	}
}

func extractTarGz(r io.Reader, dst string) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		path := filepath.Join(dst, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(path, 0755)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(path), 0755)
			f, err := os.Create(path)
			if err == nil {
				io.Copy(f, tr)
				f.Close()
			}
		}
	}
	return nil
}

// UpdateAgent triggers a self-update on a specific agent.
func UpdateAgent(registry *ws.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		type req struct {
			NodeID uint `json:"node_id" binding:"required"`
		}
		var body req
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if !registry.IsOnline(body.NodeID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "agent offline"})
			return
		}

		_, err := registry.SendCmd(body.NodeID, "agent_update", nil, 30*time.Second)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "updating"})
	}
}

// UpdateAllAgents triggers self-update on all online agents.
func UpdateAllAgents(registry *ws.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		online := registry.ListOnline()
		updated := 0
		for _, nodeID := range online {
			registry.SendCmd(nodeID, "agent_update", nil, 30*time.Second)
			updated++
		}

		c.JSON(http.StatusOK, gin.H{
			"updated": updated,
			"total":   len(online),
		})
	}
}
