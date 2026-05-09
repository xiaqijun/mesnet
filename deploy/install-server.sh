#!/bin/bash
set -e

# Default: auto-detect mirror
# https://gitee.com/xiaqiqi/mesnet  (Gitee, 国内快)
# https://github.com/xiaqijun/mesnet (GitHub, 海外)

GIT=${GIT_REPO:-auto}

if [ "$GIT" = "auto" ]; then
  if curl -s --connect-timeout 2 https://gitee.com >/dev/null 2>&1; then
    GIT="gitee"
    echo ">>> 检测到国内网络，使用 Gitee"
  else
    GIT="github"
    echo ">>> 使用 GitHub"
  fi
fi

if [ "$GIT" = "gitee" ]; then
  BASE="https://gitee.com/xiaqiqi/mesnet/releases/download/v1.0.0"
  # Gitee releases: the download URL format
  BIN_BASE="$BASE"
  WEB_BASE="$BASE"
else
  BASE="https://github.com/xiaqijun/mesnet/releases/latest/download"
  BIN_BASE="$BASE"
  WEB_BASE="$BASE"
fi

echo ">>> 下载控制端..."
curl -fsSL "${BIN_BASE}/mesnet-server" -o /usr/local/bin/mesnet-server
chmod +x /usr/local/bin/mesnet-server

echo ">>> 下载前端..."
mkdir -p /etc/mesnet/web
curl -fsSL "${WEB_BASE}/mesnet-web.tar.gz" | tar xz -C /etc/mesnet/web

echo ">>> 安装系统服务..."
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

echo ">>> 完成! 访问 http://$(curl -s ifconfig.me 2>/dev/null || hostname -I | awk '{print $1}'):8080"
