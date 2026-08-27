# Tasks: clash-sub-parser restructure-optimization

## 1. 测试安全网先行
- [ ] 1.1 download_service 用例：redirect 逐跳校验、重定向到内网拒绝、超限长截断
- [x] 1.2 config transfer 集成用例：导入中途失败整体回滚；token_hash 导入后保留

## 2. config-transfer 下沉（P1-1）
- [x] 2.1 新建 `services/config_transfer_service.py`，迁移 export/import/reset 逻辑与常量
- [x] 2.2 `routers/settings.py` 薄壳化；测试断言迁移至 service 层

## 3. generate 收口（P1-2）
- [ ] 3.1 `generate_service.render_current` + `file_response` 实现
- [ ] 3.2 routers/generate.py 12 端点改调用收口函数；main.py 根路由别名化
- [ ] 3.3 补三条 GET 路径 YAML 一致性断言

## 4. SSRF 管线合一（P2-5）
- [x] 4.1 新建 `utils/http_fetch.py::stream_fetch`
- [x] 4.2 subscription/download 迁移调用并删除手写实现
- [x] 4.3 验证 redirect 缺 Location 两路径行为一致

## 5. 认证中间件搬家（P1-3）
- [ ] 5.1 拆 `app/middleware/auth.py`；main.py 减半
- [ ] 5.2 conftest patch 目标更新，test_auth 全绿

## 6. scheduler 异常解渗（P2-4 首落地点）
- [ ] 6.1 定义 SubscriptionFetchError；scheduler 路径不再依赖 HTTPException

## 7. 前端类型契约与组件整理（P2-6/P2-7）
- [ ] 7.1 api/types.ts 核心类型 + index.ts 返回标注；vue-tsc 通过
- [ ] 7.2 NodeGroupModal.vue 移入 components/，抽 useRegexPreview.ts，主文件 <300 行

## 8. 全量验证与部署
- [ ] 8.1 pytest 全绿 + npm build + vue-tsc 全绿
- [ ] 8.2 docker compose rebuild + 容器 healthy + API 冒烟
- [ ] 8.3 .exp.md 更新 + commit + push dev + openspec archive
