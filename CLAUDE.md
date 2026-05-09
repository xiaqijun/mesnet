# MeshNet

基于 Agent 的隐蔽 Mesh 网络管理系统。

## 技术栈

| 层 | 技术 |
|---|------|
| 控制端 | Go + Gin + GORM + gorilla/websocket |
| Agent | Go + gorilla/websocket + TUN + Noise |
| 前端 | React + Vite + Tailwind CSS |
| 数据库 | SQLite (默认) / PostgreSQL / MySQL |

## 架构

- 控制端负责管理编排（REST API + WSS 管理通道），不转发数据
- Agent 之间 WSS 直连传输加密隧道数据
- 协议特征隐藏在标准 HTTPS 流量中

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
│   │   └── services/    # 业务逻辑
│   ├── agent/           # Agent 实现
│   └── proto/           # 协议定义
├── web/                 # React 前端
└── deploy/              # 部署文件
```

## 构建

```bash
# 控制端
go build -o mesnet-server ./cmd/server/

# Agent
go build -o mesnet-agent ./cmd/agent/

# 前端
cd web && npm install && npm run build
```

## 部署

```bash
# Agent (二进制 + systemd, 一行命令)
curl -sSL https://<cp>:8080/api/agent/binary -o /usr/local/bin/mesnet-agent && chmod +x /usr/local/bin/mesnet-agent
mesnet-agent --server wss://<cp>:443/ws/agent/<token> --listen :443 --name node1
```
