# 路线图与发布约束

本文档记录 `clash-sub-parser` 的当前状态、关键决策、后续计划、发布约束和近期变更摘要。它不记录运行实例数据、真实规则数量、备份路径、线上 API 管理记录或任何敏感配置。

## 当前状态

已完成主线：

- 单容器部署：前端构建产物打包进后端镜像，FastAPI 同时提供 API、静态资源和 SPA fallback。
- 订阅管理：支持多个上游订阅、手动拉取、定时拉取、正则筛选、手动包含/排除、节点预览和上游 headers 保存。
- 节点组管理：支持静态节点、内建目标、节点组引用、节点组节点展开、动态正则匹配和预览。
- 规则管理：规则分类独立排序，规则表扁平化，支持草稿编辑、搜索、排序和移动端卡片编辑。
- DNS 管理：支持可视化字段和 Raw YAML。
- 生成与下载：支持 Clash YAML、Script.js、短链接 `/yaml` / `/script` 和单订阅导出。
- 安全设置：支持运行时开启 token 保护，可分别保护 Web UI、管理 API 和导出接口。配置支持导出（含/不含订阅）、导入和重置。
- 开源基础设施：README 中英文、MIT License、CONTRIBUTING、SECURITY、CODE_OF_CONDUCT、CI、Dependabot、Dockerfile 和 Compose 示例已经具备。

## 关键决策

1. 单容器部署
   前端产物由 FastAPI 提供，降低运维成本。

2. 数据库作为状态源
   订阅、节点组、规则、DNS 和生成开关均以数据库保存内容为准。

3. 规则扁平化
   一条规则一行数据库记录，分类只作为组织和排序层。

4. 草稿保存优先
   分类和规则等复杂列表使用草稿模式，避免频繁单项保存。

5. 订阅信息来自 headers
   流量、到期、建议更新间隔和主页信息来自上游响应 headers，不从 YAML body 推断。

6. 节点筛选分层
   正则是粗筛，手动包含/排除是最终精修层。

7. 节点组正则保持动态
   不默认把匹配结果混入静态节点；如需固化，必须显式冻结。

8. 导出鉴权使用 URL query
   Clash 客户端订阅通常无法携带自定义 header，因此 `/yaml`、`/script` 和下载地址启用鉴权时支持 `?token=...`。

9. Web UI 使用 HttpOnly cookie
   登录成功后由后端设置 cookie，前端不把 token 持久化到本地存储。

10. 计划文档不维护具体规则清单
    规则明细、线上数量和运维记录容易过期，也可能包含私有信息，应留在数据库或本地运维记录中。

## 下一阶段计划

### P0：稳定性与防误操作

- [ ] 给规则分类首页和规则详情页增加更明确的保存结果提示，例如 toast 或顶部状态条。
- [ ] 保存全部前做基础校验：空类型、空目标、非 `MATCH` 规则空 value。
- [ ] 规则保存失败时保留草稿，不自动刷新页面。
- [ ] 类别删除和节点组删除增加依赖影响提示。
- [ ] 重置配置前展示影响范围，并要求明确确认。

### P1：批量编辑

- [ ] 规则分类首页的全局搜索增加“定位到具体规则行”的深链接。
- [ ] 规则详情页增加批量启用 / 禁用。
- [ ] 规则详情页增加批量修改目标 proxy。
- [ ] 规则详情页增加批量移动到其他分类。
- [ ] 支持导入或粘贴多行 Clash 规则并解析成规则草稿。

### P1：节点组能力增强

- [ ] 将 `url-test`、`fallback`、`load-balance` 的参数做成可视化表单。
- [ ] 支持节点组拖拽排序或移动到指定位置。
- [ ] 节点组预览增加引用来源解释，帮助理解最终节点来自哪里。

### P2：DNS 体验增强

- [ ] DNS 可视化模式增加更严格的字段校验。
- [ ] `nameserver-policy` 支持批量导入/导出。
- [ ] Raw YAML 与可视化模式切换时增加差异提示。

### P2：测试、安全和可复现构建

- [ ] 增加更多 API 路由级测试，覆盖主要 CRUD 路由。
- [ ] 增加端到端 UI smoke test。
- [ ] 增加依赖版本 pinning 或 Python lock 文件。
- [ ] 增加发布前 secret scan 和 compose config scan。
- [ ] 如果暴露公网，建议同时使用 HTTPS 反代；token 保护用于轻量访问控制，不替代完整账号体系。

