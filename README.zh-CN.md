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

## 订阅更新路由

在“全局配置 → 订阅更新路由”设置下次抓取使用的路由，无需重启。`environment`（迁移默认）继承 `--fetch-proxy` / `CSP_FETCH_PROXY`；`direct` 忽略环境代理；`proxy` 使用持久化的 HTTP/HTTPS 代理。单订阅 override 优先，但私有 override 必须匹配当前信任的全局代理。订阅目标和每次重定向仍须通过本地公共 IP 校验。直连固定已验证 IP；HTTP/HTTPS 代理保留目标域名（含 CONNECT authority），由受信任代理远端解析。本地校验无法保证远端解析器实际使用的 IP。

管理接口 `GET/PUT /api/v1/settings/subscription-fetch` 沿用现有鉴权和 Cookie CSRF 保护。PUT 示例：`{"mode":"proxy","proxy_url":"http://user:password@proxy.example:7890"}`（必须显式端口）。省略 `proxy_url` 会保留已保存 URL **及凭据**；`{"mode":"direct","clear_proxy_url":true}` 明确清除；`{"mode":"environment"}` 恢复继承但不清除代理。非法请求不改变旧配置或鉴权。GET 只返回不含用户信息的配置/生效 endpoint、凭据存在标记与生效来源，不回传可用凭据。代理凭据保存在既有 SQLite settings 中，而非凭据保险库，须保护数据库及备份访问权限。

抓取失败记录保留安全的 DNS、连接、TLS、HTTP、正文和解析阶段/原因码。失败或部分成功保留旧来源关系；完整成功仅清理当前订阅已移除的关系并保留历史节点资产。代理设置不能修复不可达的上游。

订阅 `prefix` 是顶层字段，不属于高级 `config`。保存前缀与已有节点显示名更新在同一事务内完成。共享节点按来源 ID 顺序合并不同的非空前缀，采用第一来源最新原始名称；清空仅移除当前来源标签，手工显示名优先。前缀不改变原始条目、逻辑 ID、连接版本或已有发布。禁用订阅仅停止定时刷新，仍允许手动刷新。

当前节点范围包含无订阅来源证据的活跃手工节点，或经 `manual_confirmed` 明确确认的恢复资产。刷新移除关系的订阅节点仍保留在台账，但不会因失去来源边而被误认为手工节点。

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
