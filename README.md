# 2FAuth Go + React UI

一个面向自托管场景的 2FAuth 重实现项目：使用 Go 提供兼容 2FAuth v9 的 HTTP API、会话认证、OTP 生成和数据存储，并使用 React、TypeScript 与 React Three Fiber 提供现代化管理界面。

> 本项目包含实验性重实现内容。部署到公网前，请阅读下面的「安全设计」，并完成你自己的依赖检查和端到端验证。

## 项目结构

```text
.
├── 2fauth-go/       Go 后端、API、数据库、认证和 OTP 逻辑
├── 2fauth-ui/       React + TypeScript 前端面板
├── 2fauth-android/  原生 Android 客户端（Kotlin + Jetpack Compose）
├── Dockerfile       构建前后端的多阶段镜像
└── .github/         CI：测试、三种数据库、漏洞扫描
```

后端可以兼容上游 2FAuth v9 的主要 `/api/v1` JSON 接口。前端构建后由 Go 服务提供；如果没有部署 React 构建产物，服务也可以回退到上游 Vue 前端资源。

## 主要功能

- TOTP、HOTP 和 Steam TOTP
- OTP 账户的创建、编辑、删除、分组、收藏和排序
- 多格式导入：2FAuth、Aegis（含加密）、2FAS（含加密）、Google Authenticator 迁移、Bitwarden、andOTP、FreeOTP+、Authenticator Pro、Raivo、LastPass、Proton Authenticator、CSV、otpauth 列表，详见 [`docs/IMPORT.md`](2fauth-go/docs/IMPORT.md)
- 加密导出（argon2id + AES-256-GCM），也可以导出明文 JSON 或 otpauth 列表
- SMTP 发信：密码重置、邮箱确认、新设备登录、登录失败、密码修改、共享和转移通知，以及测试邮件
- QR Code 解析与生成
- SQLite、MySQL 和 PostgreSQL 数据库支持
- 加密会话 Cookie、CSRF 防护和 Personal Access Token
- 用户注册、登录、密码修改、找回密码、邮箱确认和管理员管理
- 账户共享及 OTP 生成日志
- OpenID Connect 登录、PKCE 和 provider discovery
- 图标上传、删除和可配置的图标 provider chain；没有图标的账户会在后台自动获取
- 通过 SSH 公钥在命令行读取验证码，每个密钥有自己的权限范围，详见 [`docs/SSH.md`](2fauth-go/docs/SSH.md)
- 给 AI agent 和脚本的接入文档：服务器在 `/agents.md`（同时在 `/llms.txt`）按本实例的真实地址、SSH 端口和主机指纹生成，网页 `/agents` 是给人看的版本
- 原生 Android 应用：离线可用的本机验证码、与服务器同步的验证码，以及只在云端的验证码；主题和卡片样式与网页一致，网页端发布更新、手机自动提示，详见 [`2fauth-android/README.md`](2fauth-android/README.md)
- 五种卡片样式（经典色条、顶部微光、双层边框、胶囊标签、便当盒），以及未登录访客看到的项目介绍首页
- React 面板中的列表、网格和 3D 展示模式；验证码默认隐藏，鼠标移上去显示 5 秒，可一键全部显示
- 主题编辑、语言切换和本地主题配置

完整的接口兼容性说明见 [`2fauth-go/docs/COMPATIBILITY.md`](2fauth-go/docs/COMPATIBILITY.md)。

## 环境要求

- Go 1.26.10 或更高版本
- Node.js 18+，推荐使用 npm
- SQLite 可直接使用；MySQL 或 PostgreSQL 需要单独准备数据库服务

## 快速开始

### 1. 构建前端

```bash
cd 2fauth-ui
npm ci
npm run build
```

构建结果会写入 `2fauth-ui/dist/`。

### 2. 配置后端

```bash
cd ../2fauth-go
cp .env.example .env
```

至少需要设置 `APP_KEY`。可以使用 Go 命令生成，它会把 `.env` 权限设为 600；已有密钥时默认拒绝覆盖：

```bash
go run ./cmd/2fauth key:generate
```

默认使用 SQLite。生产环境请确认以下配置：

```dotenv
APP_ENV=production
APP_DEBUG=false
APP_URL=https://example.com
SESSION_SECURE_COOKIE=true
DB_CONNECTION=sqlite
```

不要把真实 `.env`、`APP_KEY`、数据库文件或包含 OTP secret 的导出文件提交到 Git。根目录 `.gitignore` 已默认忽略这些本地文件。

### 3. 构建并运行后端

```bash
go build -o 2fauth ./cmd/2fauth

# 让服务使用 React 构建产物
UI_ASSETS_PATH=../2fauth-ui/dist ./2fauth
```

默认监听 `:8000`。可以使用 `LISTEN_ADDR`、`SERVER_PORT` 或 `PORT` 修改监听地址：

```bash
LISTEN_ADDR=127.0.0.1:8000 UI_ASSETS_PATH=../2fauth-ui/dist ./2fauth
```

然后访问 <http://localhost:8000>。首次注册的用户会成为管理员。

也可以直接构建容器：

```bash
docker build -t 2fauth .
docker run -p 8000:8000 -v 2fauth-data:/data -e APP_KEY=base64:... -e APP_URL=http://localhost:8000 2fauth
```

