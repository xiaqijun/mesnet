const GH = "https://github.com/xiaqijun/mesnet/releases/latest/download";
const VER = "v1.0.5";

export default {
  async fetch(request) {
    const url = new URL(request.url);
    const path = url.pathname;

    if (path === "/" || path === "/install" || path === "/install.sh") {
      return new Response(INSTALL_SCRIPT, {
        headers: { "Content-Type": "text/plain; charset=utf-8", "Cache-Control": "public, max-age=3600" },
      });
    }

    if (path === "/version") {
      return new Response(VER, {
        headers: { "Content-Type": "text/plain", "Cache-Control": "no-cache" },
      });
    }

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

LATEST=\$(curl -s "\${BASE}/version")

CURRENT=\$(/usr/local/bin/mesnet-server --version 2>/dev/null || echo "")
if [ -n "\$CURRENT" ] && [ "\$CURRENT" = "\$LATEST" ]; then
  echo "已是最新版本 \$CURRENT，无需更新"
  exit 0
fi

echo ">>> 更新 \$CURRENT → \$LATEST"
systemctl stop mesnet-server 2>/dev/null || true

echo ">>> 下载控制端..."
curl -# -L -o /usr/local/bin/mesnet-server "\${BASE}/mesnet-server"
chmod +x /usr/local/bin/mesnet-server

echo ">>> 下载前端..."
mkdir -p /etc/mesnet/web/dist
curl -# -L "\${BASE}/mesnet-web.tar.gz" | tar xz -C /etc/mesnet/web/dist

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
