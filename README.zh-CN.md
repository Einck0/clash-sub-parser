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

推荐通过 GitHub Container Registry (GHCR) 托管的预构建云端镜像进行开箱即用部署：

```bash
cp docker-compose.example.yml docker-compose.yml
docker compose pull app && docker compose up -d --no-build
```

> **拉取与启动注意**：
> - 务必使用 `docker compose pull app && docker compose up -d --no-build`（通过 `&&` 串联），确保若远程镜像拉取失败时流程立即终止，绝不允许分号继续而静默回退至未知或陈旧的本地缓存镜像。
> - 显式携带 `--no-build` 标志，确保部署宿主机上绝不触发本地源码构建。
> - **镜像通道与版本固定**：`docker-compose.example.yml` 默认跟踪滚动预发布通道 `ghcr.io/einck0/clash-sub-parser:prelease`。由于预发布标签是可移动的渠道指针（每次通过完整云端检查并发布的 prelease 会更新该 tag），生产环境强烈建议通过环境变量固定精确的不可变摘要（如 `CSP_IMAGE=ghcr.io/einck0/clash-sub-parser@sha256:<64hex-digest>`）。
>
> **受控运维部署路径**：
> 针对生产高可靠场景，执行两阶段确权部署：
> 1. 先通过前置脚本拉取并核验镜像 OCI revision 与 RepoDigests：
>    ```bash
>    scripts/cloud-delivery/preflight-pull.sh --image <package@sha256:...> --expected-revision <git-sha>
>    ```
> 2. 验证通过后，以不可变摘要启动并指定 `--pull never`：
>    ```bash
>    CSP_IMAGE="ghcr.io/einck0/clash-sub-parser@sha256:<digest>" docker compose -f docker-compose.prod.yml up -d --no-build --pull never
>    ```
>    此处的 `--pull never` 明确建立在步骤 1 已显式完成远程拉取与 OCI 验签的基础之上，锁定使用已校验的不可变镜像，避免运行时重复拉取发生变动。
>
> **本地源码编译开发**：
> 如需在本地开发环境进行现场编译，请使用专门的开发模板 `docker-compose.dev.yml`（`docker compose -f docker-compose.dev.yml up -d --build`）。请注意：根据 CSP 云交付规范，生产服务器与 CI 宿主机严禁承担本地编译工作。

默认访问端点：
- **控制台与管理 API**：`http://127.0.0.1:18080`（API 路由统一位于 `/api/v1/*`）
- **存活检测探针**：`http://127.0.0.1:18080/healthz`
- **就绪检测探针**：`http://127.0.0.1:18080/readyz`
- **遗留端点状态**：`GET /yaml` 与 `GET /script` 明确返回 `HTTP 410 Gone`（指引客户端切换至客户端订阅直链）。

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
   - 外部订阅链接需携带独立导出令牌 `?token=<export-token>`，不能使用 `CSP_ADMIN_TOKEN`。

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

---

## 核心工作台与功能特性

- **统一节点工作台 (`/nodes`)**：单一节点账本与检测操作入口，取代独立「探针引擎」导航（历史 `/probes` 永久重定向至 `/nodes`）。支持桌面紧凑表格与移动端精简卡片自适应、跨页明确 ID 选择、按需手动探测、全库检测防误触二次确认，以及后台运行状态轮询与明细抽屉。
- **全局探针配置 (`/settings`)**：在全局配置统一管理自动探测主开关、默认周期（60..604800 秒）、检测种类（baseline、AI、流媒体）及单节点探测超时（1..30 秒）。每个运行开始时读取一次超时设置，修改对后续运行生效。自动探测关闭不影响原位手动排队。
- **每订阅源独立探测周期 (`/subscriptions`)**：订阅源可单独配置探针策略（`inherit` 继承全局、`custom` 自定义周期、`disabled` 停用探测）。多源共享节点在统一调度器中按最短有效周期计算到期。
- **策略树结构化筛选管线 (`/policy`)**：按「候选范围 → 顺序添加源 → 顺序排除源 → 实时预览」管线编排，消除复杂嵌套。实时预览精准呈现候选数、保留节点数与排除原因结构化摘要（如 `regex removed`、`filter removed`），支持全局筛选继承与终端兜底 MATCH 规则。
- **IP 探针风险筛选**：节点工作台与策略组支持按当前有效 IP 探针风险筛选：低 (0–24)、中 (25–64)、高 (65–100)、未知 (unknown) 及分数上限（`0` 有效）。未知风险不匹配分数上限；该筛选独立于既有 `risk_band` 与风险准入策略。

---

## 全局设置工作台：CSP 设置、DNS 设置与链式代理 (`/settings`, `/dns`, `/chains`)

全局设置工作台划分为三个紧密协同的子模块，可通过顶部导航栏一键切换：

