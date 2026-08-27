# 设计：clash-sub-parser 重构

## Context

审查确认分层骨架正确，问题集中在三个热点（settings 传输逻辑、generate 拷贝、main.py 中间件）加一条安全管线双实现。基线：pytest 101 passed，前端 build 全绿。部署为 docker compose 镜像内构建，数据在 named volume。

## Goals / Non-Goals

- Goals：领域规则归位 service 层；拷贝收敛单点；SSRF 防线合一；认证路径可独立测试；前端类型契约建立
- Non-Goals：不做全量 TS 化、不重排目录结构、不引入自定义异常体系全家桶（渐进式）、不改 API 契约与数据库 schema

## Decisions

### D1. config_transfer_service 抽取
`routers/settings.py` 的 export/import/reset 移入 `services/config_transfer_service.py`，四个函数签名纯数据进出（dict in/out），router 薄壳化。`EXPORT_MODELS`/`IMPORT_TABLE_ORDER` 常量随迁。测试改造为 service 层直测 + 路由薄壳测试。

### D2. generate 收口
`generate_service.render_current(db, kind)` 封装「取配置→开关→生成」，`file_response(result, kind)` 统一 headers/Disposition。12 个端点改调用收口函数，main.py 根路由改为 `include_router` 别名。预期 generate.py 缩至 ~80 行。

### D3. SSRF 管线合一
新增 `utils/http_fetch.py::stream_fetch(client, url, *, max_redirects, max_bytes) -> FetchResult`。redirect 缺 Location 统一报错（当前一处 502 一处透传，属漂移 bug）。subscription/download 各删 ~30 行手写实现。

### D4. 认证中间件搬家
token 校验中间件移入 `app/middleware/auth.py`，保留现有判定函数复用。conftest 对 `AsyncSessionLocal` 的 patch 目标同步更新。

### D5. 异常渗入的渐进治理
不批量改存量。约定新代码用领域异常；首个落地点 scheduler 复用 fetch 链路处——定义 `SubscriptionFetchError`，scheduler 不再检查 HTTPException.status_code。

### D6. 前端类型契约最小化
只做 `api/types.ts`（对照 backend/schemas 翻译核心 response 类型）+ api/index.ts 返回类型标注。存量 .vue 不动。NodeGroupModal 迁 components/ + 抽 useRegexPreview.ts。

### D7. 测试先行顺序
1. download_service 三组用例（redirect/SSRF/限长）→ 2. config transfer 回滚+token 保留集成用例 → 3. 再动对应产品代码。每步 pytest 全绿推进。

## Risks / Trade-offs

- settings 导入是最脆弱路径 → 用「先补测试再重构」对冲；导入回滚用 savepoint 已有实现，测试锁死行为后移动才安全
- stream_fetch 合并可能改变个别边界行为 → 以更严格的一侧为准（缺 Location 报错），在 tasks 中显式验证两条路径
- 前端 types 与实际响应漂移风险 → 类型仅作编辑器提示层，不做运行时校验，漂移成本可控

## Migration Plan

全部改动向后兼容，无 API/schema 变化。按 tasks 顺序执行，每步可独立提交回滚。最终 docker compose rebuild 验证。
