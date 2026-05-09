#!/bin/bash
set -e

# MeshNet 控制端一键安装
# 海外: curl -fsSL https://raw.githubusercontent.com/xiaqijun/mesnet/master/deploy/install-server.sh | bash
# 国内: curl -fsSL https://meshnet.<your>.workers.dev | bash

BASE="https://github.com/xiaqijun/mesnet/releases/latest/download"

echo ">>> 下载控制端..."
curl -fsSL "${BASE}/mesnet-server" -o /usr/local/bin/mesnet-server
chmod +x /usr/local/bin/mesnet-server

echo ">>> 下载前端..."
mkdir -p /etc/mesnet/web
curl -fsSL "${BASE}/mesnet-web.tar.gz" | tar xz -C /etc/mesnet/web

echo ">>> 安装服务..."
cat > /etc/systemd/system/mesnet-server.service <<'UNIT'
[Unit]
Description=MeshNet Control Plane
After=network-online.target

[Service]
Type=simple
WorkingDirectory=/etc/mesnet
ExecStart=/usr/local/bin/mesnet-server
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable --now mesnet-server

IP=$(curl -s ifconfig.me 2>/dev/null || hostname -I | awk '{print $1}')
echo ">>> 完成! http://${IP}:8080"
