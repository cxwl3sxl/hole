# hole — 内网穿透工具

复刻 ngrok 核心功能，全链路基于 HTTP/WS 协议，支持原始 TCP 字节透传。

```
Browser ──HTTP/HTTPS──► Nginx ──HTTP──► Server(:8080) ──WS──► Client ──TCP/TLS──► Local Service
```

---

## 快速开始

### 1. 部署服务端

在公网服务器上，准备好泛域名 DNS（如 `*.84.pj-local.cn` 指向该服务器 IP）。

```bash
# 修改配置中的 domain、global_token
vim configs/server.yaml

# 启动
./megad -config configs/server.yaml
```

### 2. 部署客户端

在内网机器上执行：

```bash
# 修改配置中的 address、token、proxy
vim configs/client.yaml

# 启动
./mega -config configs/client.yaml
```

### 3. 验证

```bash
curl http://s120.84.pj-local.cn:8080/
```

---

## 配置参考

### 服务端 `server.yaml`

```yaml
server:
  addr: ":8080"                # 监听地址
  domain: "84.pj-local.cn"        # 泛域名（用于子域名提取）
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

### 客户端 `client.yaml`

```yaml
server:
  address: "84.pj-local.cn:8080"   # 服务器地址
  tls: false                       # 是否走 WSS（客户端→服务器）
  token: "sk_global_secret"        # 与服务端 global_token 一致

# 子域名 → 目标服务映射
# 简写格式（HTTP 目标）：
#   myapp: "127.0.0.1:3000"
# 完整格式（支持 TLS）：
proxy:
  s120:
    target: "192.168.0.36:8702"
    tls: true                      # 目标服务是否 HTTPS

heartbeat:
  interval: 30
```

### 命令行快速模式

```bash
# 配置文件 + 命令行覆盖子域名和目标
mega -config configs/client.yaml \
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
   ── 提取 Host 头中的子域名
   ── 查询路由表 → 找到对应隧道会话
   ── Hijack TCP 连接 → 重建 HTTP 请求字节
   ── 通过 WebSocket 隧道发送 TUNNEL_OPEN + TUNNEL_DATA 帧

③ 客户端收到帧
   ── TUNNEL_OPEN → 连接到目标服务（支持 TCP 或 TLS）
   ── TUNNEL_DATA → 转发到目标服务的 TCP 连接
   ── 从目标服务读取响应 → 封装为 TUNNEL_DATA 发回服务器

④ 服务器收到响应帧
   ── 写入浏览器 TCP 连接
   ── 浏览器收到完整的 HTTP 响应
```

### 帧协议

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
| `0x01` | TUNNEL_OPEN | S→C | 通知客户端有新的连接到达 |
| `0x02` | TUNNEL_DATA | 双向 | 传输原始 TCP 字节流 |
| `0x03` | TUNNEL_CLOSE | 双向 | 关闭某个连接 |
| `0x04` | PING | C→S | 心跳请求 |
| `0x05` | PONG | S→C | 心跳响应 |

### TLS 支持

| 分段 | 说明 |
|------|------|
| 浏览器 → 服务器 | 建议前置 Nginx/Caddy 终结 TLS，转发明文 HTTP 到 Server |
| 服务器 → 客户端 | 隧道 WebSocket 可启用 WSS（配置 `tls: true`） |
| 客户端 → 目标 | 目标服务 HTTPS 时配置 `tls: true`，客户端自动走 `tls.Dial` |

---

## 构建

```bash
make          # 编译 megad 和 mega
make server   # 仅编译服务端
make client   # 仅编译客户端
make test     # 运行测试
```

---

## 部署建议

生产环境推荐架构：

```
                          ┌─ 公网 ─────────────────────────┐
                          │  Nginx (443) 终结 TLS           │
                          │  ↓ 转发明文 HTTP                │
                          │  hole-server (:8080)            │
                          └──────────┬──────────────────────┘
                                     │ WebSocket 隧道
                          ┌──────────▼──────────────────────┐
                          │ 内网                             │
                          │  hole-client                     │
                          │  ↓ TCP/TLS                       │
                          │  本地 HTTPS 服务 (:8702)          │
                          └─────────────────────────────────┘
```

Nginx 配置示例：

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
