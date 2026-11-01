# 2FAuth Go + React UI

一个面向自托管场景的 2FAuth 重实现项目：使用 Go 提供兼容 2FAuth v9 的 HTTP API、会话认证、OTP 生成和数据存储，并使用 React、TypeScript 与 React Three Fiber 提供现代化管理界面。

> 本项目包含实验性重实现内容。部署到公网前，请先完成安全审计、依赖检查和端到端验证。

## 项目结构

```text
.
├── 2fauth-go/       Go 后端、API、数据库、认证和 OTP 逻辑
├── 2fauth-ui/       React + TypeScript 前端面板
└── .gitignore
```

后端可以兼容上游 2FAuth v9 的主要 `/api/v1` JSON 接口。前端构建后由 Go 服务提供；如果没有部署 React 构建产物，服务也可以回退到上游 Vue 前端资源。

## 主要功能

- TOTP、HOTP 和 Steam TOTP
- OTP 账户的创建、编辑、删除、分组、收藏、排序和导入导出
- QR Code 解析与生成
- SQLite、MySQL 和 PostgreSQL 数据库支持
- 加密会话 Cookie、CSRF 防护和 Personal Access Token
- 用户注册、登录、密码修改和管理员管理
- 账户共享及 OTP 生成日志
- OpenID Connect 登录、PKCE 和 provider discovery
- 图标上传、删除和可配置的图标 provider chain
- React 面板中的列表、网格和 3D 展示模式
- 主题编辑、语言切换和本地主题配置

完整的接口兼容性说明见 [`2fauth-go/docs/COMPATIBILITY.md`](2fauth-go/docs/COMPATIBILITY.md)。

## 环境要求

- Go 1.26 或更高版本
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

至少需要设置 `APP_KEY`。可以使用 Go 命令生成：

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
- 邮件、WebAuthn 和图标 provider 配置
- 用户偏好默认值及锁定策略

建议生产环境将应用放在 HTTPS 反向代理后，并正确配置 `APP_URL`、`SESSION_SECURE_COOKIE` 和可信代理。不要仅依赖客户端传入的 `X-Forwarded-*` 请求头来实现安全策略。

## 数据迁移

项目数据库结构尽量保持与上游 2FAuth v9 一致。推荐使用上游导出的 JSON 在新实例中导入账户。

如果直接复用已有 SQLite 数据库，需要同时使用原实例的 `APP_KEY`，否则无法解密既有的加密字段。迁移前请备份数据库，并验证账户数量、OTP secret 和 HOTP counter。

## 安全状态

本仓库当前仍处于审计和验证阶段。特别是在公开部署前，请重点复核：

- OIDC 回调是否严格验证 ID Token 签名、issuer、audience、有效期、nonce 和 `email_verified`
- 反向代理信任边界及 `X-Forwarded-For` 的处理
- 多数据库方言下的迁移语句
- 上传请求体大小、文件内容类型和存储目录权限
- 生产环境的 HTTPS、Cookie、备份和日志策略

发现安全问题时，请不要在公开 Issue 中发布凭据、APP_KEY、数据库或 OTP secret。

## 许可证

本项目及相关前端兼容代码采用 AGPL-3.0。请同时确认上游 2FAuth 和第三方依赖各自的许可证要求。
