# Clash Subscription Parser

[English](README.md) | 简体中文

Clash Subscription Parser 是一个单容器 Web 应用，用于可视化管理 Clash 兼容配置。它支持订阅、策略组、规则、DNS，并可生成 YAML / Script.js 输出。

运行时数据保存在数据库中。私有迁移素材、本地备份、真实订阅 URL、token 和生成后的完整配置不应进入公开仓库。

## Demo

仓库内置了一个位于 `demo/` 的静态 GitHub Pages Demo。启用 GitHub Pages 并选择 "GitHub Actions" 作为发布来源后，Demo 地址通常是：

```text
https://<your-github-username>.github.io/clash-sub-parser/
```

该 Demo 只使用示例数据，不运行 FastAPI 后端，也不包含真实订阅 URL、token、节点或生成配置。

## 功能

- 订阅管理：支持定时拉取、手动拉取、YAML/Base64 解析。
- 节点筛选：支持正则粗筛、手动包含、手动排除。
- 策略组管理：支持静态节点、内建目标、节点组引用、节点组节点展开和动态正则匹配。
- 规则管理：支持分类、排序、搜索、草稿编辑和移动端卡片编辑。
- DNS 管理：支持可视化字段和 Raw YAML。
- 配置生成：支持 Clash YAML、Script.js、`/yaml` 和 `/script` 短订阅地址。
- 安全设置：支持运行时开启 Token 保护，可分别保护 Web UI、管理 API 和导出接口。
- 部署：支持 Docker Compose 单容器部署。

## 技术栈

| 层级 | 技术 |
| --- | --- |
| 后端 | Python 3.10 / FastAPI |
| ORM | SQLAlchemy Async |
| Schema | Pydantic |
| 前端 | Vue 3 + Vite |
| 数据库 | SQLite 默认，PostgreSQL 可选 |
| 定时任务 | APScheduler |
| 部署 | Docker Compose |
| 测试 | pytest |

## 快速启动

```bash
cp docker-compose.example.yml docker-compose.yml
docker compose up -d --build
```

默认入口：

- Web UI 和 API：`http://127.0.0.1:18080`
- 健康检查：`http://127.0.0.1:18080/health`
- YAML 输出：`http://127.0.0.1:18080/yaml`
- Script 输出：`http://127.0.0.1:18080/script`

默认 Compose 端口只绑定到 `127.0.0.1`。如果确实要暴露到其它网卡，需要显式设置 `CLASH_PORT` / `CLASH_COMPAT_PORT`。

常用检查：

```bash
docker compose ps
curl --noproxy '*' http://127.0.0.1:18080/health
```

## 安全部署

如果服务会被局域网或公网访问，不建议直接暴露默认未鉴权配置。

创建本地 `.env`：

```bash
cp .env.example .env
```

建议至少配置：

```text
CLASH_AUTH_ENABLED=true
CLASH_AUTH_TOKEN=<长随机 token>
CLASH_PORT=127.0.0.1:18080
CLASH_COMPAT_PORT=127.0.0.1:20000
```

可以用下面的命令生成随机 token：

```bash
openssl rand -base64 32
```

然后通过 HTTPS 反向代理访问，或仅绑定到可信本机接口。
如果 Web UI 通过 HTTPS 访问，建议设置 `CLASH_AUTH_COOKIE_SECURE=true`。

安全默认值：

- 订阅拉取只允许 `http` 和 `https`。
- 默认拒绝 localhost、私网、链路本地、多播、保留和未指定地址。
- 每次重定向前都会重新校验目标地址。
- 订阅响应大小受 `CLASH_REQUEST_MAX_BYTES` 限制。
- 如需拉取内网订阅源，必须显式设置 `CLASH_ALLOW_PRIVATE_FETCH_URLS=true`。
- 开启导出保护后，订阅地址需要携带 `?token=...`，因为 Clash 客户端通常无法发送自定义 header。
- Web UI 登录使用 HttpOnly cookie，前端不会把 token 持久化到本地存储。
- 使用 cookie 认证的管理请求需要 `X-Clash-CSRF: 1` header；内置前端会自动携带。

## 配置

常用环境变量：

