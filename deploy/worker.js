// Cloudflare Worker — MeshNet 全球加速
// curl -fsSL https://meshnet.kisectool.com | bash

const GH = "https://github.com/xiaqijun/mesnet/releases/latest/download";

export default {
  async fetch(request) {
    const url = new URL(request.url);
    const path = url.pathname;

    // 安装脚本
    if (path === "/" || path === "/install" || path === "/install.sh") {
      return new Response(INSTALL_SCRIPT, {
        headers: {
          "Content-Type": "text/plain; charset=utf-8",
          "Cache-Control": "public, max-age=3600",
        },
      });
    }

    // 二进制 / 前端包 — 从 GitHub 拉取，边缘缓存 24h，透传 Content-Length
    const target = `${GH}${path}`;
    const res = await fetch(target);
    if (!res.ok) return new Response("Not Found", { status: 404 });

    return new Response(res.body, {
      headers: {
        "Content-Type": "application/octet-stream",
        "Content-Length": res.headers.get("Content-Length") || "",
        "Cache-Control": "public, max-age=86400",
      },
    });
  },
};

const INSTALL_SCRIPT = `#!/bin/bash
set -e

BASE="https://meshnet.kisectool.com"

echo ">>> 下载控制端..."
curl -# -L -o /usr/local/bin/mesnet-server "\${BASE}/mesnet-server"
chmod +x /usr/local/bin/mesnet-server

echo ">>> 下载前端..."
curl -# -L "\${BASE}/mesnet-web.tar.gz" | tar xz -C /etc/mesnet/web

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

IP=\$(curl -s ifconfig.me 2>/dev/null || hostname -I | awk '{print \$1}')
echo ">>> 完成! http://\${IP}:8080"
`;
