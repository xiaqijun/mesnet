// Cloudflare Worker — MeshNet 安装脚本分发
// 部署: npx wrangler deploy

export default {
  async fetch(request) {
    const url = new URL(request.url);
    const path = url.pathname;
    const cf = request.cf || {};
    const country = cf.country || "US";

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

    // Agent 下载 — 国内走 Gitee
    if (path.startsWith("/agent")) {
      const bin = path.replace("/agent", "mesnet-agent-linux-amd64");
      const isCN = country === "CN";
      const base = isCN
        ? "https://gitee.com/xiaqiqi/mesnet/releases/download/v1.0.0"
        : "https://github.com/xiaqijun/mesnet/releases/download/v1.0.0";
      return Response.redirect(`${base}/${bin}`, 302);
    }

    // 其他文件
    const isCN = country === "CN";
    const base = isCN
      ? "https://gitee.com/xiaqiqi/mesnet/raw/master"
      : "https://raw.githubusercontent.com/xiaqijun/mesnet/master";
    return Response.redirect(`${base}${path}`, 302);
  },
};
