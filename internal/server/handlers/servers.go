package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mesnet/mesnet/internal/server/models"
	"github.com/mesnet/mesnet/internal/server/services"
	"github.com/mesnet/mesnet/internal/server/ws"
	"gorm.io/gorm"
)

type addCloudBody struct {
	Name       string `json:"name" binding:"required"`
	Host       string `json:"host" binding:"required"`
	InternalIP string `json:"internal_ip"`
	Subnets    string `json:"subnets"`
	ListenAddr string `json:"listen_addr"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	AuthType   string `json:"auth_type"`
	PrivateKey string `json:"private_key"`
	AutoDeploy bool   `json:"auto_deploy"`
}

type addLeafBody struct {
	Name       string `json:"name" binding:"required"`
	BackboneID uint   `json:"backbone_id" binding:"required"`
	Subnets    string `json:"subnets"`
}

func GetServers(db *gorm.DB, registry *ws.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		var nodes []models.Node
		db.Order("created_at desc").Find(&nodes)

		onlineSet := make(map[uint]bool)
		for _, id := range registry.ListOnline() {
			onlineSet[id] = true
		}

		cloudServers := make([]gin.H, 0)
		leafNodes := make([]gin.H, 0)

		for _, n := range nodes {
			var tunnelCount int64
			db.Model(&models.Tunnel{}).
				Where("(left_node_id = ? OR right_node_id = ?) AND status = ?", n.ID, n.ID, "up").
				Count(&tunnelCount)

			item := gin.H{
				"id":           n.ID,
				"name":         n.Name,
				"host":         n.Host,
				"subnets":      n.Subnets,
				"virtual_ip":   n.VirtualIP,
				"listen_addr":  n.ListenAddr,
				"online":       onlineSet[n.ID],
				"version":      n.AgentVersion,
				"cpu":          n.CPU,
				"memory_mb":    n.MemoryMB,
				"last_seen":    n.LastSeen,
				"created_at":   n.CreatedAt,
				"tunnel_count": tunnelCount,
			}

			if n.Backbone {
				cloudServers = append(cloudServers, item)
			} else {
				leafNodes = append(leafNodes, item)
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"cloud_servers": cloudServers,
			"leaf_nodes":    leafNodes,
		})
	}
}

func AddCloudServer(db *gorm.DB, registry *ws.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body addCloudBody
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		token := generateToken()

		node := models.Node{
			Name:       body.Name,
			Host:       body.Host,
			Subnets:    body.Subnets,
			Backbone:   true,
			AgentToken: token,
			ListenAddr: body.ListenAddr,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		db.Create(&node)

		var count int64
		db.Model(&models.Node{}).Count(&count)
		node.VirtualIP = fmt.Sprintf("10.100.0.%d", count)
		db.Save(&node)

		addAudit(db, "server.cloud_add", "node", node.ID, "Added cloud server "+node.Name)

		response := gin.H{"node": node}

		if body.AutoDeploy && body.Username != "" {
			serverAddr := c.Request.Host
			if serverAddr == "" {
				serverAddr = "localhost:8080"
			}

			ips := []string{}
			if body.InternalIP != "" {
				ips = append(ips, body.InternalIP)
			}
			ips = append(ips, body.Host)

			deployed := false
			for _, ip := range ips {
				ssh := services.NewSSHClient(ip, 22, body.Username, body.Password, nil)
				if _, err := ssh.TestConnection(); err != nil {
					continue
				}
				steps, err := ssh.DeployAgent(serverAddr, token, node.Name, true)
				if err == nil {
					response["ssh_ip"] = ip
					response["ssh_steps"] = steps
					response["deployed"] = true
					deployed = true
				}
				break
			}
			if !deployed {
				response["deployed"] = false
				response["ssh_error"] = "SSH 不通（内网公网均已尝试）"
				response["script"] = onelinerDeploy(token, node.Name, c.Request.Host, true)
			}
		} else {
			response["script"] = onelinerDeploy(token, node.Name, c.Request.Host, true)
		}

		c.JSON(http.StatusCreated, response)
	}
}

func AddLeafNode(db *gorm.DB, registry *ws.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body addLeafBody
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		var backbone models.Node
		if err := db.First(&backbone, body.BackboneID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "backbone node not found"})
			return
		}

		subnets := body.Subnets
		if subnets == "" && backbone.Subnets != "" {
			subnets = backbone.Subnets
		}

		token := generateToken()
		node := models.Node{
			Name:       body.Name,
			Subnets:    subnets,
			Backbone:   false,
			AgentToken: token,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		db.Create(&node)

		var count int64
		db.Model(&models.Node{}).Count(&count)
		node.VirtualIP = fmt.Sprintf("10.100.0.%d", count)
		db.Save(&node)

		addAudit(db, "server.leaf_add", "node", node.ID,
			fmt.Sprintf("Added leaf node %s under backbone %s", node.Name, backbone.Name))

		c.JSON(http.StatusCreated, gin.H{
			"node":     node,
			"backbone": backbone,
			"script":   onelinerDeploy(token, node.Name, c.Request.Host, false),
		})
	}
}

func GetServerDeploy(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		var node models.Node
		if err := db.First(&node, id).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"node":   node,
			"script": onelinerDeploy(node.AgentToken, node.Name, c.Request.Host, node.Backbone),
		})
	}
}

// onelinerDeploy returns a single-line install command.
func onelinerDeploy(token, name, serverAddr string, backbone bool) string {
	bf := ""
	if !backbone {
		bf = " --backbone=false"
	}
	return fmt.Sprintf(
		"curl -fsSL https://meshnet.kisectool.com/mesnet-agent-linux-amd64 -o /usr/local/bin/mesnet-agent && chmod +x /usr/local/bin/mesnet-agent && systemctl stop mesnet-agent 2>/dev/null; printf '[Unit]\\nDescription=MeshNet Agent\\nAfter=network-online.target\\n[Service]\\nType=simple\\nExecStart=/usr/local/bin/mesnet-agent --server ws://%s/ws/agent/%s --listen :443 --name \"%s\"%s\\nRestart=always\\n[Install]\\nWantedBy=multi-user.target\\n' > /etc/systemd/system/mesnet-agent.service && systemctl daemon-reload && systemctl enable --now mesnet-agent",
		serverAddr, token, name, bf)
}
