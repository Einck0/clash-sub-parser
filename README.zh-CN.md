# Clash Subscription Parser (CSP 1.0)

[English](README.md) | 简体中文

Clash Subscription Parser (CSP 1.0) 是基于 Go 1.27.1 与 Vue 3 单二进制打造的高性能代理订阅控制平面与配置编译器。支持托管多协议节点台账、运行探针健康观测证据链、可视化编排策略树，并向现代客户端（Mihomo / Clash Meta、Sing-box、Quantumult X 与 Surge）输出精准编译配置。

静态前端产物通过 Go 原生 `embed` 打包至单一可执行文件，开箱即用，零外部运行依赖，底座采用 SQLite WAL 模式。

---

## 架构与技术栈

| 层级 | 技术方案 |
| --- | --- |
| 控制平面与编译器 | Go 1.27.1 / Chi v5 / Sing-box Core |
| 前端界面 | Vue 3 + TypeScript + TailwindCSS + DaisyUI（Go 二进制内置） |
| 数据持久化 | SQLite（WAL 模式 + 内置模式版本迁移） |
| 测试与质检体系 | Go Testing、Vitest 单元测试、Playwright 真实服务端到端测试 |
| 运维部署 | 单容器 Docker Compose 或单独立可执行二进制 |

---

## 快速启动

### Docker Compose 部署

```bash
cp docker-compose.example.yml docker-compose.yml
docker compose up -d --build
```

默认访问端点：
- **控制台与管理 API**：`http://127.0.0.1:18080`（API 路由统一位于 `/api/v1/*`）
- **存活检测探针**：`http://127.0.0.1:18080/healthz`
- **就绪检测探针**：`http://127.0.0.1:18080/readyz`
- **遗留端点状态**：`GET /yaml` 与 `GET /script` 明确返回 `HTTP 410 Gone`（指引客户端切换至带版本的订阅发布链接）。

运行态健康检查：
```bash
curl -f http://127.0.0.1:18080/healthz
curl -f http://127.0.0.1:18080/readyz
```

---

## 端口分配与生产隔离纪律

- **18080 端口**：标准默认业务与控制平面端口。
- **17000 端口**：**宿主机生产映射别名契约**。宿主机上的 17000 端口已映射绑定至线上生产实例。所有自动化测试、Playwright E2E 测试及本地预览严格使用系统动态临时端口（`0`）与独立临时数据库，**严禁向 17000 或 18080 发起网络请求或注入测试数据**。

---

## 安全鉴权与生产加固

CSP 支持两种运行鉴权模式：

1. **开放模式（Open Mode，未配置 `CSP_ADMIN_TOKEN`）**：
   - 零配置私有模式，适用于受信任的本机开发或隔离的内网环境。
   - 所有管理 API 与前端页面无需凭据即可访问。

2. **保护模式（Protected Mode，已配置 `CSP_ADMIN_TOKEN`）**：
   - 任何暴露于局域网或公网的环境必须开启。
   - 前端挂载 `AuthGate` 访问控制门禁，需要输入管理令牌。
   - 管理 API 需附带 `Authorization: Bearer <token>` 或 Cookie 会话（自动注入 `X-CSRF-Token` 头）。
   - 外部订阅链接需携带发布接口返回的独立导出令牌 `?token=<export-token>`，不能使用 `CSP_ADMIN_TOKEN`。

### 配置环境变量

复制配置文件模版：
```bash
cp .env.example .env
```

生成强随机密钥：
```bash
openssl rand -hex 32
```
将生成的密钥填入 `.env` 中的 `CSP_ADMIN_TOKEN`。

生产环境下建议将服务绑定在 `127.0.0.1:18080`，并通过带有 TLS 加密的反向代理（如 Caddy 或 Nginx）对外提供服务。

---

## 环境变量速查表

在 `.env` 与 `docker-compose.yml` 中支持的环境变量：

| 变量名 | 默认值 | 作用说明 |
| --- | --- | --- |
| `CSP_CONTAINER_NAME` | `clash-sub-parser` | Docker 容器名称 |
| `CSP_PORT` | `127.0.0.1:18080` | 宿主机端口映射（默认绑定本地回环） |
| `CSP_ADDR` | `0.0.0.0:18080` | 控制面服务监听地址与端口 |
| `CSP_DB_PATH` | `/data/csp-v1.db` | SQLite 数据库文件挂载路径 |
| `CSP_ADMIN_TOKEN` | *(留空)* | 管理员令牌（非空时启用保护模式） |
| `CSP_FETCH_PROXY` | `http://host.docker.internal:7890` | 订阅拉取专属出站代理 |
| `CSP_VOLUME_NAME` | `csp-v1-data` | 专属持久化 Docker 数据卷名称 |
| `TZ` | `Asia/Shanghai` | 容器运行时区 |

---

## 本地开发与测试

### 单二进制运行
```bash
go build -o csp ./cmd/csp
./csp serve -addr 127.0.0.1:18080 -db ./data/csp-v1.db
```

### 前端开发
```bash
cd web
npm install
npm run dev
```

### 执行自动化测试套件

- **Go 单元测试与集成测试**：
  ```bash
  go test ./...
  ```

- **前端单元测试与组件测试**：
  ```bash
  cd web && npm test
  ```

- **前端 TypeScript 类型检查**：
  ```bash
  cd web && npm run type-check
  ```

- **Playwright 真实服务端到端集成测试**：
  ```bash
  cd web && npx playwright test -c e2e/playwright.config.ts
  ```

- **全流程运维与冒烟测试**：
  ```bash
  bash scripts/smoke_test.sh
  ```
