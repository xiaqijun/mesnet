package handlers

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mesnet/mesnet/internal/server/config"
	"github.com/mesnet/mesnet/internal/server/models"
	"gorm.io/gorm"
)

func GetDeployScript(db *gorm.DB, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		var node models.Node
		if err := db.First(&node, id).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
			return
		}

		wsURL := fmt.Sprintf("wss://YOUR_SERVER:443/ws/agent/%s", node.AgentToken)

		backboneFlag := ""
		if !node.Backbone {
			backboneFlag = " \\\n  --backbone=false"
		}

		script := fmt.Sprintf(`#!/bin/bash
set -e

echo "=== MeshNet Agent ==="

# 下载 Agent 二进制
echo "下载 Agent ..."
curl -sSL -o /usr/local/bin/mesnet-agent "%s"
chmod +x /usr/local/bin/mesnet-agent

# 写入 systemd 服务
cat > /etc/systemd/system/mesnet-agent.service <<'UNIT'
[Unit]
Description=MeshNet Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/mesnet-agent \
  --server %s \
  --listen :443 \
  --name "%s"%s
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
UNIT

# 启动
systemctl daemon-reload
systemctl enable mesnet-agent
systemctl start mesnet-agent

echo "安装完成: systemctl status mesnet-agent"`,
			cfg.Agent.BinaryDownloadURL, wsURL, node.Name, backboneFlag)

		c.JSON(http.StatusOK, gin.H{
			"script": script,
			"node":   node,
		})
	}
}
