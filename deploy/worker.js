// Cloudflare Worker — MeshNet 全球加速
// curl -fsSL https://meshnet.kisectool.com | bash

const GH = "https://github.com/xiaqijun/mesnet/releases/latest/download";

export default {
  async fetch(request) {
    const url = new URL(request.url);
    const path = url.pathname;

    // 安装脚本
    if (path === "/" || path === "/install" || path === "/install.sh") {
      const res = await fetch(`${GH}/mesnet-server`, { method: "HEAD" }).catch(() => null);
      return new Response(INSTALL_SCRIPT, {
        headers: {
          "Content-Type": "text/plain; charset=utf-8",
          "Cache-Control": "public, max-age=3600",
          "X-Latest-Version": res ? res.headers.get("X-GitHub-Release") || "v1.0.0" : "unknown",
        },
      });
    }

    // 二进制 / 前端包 — 从 GitHub 拉取，边缘缓存 24h
    const target = `${GH}${path}`;
    const res = await fetch(target);
    if (!res.ok) return new Response("Not Found", { status: 404 });

    return new Response(res.body, {
      headers: {
        "Content-Type": res.headers.get("Content-Type") || "application/octet-stream",
        "Cache-Control": "public, max-age=86400",
      },
    });
  },
};

const INSTALL_SCRIPT = `#!/bin/bash
set -e

BASE="https://meshnet.kisectool.com"

echo ">>> 下载控制端..."
curl -fsSL "\${BASE}/mesnet-server" -o /usr/local/bin/mesnet-server
chmod +x /usr/local/bin/mesnet-server

echo ">>> 下载前端..."
mkdir -p /etc/mesnet/web
curl -fsSL "\${BASE}/mesnet-web.tar.gz" | tar xz -C /etc/mesnet/web

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
