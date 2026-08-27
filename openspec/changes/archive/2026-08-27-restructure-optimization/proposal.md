# 重构优化：clash-sub-parser

## Why

项目分层骨架（router→service→model）方向正确，但审查发现三处结构性热点：配置导出/导入的脆弱领域规则（事务边界、外键顺序、token 保留）内联在 router 层无法复用；同一套 YAML/script 生成逻辑存在 3 份拷贝（改一个响应头要同步 4 处，.exp.md 已有 UA/header 漏改事故记录）；SSRF 防线（redirect 校验+限长管线）双实现且行为已漂移。这些是回归风险最高的区域。

## What Changes

- 抽出 `config_transfer_service.py`：export/import/reset 业务逻辑从 `routers/settings.py` 下沉，router 只留参数解析与状态码映射
- generate 生成链路收口：`generate_service.render_current()` + `file_response()` 统一 4 组重复端点；main.py 根路由 `/yaml` `/script` 改为别名
- 合并 SSRF 安全管线：新增 `utils/http_fetch.py::stream_fetch`，subscription/download 两处手写 redirect+限长实现统一走它
- 认证中间件移出 main.py：拆 `app/middleware/auth.py`，main.py 只剩 app 构建与路由挂载
- services 层 HTTPException 渗入：新代码约定领域异常，首个落地点为 scheduler 路径（不再 `except HTTPException` 判 404）
- 前端：新增 `api/types.ts` response 类型契约（对照后端 schemas 翻译），api/index.ts 返回类型化 Promise；NodeGroupModal.vue 移入 components/ 并抽出 useRegexPreview composable
- 测试安全网先行：download_service redirect/SSRF/超大文件用例、config transfer 回滚与 token 保留集成用例

## Capabilities

### New Capabilities

- `config-transfer`: 配置导出/导入/重置的领域逻辑（事务完整性、敏感字段保留、表顺序）
- `http-fetch-pipeline`: 统一的带 redirect 校验与字节限长的流式抓取管线（SSRF 防线单点化）

### Modified Capabilities
<!-- 无既有 openspec specs，本项目首次建立规格 -->

## Impact

- 影响：backend/app/{routers/settings.py, routers/generate.py, main.py, services/generate_service.py, services/subscription_service.py, services/download_service.py}、frontend/src/api/
- 新增：services/config_transfer_service.py、utils/http_fetch.py、middleware/auth.py、frontend api/types.ts、useRegexPreview.ts
- 不变：全部 API 契约、数据库 schema、部署方式（docker compose）
- 风险控制：重构前先补 download_service 与 config transfer 测试；每步 pytest 全绿再推进