### 命令行读取验证码

在 `.env` 中设置 `SSH_LISTEN_ADDR=:2222` 并重启，然后在「设置 → 安全 → SSH 密钥」添加机器的公钥，并选择它能读取哪些账户。之后这台机器无需安装任何软件：

```bash
ssh -p 2222 otp@your-server get github
```

详见 [docs/SSH.md](2fauth-go/docs/SSH.md)。`ALLOWED_CLIENT_IPS` 同样限制这个端口。如果客户端要通过别的地址或端口才能连到 SSH 服务（隧道、端口转发），设置 `SSH_PUBLIC_ADDR=host:port`，面板和 agent 文档里显示的就是这个地址。

### 让 AI agent 取验证码

把 `https://your-server/agents.md` 交给 agent（网页 `/agents` 上有「复制给 agent 的指令」按钮）。文档会告诉它如何生成自己的 SSH 密钥、请你登记公钥并选择可读的账户，以及如何调用命令和处理过期、HOTP 计数器等细节。agent 自己无法获得权限，必须由你在「设置 → 安全」里授予 SSH 密钥（推荐，只读且限定范围）或个人访问令牌（拥有账户全部权限）。

### 只允许特定地址访问

`ALLOWED_CLIENT_IPS` 接受地址或 CIDR 网段，名单外的连接在 TLS 和 HTTP 之前就被关闭。它看的是 TCP 对端地址，不看任何请求头；放在反向代理后面时，这里要写代理的地址。

```dotenv
LISTEN_ADDR=192.168.1.237:8444
ALLOWED_CLIENT_IPS=192.168.1.118
TLS_CERT_FILE=/path/to/cert.pem
TLS_KEY_FILE=/path/to/key.pem
APP_URL=https://192.168.1.237:8444
```

### 忘记管理员密码

```bash
2fauth user:reset-password admin@example.com
```

命令会打印一个新的随机密码，并结束该账户的所有会话和访问令牌。

## 开发流程

后端：

```bash
cd 2fauth-go
go test ./...
go vet ./...
gofmt -l .
```

前端：

```bash
cd 2fauth-ui
npm ci
npm run typecheck
npm run build
npm run smoke
npm run dev
```

前端开发服务器通常只用于开发 React UI。完整运行时仍需要 Go 服务提供 API 和 `window.__DSH_BOOT__` 等启动配置。

## 配置说明

后端配置模板位于 [`2fauth-go/.env.example`](2fauth-go/.env.example)，其中包含：

- 应用 URL、子目录和静态资源路径
- 数据库连接参数
- Session、CSRF、登录限流和反向代理认证
- OIDC issuer、client ID、client secret 和登录行为
- 邮件（也可在面板「设置 > 邮件」中修改）、WebAuthn 和图标 provider 配置
- 来源地址白名单（`ALLOWED_CLIENT_IPS`）和原生 TLS（`TLS_CERT_FILE`、`TLS_KEY_FILE`）
- 用户偏好默认值及锁定策略

建议生产环境将应用放在 HTTPS 反向代理后，并正确配置 `APP_URL`、`SESSION_SECURE_COOKIE` 和可信代理。不要仅依赖客户端传入的 `X-Forwarded-*` 请求头来实现安全策略。

## 数据迁移

项目数据库结构尽量保持与上游 2FAuth v9 一致。推荐使用上游导出的 JSON 在新实例中导入账户。

如果直接复用已有 SQLite 数据库，需要同时使用原实例的 `APP_KEY`，否则无法解密既有的加密字段。首次启动时，明文保存的 OTP secret 会被加密，所以迁移前请备份数据库，之后验证账户数量、OTP secret 和 HOTP counter。

## 安全设计

- **密钥加密**：OTP 密钥、SMTP 密码和 SSO 客户端密钥用 `APP_KEY` 加密存储，启动时会自动加密旧的明文记录。`APP_KEY` 要和数据库分开备份，丢失后密钥无法恢复。
- **会话**：会话记录在服务端。登出、改密码和管理员重置密码都会立即生效，改密码还会吊销所有个人访问令牌。
- **SSO**：严格校验 ID Token 的签名、`iss`、`aud`、`exp` 和 `nonce`，只信任 `email_verified=true` 的邮箱。把未验证邮箱的本地账户关联到 SSO 时，会同时作废它原有的密码、会话和令牌，防止抢注。
- **上传**：图标按内容判断类型，SVG 按白名单重写；图标响应带 `nosniff` 和沙箱 CSP。
- **共享**：共享只授予生成验证码的权限，接收者看不到也导出不了密钥。
- **限流**：登录按地址和账户分别限流，注册、找回密码、导入和 API 也有限额。`X-Forwarded-For` 只对 `TRUSTED_PROXIES` 中的对端生效。

部署到公网时：

- 使用 HTTPS，并正确设置 `APP_URL`。
- 确认注册策略：默认开放注册，第一个注册的用户是管理员。
- 定期备份数据库和 `APP_KEY`。

发现安全问题时，请不要在公开 Issue 中发布凭据、APP_KEY、数据库或 OTP secret。

## 许可证

本项目及相关前端兼容代码采用 AGPL-3.0。请同时确认上游 2FAuth 和第三方依赖各自的许可证要求。
