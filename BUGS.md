# MeshNet 开发守则

## 致命 Bug 清单（每个都踩过）

### 1. 版本号：永远只改一个文件
❌ `internal/version/version.go` 是唯一版本源。禁止在 main.go、update.go、worker.js 里硬编码版本。
✅ CI 通过 `-ldflags "-X .../version.Current=$VER"` 注入。
✅ Worker 手动改 `const VER = "v0.0.0"`，发版时别忘了。

### 2. GORM bool 字段：禁止用 default tag
❌ `gorm:"default:true"` 会让 `false` 变成 `true`（Go 零值被 GORM 覆盖）。
✅ 在 handler 里显式设置默认值。

### 3. Go 文件名：禁止 `_test.go` 后缀给非测试文件
❌ `ssh_test.go` 被 Go 编译器当作测试文件跳过。
✅ 普通文件用 `sshcheck.go` 或其他名字。

### 4. fmt.Sprintf 里的 `%`：单层转义
❌ `%%s` 会产生字面量 `%s`，不会插入值。
✅ 普通字符串用 `%s`。只有在 printf 格式里嵌套字面 `%` 时才用 `%%`。

### 5. Gin 路由参数：用 `c.Param`，别用 `r.PathValue`
❌ `r.PathValue("token")` 在 Gin 的原始 Request 里永远为空。
✅ 从 `r.URL.Path` 手动解析，或使用 Gin 的 `c.Param()`。

### 6. WebSocket 协议：ws vs wss
❌ 控制端只监听了 HTTP(:8080)，Agent 用 `wss://` 连不上。
✅ 无 TLS 时用 `ws://`。

### 7. 部署脚本里的地址：禁止占位符
❌ `YOUR_SERVER` 占位符忘记替换，Agent 连到不存在的地址。
✅ 用 `c.Request.Host` 动态获取。

### 8. SSH 认证：必须支持 keyboard-interactive
❌ 只支持 password 认证，大部分云服务器用 keyboard-interactive。
✅ `ssh.KeyboardInteractive()` 作为 fallback。

### 9. Git http.postbuffer：上限 50MB
❌ `http.postbuffer=5242880000`（5GB）导致每次 push 都超时。
✅ `git config --global http.postbuffer 52428800`

### 10. 日志审计：所有错误都要记录
✅ 用 `logwatch.Error(source, msg)` 记录所有关键错误。
✅ `/api/logs` 端点 + `/audit` 页面实时查看。

## 发版流程
```bash
# 1. 改版本号（只改这里）
vim internal/version/version.go  # Current = "vX.Y.Z"

# 2. 提交 + 打 tag
git add -A && git commit -m "release: vX.Y.Z"
git tag -a vX.Y.Z -m "vX.Y.Z"
git push origin master && git push origin vX.Y.Z

# 3. CI 自动构建 Release 后，手动更新 Worker
vim deploy/worker.js  # const VER = "vX.Y.Z"
npx wrangler deploy --config wrangler.toml

# 4. 更新云服务器
curl -fsSL https://meshnet.kisectool.com | bash
```
