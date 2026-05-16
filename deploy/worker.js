const GH_API = "https://api.github.com/repos/xiaqijun/mesnet/releases/latest";
const GH_DL = "https://github.com/xiaqijun/mesnet/releases/latest/download";

// Cache latest version for 5 min to avoid hitting GitHub API rate limits
let cachedVer = "";
let cachedAt = 0;

async function getLatestVersion() {
  if (cachedVer && Date.now() - cachedAt < 300000) {
    return cachedVer;
  }
  try {
    const res = await fetch(GH_API, {
      headers: { "User-Agent": "mesnet-worker", "Accept": "application/vnd.github+json" },
    });
    if (res.ok) {
      const data = await res.json();
      cachedVer = data.tag_name || cachedVer;
      cachedAt = Date.now();
    }
  } catch { /* use cached value */ }
  return cachedVer || "unknown";
}

export default {
  async fetch(request) {
    const url = new URL(request.url);
    const path = url.pathname;

    if (path === "/version") {
      const ver = await getLatestVersion();
      return new Response(ver, {
        headers: { "Content-Type": "text/plain", "Cache-Control": "no-cache" },
      });
    }

    if (path === "/" || path === "/install" || path === "/install.sh") {
      const ver = await getLatestVersion();
      return new Response(INSTALL_SCRIPT.replace("__VERSION__", ver), {
        headers: { "Content-Type": "text/plain; charset=utf-8", "Cache-Control": "public, max-age=300" },
      });
    }

    // Binary downloads: proxy to GitHub with Cloudflare edge cache
    const cache = caches.default;
    const cached = await cache.match(request);
    if (cached) return cached;

    const target = `${GH_DL}${path}`;
    const res = await fetch(target);
    if (!res.ok) return new Response("Not Found", { status: 404 });

    const response = new Response(res.body, res);
    response.headers.set("Cache-Control", "public, max-age=86400");
    const ctx = request.ctx;
    if (ctx) ctx.waitUntil(cache.put(request, response.clone()));

    return response;
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
