# MeshNet

基于 Agent 的隐蔽 Mesh 网络管理系统。

**当前版本**: v1.0.80 | **状态**: 四节点（BG-1 / HK-1 / NJ-1 / GZ-1）全部在线，加密信道互通

## 技术栈

| 层 | 技术 |
|---|------|
| 控制端 | Go + Gin + GORM + gorilla/websocket |
| Agent | Go + gorilla/websocket + TUN + ChaCha20-Poly1305 + Curve25519 |
| 前端 | React + Vite + Tailwind CSS |
| 数据库 | SQLite (默认) / PostgreSQL / MySQL |
| 二进制分发 | Cloudflare Worker (meshnet.kisectool.com) -> GitHub Releases |

## 架构

```
Agent ──WSS──> Control Plane (:8080)     ← 管理通道
Agent ──WSS──> Agent (:443)              ← 数据通道（加密隧道）
```

- **控制端** 负责管理编排（REST API + WSS 管理通道），**不转发数据**
- **Agent 之间 WSS 直连**传输加密隧道数据
- **两种角色**:
  - **骨干节点** (`--backbone=true`): 监听 :443 接受连接，骨干间全互联
  - **叶子节点** (`--backbone=false`): 只向外拨号连接一个骨干，通过骨干中继到其他叶子
- **数据中继**: 叶子 → 骨干 → 骨干 → 目标叶子，通过 BFS 计算 next-hop
- **协议自定义帧格式**: Magic "MESH" (0x4D45354E) + 16 字节 header + 负载

## 关键技术细节

### 加密协议
- **密钥交换**: Curve25519 ECDH 4-way DH（静态+临时密钥对）
- **会话密钥**: SHA256(排序后的 4 个 DH 值 + 固定 salt `mesnet-v2-session`)，取前 32 字节
- **数据加密**: ChaCha20-Poly1305 AEAD
- **安全信道状态**: Init -> HandshakeSent -> Established -> Wiped

### 故障检测与切换
- **心跳**: Agent 间 500ms WebSocket ping，2s read deadline
- **TCP keepalive**: 1s 周期，3 次探测
- **服务端扫描**: 5s 间隔 `CheckAndFailover`，检测断线或 RTT > 200ms（连续 3 次）
- **切换**: `SwitchBackbone` 调用 agent 的 `backbone_probe` 探测所有骨干选最优
- **重连处理**: `onReconnect` 重置 router send queue，`onDisconnect` 通知控制面

### onRecv 数据包处理顺序（关键！）
```go
// 1. 中继优先 — 不是自己的包先转发
if nextHop != 0 && nextHop != nodeID {
    tun.SendEncrypted(nextHop, plaintext)
    return
}
// 2. 然后检查本地子网
if isLocalIP(a.tun.IP(), dstIP) || a.isInOurSubnets(dstIP) {
    a.tun.Write(plaintext)
    return
}
```
顺序不可颠倒 — 先检查子网会吞掉中继包（VirtualIP /32 路由匹配导致写入 TUN 而非转发）。

### 自动编排流程
1. Agent 连接控制端 WebSocket (`/ws/agent/:token`)
2. 控制端 `onHello` 触发 `AutoMesh`（3s 延迟）
3. `AutoMesh`: `tun_setup` → `subnet_detect` → 找最优骨干 → `peer_accept` + `peer_connect` → `route_add`
4. 完成后 `syncAllRoutes` BFS 计算路由同步到所有节点

## 目录结构

```
MeshNet/
├── cmd/
│   ├── server/          # 控制端入口
│   └── agent/           # Agent 入口
├── internal/
│   ├── server/          # 控制端实现
│   │   ├── config/      # 配置
│   │   ├── database/    # 数据库
│   │   ├── models/      # 数据模型
│   │   ├── handlers/    # API 处理器
│   │   ├── ws/          # WebSocket 管理面
│   │   └── services/    # 业务逻辑（mesh, failover, stats）
│   ├── agent/           # Agent 实现
│   │   ├── agent.go     # 生命周期 + 命令处理
│   │   ├── peer.go      # 对等连接 WebSocket 管理 + 心跳
│   │   ├── tunnel.go    # 加密隧道（SendEncrypted/ReceiveEncrypted/Run）
│   │   ├── tun.go       # TUN 设备（raw fd，跨平台）
│   │   ├── tun_linux.go # Linux TUN 实现
│   │   ├── channel.go   # SecureChannel 密钥交换
│   │   ├── crypto.go    # AEAD 加密
│   │   ├── frame.go     # 二进制帧协议
│   │   ├── route.go     # 路由表（最长前缀匹配）
│   │   ├── route_linux.go # 内核路由
│   │   ├── router.go    # PacketRouter 发送队列
│   │   ├── ws.go        # 管理 WebSocket
│   │   ├── stats.go     # 统计收集
│   │   └── probe.go     # 延迟探测
│   └── proto/           # 协议定义
├── web/                 # React 前端
├── deploy/              # 部署文件
└── BUGS.md              # 踩坑记录 + 发版流程
```

## 关键命令

### 构建
```bash
# Agent (Linux amd64)
cd /e/github/MeshNet && go build -ldflags "-X github.com/mesnet/mesnet/internal/version.Current=1.0.78" -o mesnet-agent-linux-amd64 ./cmd/agent

# 控制端
cd /e/github/MeshNet && CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -ldflags "-X github.com/mesnet/mesnet/internal/version.Current=1.0.78" -o mesnet-server ./cmd/server

# 前端
cd /e/github/MeshNet/web && npm run build && tar czf mesnet-web.tar.gz -C dist .
```

### 标签+推送
```bash
git tag v1.0.80 && git push origin v1.0.80  # 触发 CI 构建+Release
```

### 全量部署（版本升级）
```bash
curl -fsSL https://meshnet.kisectool.com/mesnet-server -o /usr/local/bin/mesnet-server
curl -fsSL https://meshnet.kisectool.com/mesnet-agent-linux-amd64 -o /usr/local/bin/mesnet-agent
curl -fsSL https://meshnet.kisectool.com/mesnet-web.tar.gz | tar xz -C /etc/mesnet/web/dist
systemctl restart mesnet-server
systemctl restart mesnet-agent
```

### Agent 参数
```bash
--server wss://HOST/ws/agent/TOKEN   # 控制端地址
--listen :443                         # 监听端口（骨干节点）
--name BG-1                           # 节点名称
--backbone=true/false                 # 是否为骨干节点
```

## 开发守则

详见 [BUGS.md](./BUGS.md)，包含所有踩过的坑和发版流程。