## 开源发布约束

公开仓库应只包含代码、文档、测试、示例配置和必要的项目元数据。以下内容必须留在本地，或通过 `.gitignore` / `.dockerignore` 排除：

- `.env` 与真实部署配置。
- 本地 `docker-compose.yml`。
- SQLite/PostgreSQL dump、备份、缓存和运行日志。
- 真实订阅 URL、访问 token、代理节点凭据、完整生成配置。
- 私有迁移脚本、私有规则素材、原始需求记录。
- `frontend/node_modules/`、`frontend/dist/`、Python venv、pytest/ruff/cache 目录。

当前开源框架文件：

- `.gitignore`：排除环境变量、数据库、备份、依赖、构建产物和缓存。
- `.dockerignore`：排除 Docker 构建不需要的本地状态和私有数据。
- `.env.example`：提供无敏感信息的运行配置模板。
- `docker-compose.example.yml`：公开 Compose 示例，镜像源配置保持注释。
- `LICENSE`：MIT License。
- `CONTRIBUTING.md`：开发、测试和提交约定。
- `SECURITY.md`：漏洞报告与敏感数据处理说明。
- `CODE_OF_CONDUCT.md`：基础社区行为准则。

发布前最低验证：

```bash
bash scripts/test_py310.sh
cd frontend
npm ci
npm run build
cd ..
docker compose -f docker-compose.example.yml config
```

发布前 Git 检查：

```bash
git status --short
git ls-files docker-compose.yml info.txt backups references .learnings .pytest_cache frontend/dist frontend/node_modules backend/.pytest_cache
```

第二条命令应无输出。如有输出，说明本地私有文件、备份或生成产物已经被跟踪，需要先从 Git 索引移除。

## 安全边界

- 管理 UI 默认关闭认证；不要直接暴露到公网。
- Docker Compose 示例默认仅绑定 `127.0.0.1`。
- 需要公网或内网暴露时，应显式配置端口、HTTPS 反代和 token。
- 订阅 URL、数据库、导出链接和带 query token 的地址都视为敏感数据。
- `.env.example` 只能包含占位符和安全默认值。
- 订阅拉取默认拒绝 localhost、私网、链路本地和保留地址；如需拉取内网源，必须显式启用 `CLASH_ALLOW_PRIVATE_FETCH_URLS`。
- Compose 使用 `CLASH_HTTP_PROXY` / `CLASH_HTTPS_PROXY` / `CLASH_ALL_PROXY`，避免宿主机普通 `HTTP_PROXY` 被意外写入项目配置。

## 近期变更摘要

### 2026-05-28：开源发布整理

- README 拆分为英文和中文版本。
- 根目录 `project.md` 缩减为项目索引和发布前检查。
- 详细设计迁移到 `docs/design.zh-CN.md`。
- 路线图、发布约束和变更摘要迁移到 `docs/roadmap.zh-CN.md`。
- 公开 Compose 示例保留可选国内构建源注释；本地 `docker-compose.yml` 由 `.gitignore` 排除。

### 2026-05-28：设置页与安全边界增强

- Web UI 登录改为 HttpOnly cookie。
- 管理 API 兼容 header token 和 Bearer token。
- 使用 cookie 认证的写操作需要 CSRF header。
- 导出地址在启用保护时继续支持 query token，以兼容 Clash 客户端。
- 设置页增加随机 token 生成功能。
- 订阅拉取增加 URL scheme、私网地址、重定向目标和响应大小限制。

### 2026-05-28：节点组兜底与配置备份/重置/导入

- 节点组支持 `add_fallback` 配置，可在解析节点末尾追加 `REJECT`。历史组默认 `add_fallback=false`；新建组默认 `true`。
- 设置页提供导出配置（含/不含订阅）、导入配置和重置配置入口。
- 导出 JSON 排除访问 token/hash；导入时保留当前 token_hash，不会因导入而丢失鉴权。
- 导入会按表清空后写入，datetime 字段自动从 ISO 字符串还原；支持部分表导入和错误回报。
- 导入和重置操作均有二次确认，明确提示会覆盖当前配置。

### 2026-05-28：地区组兜底调整

- 地区 `url-test` 组不再手动追加 `PASS` 或 `REJECT` 兜底。
- 地区组应反映正则匹配到的真实节点；空组应暴露配置或订阅异常，而不是由兜底节点掩盖。
