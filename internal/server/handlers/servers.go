package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mesnet/mesnet/internal/server/logwatch"
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
			var tc int64
			db.Model(&models.Tunnel{}).Where("(left_node_id = ? OR right_node_id = ?) AND status = ?", n.ID, n.ID, "up").Count(&tc)
			item := gin.H{
				"id": n.ID, "name": n.Name, "host": n.Host, "subnets": n.Subnets,
				"virtual_ip": n.VirtualIP, "listen_addr": n.ListenAddr, "online": onlineSet[n.ID],
				"version": n.AgentVersion, "cpu": n.CPU, "memory_mb": n.MemoryMB,
				"last_seen": n.LastSeen, "created_at": n.CreatedAt, "tunnel_count": tc,
			}
			if n.Backbone {
				cloudServers = append(cloudServers, item)
			} else {
				leafNodes = append(leafNodes, item)
			}
		}
		c.JSON(http.StatusOK, gin.H{"cloud_servers": cloudServers, "leaf_nodes": leafNodes})
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
			Name: body.Name, Host: body.Host, Subnets: body.Subnets, Backbone: true,
			AgentToken: token, ListenAddr: body.ListenAddr, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
		db.Create(&node)
		var count int64
		db.Model(&models.Node{}).Count(&count)
		node.VirtualIP = fmt.Sprintf("10.100.0.%d", count)
		db.Save(&node)
		addAudit(db, "server.cloud_add", "node", node.ID, "Added "+node.Name)

		resp := gin.H{"node": node}

		if body.AutoDeploy && body.Username != "" {
			serverAddr := c.Request.Host
			if serverAddr == "" {
				serverAddr = "localhost:8080"
			}
			ips := []struct{ addr, label string }{}
			if body.InternalIP != "" {
				ips = append(ips, struct{ addr, label string }{body.InternalIP, "内网"})
			}
			ips = append(ips, struct{ addr, label string }{body.Host, "公网"})

			var logs []string
			deployed := false
			for _, ip := range ips {
				ssh := services.NewSSHClient(ip.addr, 22, body.Username, body.Password, nil)
				out, err := ssh.TestConnection()
				if err != nil {
					logs = append(logs, fmt.Sprintf("%s %s: 不通", ip.label, ip.addr))
					continue
				}
				logs = append(logs, fmt.Sprintf("%s %s: 连接成功 %s", ip.label, ip.addr, strings.TrimSpace(out)))
				steps, err := ssh.DeployAgent(serverAddr, token, node.Name, true)
				if err == nil {
					resp["deployed"] = true
					resp["ssh_ip"] = ip.addr
					resp["ssh_steps"] = steps
					deployed = true
				} else {
					logs = append(logs, fmt.Sprintf("部署失败: %v", err))
				}
				break
			}
			resp["ssh_test"] = logs
			if !deployed {
				resp["deployed"] = false
				resp["ssh_error"] = strings.Join(logs, "; ")
				resp["script"] = onelinerDeploy(token, node.Name, c.Request.Host, true)
				logwatch.Error("ssh", fmt.Sprintf("deploy %s: %s", node.Name, strings.Join(logs, "; ")))
			}
		} else {
			resp["script"] = onelinerDeploy(token, node.Name, c.Request.Host, true)
		}
		c.JSON(http.StatusCreated, resp)
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
			c.JSON(http.StatusBadRequest, gin.H{"error": "backbone not found"})
			return
		}
		subnets := body.Subnets
		if subnets == "" && backbone.Subnets != "" {
			subnets = backbone.Subnets
		}
		token := generateToken()
		node := models.Node{
			Name: body.Name, Subnets: subnets, Backbone: false,
			AgentToken: token, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
		db.Create(&node)
		var count int64
		db.Model(&models.Node{}).Count(&count)
		node.VirtualIP = fmt.Sprintf("10.100.0.%d", count)
		db.Save(&node)
		addAudit(db, "server.leaf_add", "node", node.ID, "Added leaf "+node.Name)
		c.JSON(http.StatusCreated, gin.H{
			"node": node, "backbone": backbone,
			"script": onelinerDeploy(token, node.Name, c.Request.Host, false),
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
			"node": node,
			"script": onelinerDeploy(node.AgentToken, node.Name, c.Request.Host, node.Backbone),
		})
	}
}

func onelinerDeploy(token, name, serverAddr string, backbone bool) string {
	bf := ""
	if !backbone {
		bf = " --backbone=false"
	}
	return fmt.Sprintf(
		"curl -fsSL https://meshnet.kisectool.com/mesnet-agent-linux-amd64 -o /usr/local/bin/mesnet-agent && chmod +x /usr/local/bin/mesnet-agent && systemctl stop mesnet-agent 2>/dev/null; printf '[Unit]\\nDescription=MeshNet Agent\\nAfter=network-online.target\\n[Service]\\nType=simple\\nExecStart=/usr/local/bin/mesnet-agent --server ws://%s/ws/agent/%s --listen :443 --name \"%s\"%s\\nRestart=always\\n[Install]\\nWantedBy=multi-user.target\\n' > /etc/systemd/system/mesnet-agent.service && systemctl daemon-reload && systemctl enable --now mesnet-agent",
		serverAddr, token, name, bf)
}
