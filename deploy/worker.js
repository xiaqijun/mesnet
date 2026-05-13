const GH = "https://github.com/xiaqijun/mesnet/releases/latest/download";
const VER = "v1.0.10"; // CI auto-replaces this on release

export default {
  async fetch(request) {
    const url = new URL(request.url);
    const path = url.pathname;

    if (path === "/version") {
      return new Response(VER, {
        headers: { "Content-Type": "text/plain", "Cache-Control": "no-cache" },
      });
    }

    if (path === "/" || path === "/install" || path === "/install.sh") {
      return new Response(INSTALL_SCRIPT.replace("__VERSION__", VER), {
        headers: { "Content-Type": "text/plain; charset=utf-8", "Cache-Control": "public, max-age=300" },
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
LATEST="__VERSION__"

CURRENT=\$(/usr/local/bin/mesnet-server --version 2>/dev/null || echo "")
if [ -n "\$CURRENT" ] && [ "\$CURRENT" = "\$LATEST" ]; then
  echo "已是最新版本 \$CURRENT，无需更新"
  exit 0
fi

echo ">>> 更新 \$CURRENT → \$LATEST"
systemctl stop mesnet-server 2>/dev/null || true
curl -# -L -o /usr/local/bin/mesnet-server "\${BASE}/mesnet-server"
chmod +x /usr/local/bin/mesnet-server
mkdir -p /etc/mesnet/web/dist
curl -# -L "\${BASE}/mesnet-web.tar.gz" | tar xz -C /etc/mesnet/web/dist
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
echo ">>> 完成! http://\$(curl -s ifconfig.me 2>/dev/null || hostname -I | awk '{print \$1}'):8080"
`;