### 1. CSP 基础设置 (`/settings`)
- **公网访问基准地址 (Public Base URL)**：统一维护订阅直链生成的基准 URL，支持一键填入当前浏览器访问 Origin。
- **订阅更新路由**：在线切换订阅源抓取时的出站路径（`environment` 继承环境变量、`direct` 直连、`proxy` 持久化代理），无需重启服务。
- **全局探针巡检与超时**：集中调度自动探测主开关、执行周期（60..604800 秒）、检测维度（基础、地理位置、流媒体、AI、测速、IP 风险）及单节点探测握手超时（1..30 秒）。
- **全局节点筛选**：在策略组编排前置执行的全局过滤条件管线（支持协议、时延阈值、探针判定等多维度组合）。
- **访问安全与认证**：支持开放模式与保护模式无缝切换、管理员令牌轮换与脱敏显示，以及连接状态即时探测。
- **外观与本地偏好**：内置 7 种配色主题、中英双语即时切换，以及运行底座架构与打包信息展示。

### 2. DNS 设置 (`/dns`)
- **全局 DNS 解析配置**：提供可视化表单与原始 YAML 双模编辑，支持双向无缝转换、语法格式静态校验与实时解析结果预览。
- **上游解析源与分流映射**：配置默认 Nameservers、Fallback 兜底解析服务器、Fake-IP / 真实 IP 模式，以及域名到专有 DNS 的分流策略映射。

### 3. 链式代理 (`/chains`)
- **前置跳板绑定**：为出口目标（订阅 `subscription`、策略组 `node_group`、单节点 `node`）挂载前置跳板代理（策略组 `node_group` 或单节点 `node`）。
- **范围优先级与抢占规则**：
  1. **层级范围优先级**：**单节点 (`node`, P1)** > **策略组 (`node_group`, P2)** > **订阅 (`subscription`, P3)**。单节点绑定永远优先于策略组和订阅层级的同名节点规则。
  2. **同级排序规则**：相同层级范围下，`sort_order` 数值较小者优先胜出。
  3. **确定性打破平局**：层级与排序值均相同时，按 Binding ID 字典序较小者确定性生效。
- **实时效果预览与冲突保护**：250ms 防抖实时预览呈现目标覆盖数、胜出生效数、自指/环路跳过数及高优覆盖数。遭遇并发保存冲突（`HTTP 409 Conflict`）时严格保留编辑草稿，提供重新加载或基于新版本 Rebase，避免静默覆盖。
- **生效节点跳板台账**：全量呈现当前策略快照中各出站节点的最终 `dialer-proxy` 跳板分配与绑定归属来源（单节点、策略组或订阅）。

### 客户端格式支持约束与设计不变量
- **仅 Mihomo 支持链式代理**：链式代理（编译为节点属性 `dialer-proxy`）为 **Mihomo (Clash Meta)** 专属输出能力。
- **格式能力校验拦截 (`HTTP 422`)**：若当前生效配置中包含非空跳板，而导出发布目标为 Sing-box、Quantumult X 或 Surge 时，编译器将严格拦截并返回 **`HTTP 422 (unsupported_target_capability)`**，绝不进行隐式静默丢弃或生成残缺配置。无链式代理的普通发布在四种格式下均可正常编译导出。
- **PROCESS-NAME 规则不变量**：链式代理仅作用于出站代理节点之间的网络拓扑转发，**绝对不改变、不改写客户端分流中的 `PROCESS-NAME` 进程规则**与策略分类树行为。

---

## 智能体自动化运维与 API 技能 (Agent Operations & Skills)

为了让外部智能体（Hermes Agent、Pi 多智能体集群等）或自动化脚本能够在**无需修改源码、无需重新构建容器**的前提下便捷地调用与操作 CSP 1.0，我们在仓库中提供了专用的智能体技能目录：[`skills/csp-ops/`](skills/csp-ops/SKILL.md)。

### 内置运维工具 CLI (`csp_ctl.py`)
位于 `skills/csp-ops/scripts/csp_ctl.py`，采用 Python 3 原生标准库编写（零外部依赖），提供开箱即用的命令行工具与 Python SDK：

```bash
# 1. 查看集群全貌与健康状态（节点数、策略组数、470 条规则、订阅状态与诊断）
python3 skills/csp-ops/scripts/csp_ctl.py status

# 2. 分流规则管理（模糊分类与目标组解析）
python3 skills/csp-ops/scripts/csp_ctl.py rule list --limit 100
python3 skills/csp-ops/scripts/csp_ctl.py rule list --search "openai"
python3 skills/csp-ops/scripts/csp_ctl.py rule add --expression "DOMAIN-SUFFIX,anthropic.com" --target "AI" --category "AI"
python3 skills/csp-ops/scripts/csp_ctl.py rule delete --id "<rule-uuid>"

# 3. 策略组管线编排与空节点兜底控制
python3 skills/csp-ops/scripts/csp_ctl.py group list
python3 skills/csp-ops/scripts/csp_ctl.py group get "选择节点"
python3 skills/csp-ops/scripts/csp_ctl.py group add-source "选择节点" --kind filter --filter-display-not-regex "自动|故障|官网|套餐|机场|订阅"
python3 skills/csp-ops/scripts/csp_ctl.py group set-empty-pass "加拿大" --enable

# 4. 订阅同步管理
python3 skills/csp-ops/scripts/csp_ctl.py sub list
python3 skills/csp-ops/scripts/csp_ctl.py sub refresh --all
```

完整规范、Python SDK 导入范例与全量 RESTful API 契约详见：[skills/csp-ops/SKILL.md](skills/csp-ops/SKILL.md)。
