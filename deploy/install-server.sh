#!/bin/bash
set -e

BASE="https://meshnet.kisectool.com"

echo ">>> 下载控制端..."
systemctl stop mesnet-server 2>/dev/null || true
curl -# -L -o /usr/local/bin/mesnet-server "${BASE}/mesnet-server"
chmod +x /usr/local/bin/mesnet-server

echo ">>> 下载前端..."
mkdir -p /etc/mesnet/web/dist
curl -# -L "${BASE}/mesnet-web.tar.gz" | tar xz -C /etc/mesnet/web/dist

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
