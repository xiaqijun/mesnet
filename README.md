# MeshNet

基于 Agent 的隐蔽 Mesh 网络管理系统。节点间通过加密隧道直连，控制端负责编排和监控，不转发数据。

## 架构

```
Agent ──WSS──> Control Plane (:8080)     ← 管理通道（REST API + WSS）
Agent ──WSS──> Agent (:443)              ← 数据通道（加密隧道）
```

- **控制端** — Go + Gin + GORM，负责节点编排、路由计算、故障切换
- **Agent** — 部署在目标机器上，TUN 设备 + Curve25519 ECDH + ChaCha20-Poly1305 AEAD
- **前端** — React + Vite + Tailwind CSS，实时监控和拓扑可视化

### 节点角色

| 角色 | 说明 |
|------|------|
| **骨干节点** | 监听 :443 接受连接，骨干间全互联，承载中转流量 |
| **叶子节点** | 拨号连接一个骨干，通过骨干中继到其他叶子 |

数据中继路径：叶子 → 骨干 → 骨干 → 目标叶子，BFS 计算 next-hop。

## 快速开始

### 控制端

```bash
# 构建
cd MeshNet && CGO_ENABLED=1 go build -o mesnet-server ./cmd/server

# 运行
./mesnet-server -config config.yaml
```

### Agent

```bash
# 构建
cd MeshNet && go build -o mesnet-agent ./cmd/agent

# 运行（叶子节点）
./mesnet-agent --server wss://YOUR_SERVER/ws/agent/TOKEN --name node-1

# 运行（骨干节点）
./mesnet-agent --server wss://YOUR_SERVER/ws/agent/TOKEN --name bg-1 --backbone=true --listen :443
```

### 前端开发

```bash
cd web
npm install
npm run dev   # Vite dev server on :5173，自动代理 API/WS 到 :8080
```

### Docker Compose

```bash
cd deploy && docker compose up -d
```

## 核心特性

- **加密隧道** — Curve25519 4-way DH 密钥交换 + ChaCha20-Poly1305 AEAD
- **自动编排** — Agent 上线后自动分配子网、建立隧道、同步路由
- **故障切换** — 500ms 心跳检测，自动探测最优骨干切换
- **加权骨干选择** — 按 RTT + CPU + 内存加权排序选最优
- **Web 管理界面** — 实时拓扑图、流量监控、审计日志

## 配置

参见 [config.yaml](./config.yaml)：

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  ws_port: 443

database:
  driver: "sqlite"       # sqlite | postgres | mysql
  dsn: "mesnet.db"

agent:
  secret_key: "change-me-in-production"
  virtual_network: "10.100.0.0/16"
```

## 部署

发版流程详见 [BUGS.md](./BUGS.md)。CI 自动构建后通过 Cloudflare Worker 分发：

```bash
curl -fsSL https://meshnet.kisectool.com | bash
```

## 技术栈

| 层 | 技术 |
|---|------|
| 控制端 | Go + Gin + GORM + gorilla/websocket |
| Agent | Go + TUN + ChaCha20-Poly1305 + Curve25519 |
| 前端 | React + Vite + Tailwind CSS + D3 + Recharts |
| 数据库 | SQLite (默认) / PostgreSQL / MySQL |
| 分发 | Cloudflare Worker → GitHub Releases |
