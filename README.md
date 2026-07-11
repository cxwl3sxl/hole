# 虫洞 — 内网穿透工具

![logo](assets/logo.png)

复刻 ngrok 核心功能，全链路基于 **HTTP / WebSocket** 协议，支持 **原始 TCP 字节透传**。

```
Browser ──HTTP/HTTPS──► Nginx ──HTTP──► whd(:8080) ──WS──► wh ──TCP/TLS──► Local Service
```

---

## 快速开始

### 1. 部署服务端

在公网服务器上准备好 **泛域名 DNS**（例如 `*.84.pj-local.cn` 指向服务器 IP）。

```bash
# 修改配置中的 domain、global_token
vim configs/server.yaml

# 启动服务端
./whd -config configs/server.yaml
```

---

### 2. 部署客户端

在内网机器上执行：

```bash
# 修改配置中的 address、token、proxy
vim configs/client.yaml

# 启动客户端
./wh -config configs/client.yaml
```

---

### 3. 验证隧道是否正常工作

```bash
curl http://s120.84.pj-local.cn:8080/
```

---

## 配置参考

### 服务端配置：`server.yaml`

```yaml
server:
  addr: ":8080"                  # 监听地址
  domain: "84.pj-local.cn"       # 泛域名（用于子域名提取）
  tunnel_path: "/_tunnel/"       # WebSocket 升级路径
  idle_timeout: 300              # 连接空闲超时（秒）
  max_tunnels: 1000              # 最大隧道连接数

auth:
  global_token: "sk_global_secret"   # 全局共享 Token

heartbeat:
  interval: 30                  # 心跳间隔（秒）
  timeout_multiplier: 3         # 超时倍数（连续 N 次未收到断开）

logging:
  level: "info"                 # debug | info | warn | error
```

---

### 客户端配置：`client.yaml`

```yaml
server:
  address: "84.pj-local.cn:8080"   # 服务端地址
  tls: false                       # 是否启用 WSS（客户端→服务端）
  tunnel_path: "/_tunnel/"         # WebSocket 升级路径（需与服务端一致）
  token: "sk_global_secret"        # 与服务端 global_token 一致

# 子域名 → 目标服务映射
# 简写格式（HTTP 目标）：
#   myapp: "127.0.0.1:3000"
# 完整格式（支持 TLS）：
proxy:
  s120:
    target: "192.168.0.36:8702"
    tls: true                      # 目标服务是否为 HTTPS

heartbeat:
  interval: 30
```

---

### 命令行快速模式

```bash
wh -config configs/client.yaml \
   -subdomain s120 \
   -target 192.168.0.36:8702 \
   -tls              # 目标服务走 TLS
```

---

## 架构说明

### 通信流程

```
① 客户端通过 HTTP Upgrade 建立 WebSocket 隧道连接服务器
   ── 认证：Authorization: Bearer sk_xxx
   ── 注册子域名：X-Tunnel-Subdomains: s120

② 外部请求到达服务器
   ── 从 Host 头提取子域名
   ── 查询路由表 → 找到对应隧道会话
   ── Hijack TCP 连接 → 重建 HTTP 请求字节
   ── 通过 WebSocket 隧道发送 TUNNEL_OPEN + TUNNEL_DATA 帧

③ 客户端收到帧
   ── TUNNEL_OPEN → 连接目标服务（TCP 或 TLS）
   ── TUNNEL_DATA → 转发到目标服务的 TCP 连接
   ── 读取目标服务响应 → 封装为 TUNNEL_DATA 发回服务器

④ 服务器收到响应帧
   ── 写入浏览器 TCP 连接
   ── 浏览器收到完整 HTTP 响应
```

---

## 帧协议

二进制帧格式（41 字节定长头 + 变长载荷）：

```
┌─────┬──────────┬──────────┬─────────────┐
│类型 │  连接ID   │  载荷长度 │    载荷      │
│1字节│ 36字节   │  4字节   │ 变长         │
│     │ (UUID)   │ (大端)   │              │
└─────┴──────────┴──────────┴─────────────┘
```

| 类型 | 名称 | 方向 | 说明 |
|------|------|------|------|
| `0x01` | TUNNEL_OPEN | S→C | 新连接到达 |
| `0x02` | TUNNEL_DATA | 双向 | 原始 TCP 字节流 |
| `0x03` | TUNNEL_CLOSE | 双向 | 关闭连接 |
| `0x04` | PING | C→S | 心跳请求 |
| `0x05` | PONG | S→C | 心跳响应 |

---

## TLS 支持

| 分段 | 说明 |
|------|------|
| 浏览器 → whd | 建议使用 Nginx/Caddy 终结 TLS，转发明文 HTTP |
| whd → wh | WebSocket 可启用 WSS（`tls: true`） |
| wh → 目标服务 | 若目标为 HTTPS，配置 `tls: true` 自动走 `tls.Dial` |

---

## 构建

### Linux / macOS

```bash
make          # 编译 whd 和 wh
make server   # 仅编译服务端
make client   # 仅编译客户端
make test     # 运行测试
```

### Windows（PowerShell）

```powershell
.\build.ps1                 # 编译 whd 和 wh
.\build.ps1 -Target server  # 仅编译服务端
.\build.ps1 -Target client  # 仅编译客户端
.\build.ps1 -Target clean   # 清理构建产物
```

---

## 部署建议（生产环境）

```
                          ┌─ 公网 ─────────────────────────┐
                          │  Nginx (443) 终结 TLS           │
                          │  ↓ 转发明文 HTTP                │
                          │  whd (:8080)                      │
                          └──────────┬──────────────────────┘
                                     │ WebSocket 隧道
                          ┌──────────▼──────────────────────┐
                          │ 内网                             │
                          │  wh                               │
                          │  ↓ TCP/TLS                       │
                          │  本地服务 (:port)                 │
                          └─────────────────────────────────┘
```

### Nginx 配置示例

```nginx
server {
    listen 443 ssl http2;
    server_name *.84.pj-local.cn;

    ssl_certificate     /etc/nginx/certs/fullchain.pem;
    ssl_certificate_key /etc/nginx/certs/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }

    # tunnel_path 需与 server.yaml 中一致
    location /_tunnel/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```