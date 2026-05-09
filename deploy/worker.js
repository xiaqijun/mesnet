// Cloudflare Worker — MeshNet 全球加速安装
// curl -fsSL https://meshnet.<your>.workers.dev | bash

export default {
  async fetch(request) {
    const url = new URL(request.url);
    const path = url.pathname;
    const BASE = "https://github.com/xiaqijun/mesnet/releases/latest/download";

    // 安装脚本
    if (path === "/" || path === "/install" || path === "/install.sh") {
      const script = await fetch(
        "https://raw.githubusercontent.com/xiaqijun/mesnet/master/deploy/install-server.sh"
      );
      return new Response(await script.text(), {
        headers: {
          "Content-Type": "text/plain; charset=utf-8",
          "Cache-Control": "public, max-age=3600",
        },
      });
    }

    // Agent 二进制
    if (path.startsWith("/agent")) {
      const bin = path.replace("/agent", "mesnet-agent-linux-amd64");
      return Response.redirect(`${BASE}/${bin}`, 302);
    }

    // 其他文件直连 GitHub
    return Response.redirect(`${BASE}${path}`, 302);
  },
};
