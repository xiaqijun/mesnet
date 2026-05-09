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

## 一键安装

### 控制端 (Linux amd64)

```bash
curl -fsSL https://github.com/xiaqijun/mesnet/releases/latest/download/mesnet-server -o /usr/local/bin/mesnet-server && chmod +x /usr/local/bin/mesnet-server && curl -fsSL https://github.com/xiaqijun/mesnet/releases/latest/download/mesnet-web.tar.gz | tar xz -C /etc/mesnet/web && cat > /etc/systemd/system/mesnet-server.service <<'EOF'
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
EOF
systemctl daemon-reload && systemctl enable --now mesnet-server
```

### Agent (Linux amd64, 骨干节点)

```bash
curl -fsSL https://github.com/xiaqijun/mesnet/releases/latest/download/mesnet-agent-linux-amd64 -o /usr/local/bin/mesnet-agent && chmod +x /usr/local/bin/mesnet-agent && cat > /etc/systemd/system/mesnet-agent.service <<'EOF'
[Unit]
Description=MeshNet Agent
After=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/mesnet-agent --server wss://YOUR_SERVER/ws/agent/YOUR_TOKEN --listen :443 --name YOUR_NAME
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload && systemctl enable --now mesnet-agent
```

### Agent (叶子节点, 终端主机)

```bash
curl -fsSL https://github.com/xiaqijun/mesnet/releases/latest/download/mesnet-agent-linux-amd64 -o /usr/local/bin/mesnet-agent && chmod +x /usr/local/bin/mesnet-agent && cat > /etc/systemd/system/mesnet-agent.service <<'EOF'
[Unit]
Description=MeshNet Agent
After=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/mesnet-agent --server wss://YOUR_SERVER/ws/agent/YOUR_TOKEN --listen :443 --name YOUR_NAME --backbone=false
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload && systemctl enable --now mesnet-agent
```

### 更新

```bash
# 控制端
curl -fsSL https://github.com/xiaqijun/mesnet/releases/latest/download/mesnet-server -o /usr/local/bin/mesnet-server && chmod +x /usr/local/bin/mesnet-server && systemctl restart mesnet-server

# Agent
curl -fsSL https://github.com/xiaqijun/mesnet/releases/latest/download/mesnet-agent-linux-amd64 -o /usr/local/bin/mesnet-agent && chmod +x /usr/local/bin/mesnet-agent && systemctl restart mesnet-agent
```