```text
CLASH_DATABASE_URL=sqlite+aiosqlite:////data/clash_sub_parser.db
CLASH_REQUEST_TIMEOUT_SECONDS=30
CLASH_REQUEST_MAX_BYTES=5242880
CLASH_REQUEST_USER_AGENT=ClashforWindows/0.20
CLASH_REQUEST_TRUST_ENV=false
CLASH_ALLOW_PRIVATE_FETCH_URLS=false
CLASH_DEFAULT_PROXY_TEST_URL=http://www.gstatic.com/generate_204
CLASH_SCHEDULER_ENABLED=true
CLASH_AUTH_ENABLED=false
CLASH_AUTH_TOKEN=
CLASH_AUTH_COOKIE_SECURE=false
```

需要使用国内构建镜像源时，先取消 `docker-compose.example.yml` 中可选 `build.args` 的注释，再复制为本地 `docker-compose.yml`。

```bash
cp docker-compose.example.yml docker-compose.yml
docker compose up -d --build
```

订阅拉取代理：

```text
CLASH_HTTP_PROXY=
CLASH_HTTPS_PROXY=
CLASH_ALL_PROXY=
CLASH_NO_PROXY=localhost,127.0.0.1
```

Compose 文件使用 `CLASH_*` 代理变量，再映射为运行时代理变量，避免宿主机普通 `HTTP_PROXY` 被意外写入部署配置。

## 本地开发

后端：

```bash
cd backend
python3.10 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
python -m app.main
```

前端：

```bash
cd frontend
npm install
npm run dev
```

前端开发服务器默认把 `/api` 代理到 `http://localhost:18080`。如后端地址不同：

```bash
VITE_API_PROXY_TARGET=http://127.0.0.1:18080 npm run dev
```

## 文档

- [项目计划书](project.md)
- [设计说明](docs/design.zh-CN.md)
- [路线图与发布约束](docs/roadmap.zh-CN.md)
- [贡献指南](CONTRIBUTING.md)
- [安全策略](SECURITY.md)

## 测试与构建

后端测试：

```bash
bash scripts/test_py310.sh
```

前端构建：

```bash
cd frontend
npm run build
```

Docker 构建：

```bash
cp docker-compose.example.yml docker-compose.yml
docker compose up -d --build
```

## API 概览

```text
GET    /api/subscriptions
POST   /api/subscriptions
PATCH  /api/subscriptions/{id}
DELETE /api/subscriptions/{id}
POST   /api/subscriptions/{id}/fetch

GET    /api/node-groups
POST   /api/node-groups
PATCH  /api/node-groups/{id}
DELETE /api/node-groups/{id}
GET    /api/node-groups/_preview
POST   /api/node-groups/validate

GET    /api/rule-categories
POST   /api/rule-categories
PATCH  /api/rule-categories/{id}
DELETE /api/rule-categories/{id}

GET    /api/rules
POST   /api/rules
PATCH  /api/rules/{id}
DELETE /api/rules/{id}

GET    /api/dns
PATCH  /api/dns

GET    /api/generate/settings
PATCH  /api/generate/settings
POST   /api/generate/yaml
POST   /api/generate/script
GET    /yaml
GET    /script
```

## 仓库清洁

不要提交：

- `.env` 或真实部署配置。
- SQLite/PostgreSQL dump、备份、缓存或运行日志。
- 真实订阅 URL、访问 token、代理节点凭据或生成后的完整 Clash 配置。
- 私有迁移脚本、私有规则素材或本地记录。
- `frontend/node_modules/`、`frontend/dist/`、虚拟环境和测试/构建缓存。

公开配置模板使用 `.env.example`。

## 致谢

本项目面向 Clash 兼容生态构建，并与以下项目或工具相关 / 受其启发：

- [Clash Meta / Mihomo](https://github.com/MetaCubeX/mihomo)
- [Clash Verge Rev](https://github.com/clash-verge-rev/clash-verge-rev)
- [OpenClaw](https://github.com/openclaw/openclaw)
- [OpenAI Codex](https://github.com/openai/codex)

列出这些项目不代表其对本项目背书，也不代表存在官方关联。

## License

MIT
