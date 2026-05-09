#!/bin/bash
set -e

# Auto-detect network: use mirror for China
if curl -s --connect-timeout 2 https://ghproxy.com >/dev/null 2>&1; then
  MIRROR="https://ghproxy.com/"
  echo "检测到国内网络，使用代理加速"
else
  MIRROR=""
  echo "使用直连下载"
fi

BASE="${MIRROR}https://github.com/xiaqijun/mesnet/releases/latest/download"

echo ">>> 下载控制端..."
curl -fsSL "${BASE}/mesnet-server" -o /usr/local/bin/mesnet-server
chmod +x /usr/local/bin/mesnet-server

echo ">>> 下载前端..."
mkdir -p /etc/mesnet/web
curl -fsSL "${BASE}/mesnet-web.tar.gz" | tar xz -C /etc/mesnet/web

echo ">>> 安装系统服务..."
cat > /etc/systemd/system/mesnet-server.service <<'EOF'
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
EOF

systemctl daemon-reload
systemctl enable --now mesnet-server

echo ">>> 完成! 访问 http://$(curl -s ifconfig.me):8080"
