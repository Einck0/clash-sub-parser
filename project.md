# clash-sub-parser 项目计划书

> 当前阶段：核心功能与主要管理界面已经进入稳定可用阶段。根目录计划书只保留项目定位、文档索引和发布前检查项；详细设计与路线图拆分到 `docs/` 目录维护。

## 项目定位

`clash-sub-parser` 是一个单容器 Web 应用，用于可视化管理 Clash 兼容配置。它支持订阅、策略组、规则、DNS，并可生成可直接订阅的 `YAML` / `Script.js` 输出。

运行时数据保存在数据库中。真实订阅 URL、访问 token、代理节点凭据、备份文件、私有迁移素材和线上运维记录不应写入公开项目文档，也不应提交到公开仓库。

## 文档索引

- [README.md](README.md)：英文入口文档，面向首次部署和开源用户。
- [README.zh-CN.md](README.zh-CN.md)：中文入口文档。
- [docs/design.zh-CN.md](docs/design.zh-CN.md)：架构、数据模型、生成链路、鉴权和前端交互设计。
- [docs/roadmap.zh-CN.md](docs/roadmap.zh-CN.md)：当前状态、关键决策、后续计划、发布约束和变更摘要。
- [CONTRIBUTING.md](CONTRIBUTING.md)：开发、测试和贡献约定。
- [SECURITY.md](SECURITY.md)：漏洞报告和敏感数据处理说明。

## 当前状态

- 单容器 Docker Compose 部署已经可用。
- 后端使用 FastAPI + SQLAlchemy Async，默认 SQLite，可配置 PostgreSQL。
- 前端使用 Vue 3 + Vite，构建产物由 FastAPI 同容器提供。
- 订阅、节点组、规则分类、规则、DNS、生成和设置页主流程已打通。
- Web UI / 管理 API / 导出接口支持可选 token 保护。
- Web UI 登录使用 HttpOnly cookie；导出订阅地址在启用保护时使用 `?token=...`，以兼容 Clash 客户端。
- 设置页支持配置导出（含/不含订阅）、导入和重置；导入时保留当前 token_hash。
- 订阅拉取默认限制私网、localhost、链路本地、保留地址等目标，降低 SSRF 风险。

## 发布前检查

公开发布前至少执行：

```bash
bash scripts/test_py310.sh
cd frontend
npm ci
npm run build
cd ..
docker compose -f docker-compose.example.yml config
```

同时确认本地私有文件没有被 Git 跟踪：

```bash
git status --short
git ls-files docker-compose.yml info.txt backups references .learnings .pytest_cache frontend/dist frontend/node_modules backend/.pytest_cache
```

第二条命令应无输出。如有输出，说明本地私有文件、备份或生成产物已经被跟踪，需要先从 Git 索引中移除。

## 开源边界

公开仓库应只包含代码、测试、文档、示例配置和必要的项目元数据。不要提交：

- `.env` 或真实部署配置。
- `docker-compose.yml` 本地部署文件。
- SQLite/PostgreSQL dump、备份、缓存和运行日志。
- 真实订阅 URL、访问 token、代理节点凭据或完整生成配置。
- 私有迁移脚本、私有规则素材、原始需求记录或线上运维记录。
- `frontend/node_modules/`、`frontend/dist/`、Python venv、pytest/ruff/cache 目录。

## 下一步重点

短期优先补强：

- API 路由级测试和端到端 UI smoke test。
- 批量编辑、防误操作和保存失败时的草稿保留体验。
- 节点组高级参数可视化。
- DNS 字段校验与 Raw YAML / 可视化模式差异提示。
- Python 依赖锁定或版本 pinning，提升可复现构建能力。

## 2026-05-29 客户端下载功能

- 在设置页新增“客户端下载”卡片：支持一键从 GitHub latest release 下载 Clash Verge Rev Windows x64、Clash Meta for Android arm64，并支持自定义 URL 下载。
- 后端新增 `/api/downloads`：列出已下载文件；`/api/downloads/preset`：按预设拉取 GitHub 最新 release asset；`/api/downloads/custom`：下载自定义 URL；`/api/downloads/files/{filename}`：提供本地文件下载。
- 下载目录默认 `/data/downloads`，使用现有 Docker volume `backend-data` 持久化；默认单文件上限 300MB、超时 300s。
- 下载 URL 复用现有 `validate_fetch_url` SSRF 防护，默认不允许私网/localhost 目标。
- 已验证：`python -m compileall backend/app`、`npm run build`、`PYTHONPATH=backend pytest -q`、Docker compose rebuild/up。

### 2026-05-29 下载失败修复

- 原因：下载功能只在订阅拉取中使用了运行时“订阅拉取代理”，新增的 GitHub latest API / asset 下载没有读取该代理设置；容器内直连 GitHub 在当前网络环境下会超时或 502。
- 修复：下载服务复用 `security_settings.fetch_proxy_enabled/fetch_proxy_url`；`docker-compose.yml` 增加 `host.docker.internal:host-gateway`，当前运行时代理设为 `http://host.docker.internal:7890`。
- 验证：容器内可解析并连接 `host.docker.internal:7890`；`download_preset('clash-meta-android-arm64')` 成功下载 `cmfa-2.11.28-meta-arm64-v8a-release.apk`。

### 2026-05-29 下载缓存与单文件控制

- 预设下载使用 `cache_prefix` 标记（如 `clash-meta-android-arm64-`），下载新版时自动删除同前缀旧文件，每个预设只保留一份。
- 文件存放在容器内 `/data/downloads`，对应 docker volume `backend-data`，重启不丢失。
- 设置页 `onMounted` 自动加载已有缓存列表，无需手动刷新。

### 2026-05-29 客户端刷新语义调整

- 设置页打开时只读取本地 `/data/downloads` 缓存列表，不请求 GitHub、不下载。
- “重新读取本地缓存”仅刷新本地文件列表。
- “刷新最新版客户端”才会依次拉取 Clash Verge Rev Windows x64 与 Clash Meta for Android arm64 的 GitHub latest release 并下载覆盖缓存。
