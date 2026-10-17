# Implementation Tasks: Minimal Client Subscriptions (Zero-Glue Clean Slate)

- **Feature Directory**: `specs/004-publication/`
- **Run ID**: `pi_run_20261017_165647_2800333`
- **Change**: `004-publication`
- **Work Package**: `default`
- **Epoch**: 1
- **Status**: IMPLEMENTING — 主订阅流量透传与策略组顺序修复

---

## 历史已完工任务（过往交付）

- [x] **S401 [ARCH]** 主脑完成架构反思与契约更新：废除独立链接表与 UUID 路由契约，确立统一 Settings Token 鉴权与直观客户端直链。
- [x] **S402 [BACKEND]** 后端实现直观导出路由与 Settings Token 鉴权：现场动态编译接口与 HTTP 路由注册。
- [x] **S403 [FRONTEND]** 前端推平过度工程化交互：四端直链展示与一键复制。
- [x] **S404 [SELF-TEST & PREVIEW]** 施工子机自测与隔离容器成品预览准备。
- [x] **S405 [REVIEWER]** 独立 Reviewer 只读审查：通过。
- [x] **S406 [CRITIC]** Critic 最终黑盒与多模态视觉验收：通过。
- [x] **S407 [DEPLOY]** Git 提交、官方 Dev 云构建与直接上线：完成不可变镜像 `prelease-047b2bd` 上线与数据热备。

---

## 本轮任务序列（主订阅流量透传与策略组排序修复）

- [x] **S408 [ARCH]** 主脑完成契约与任务规划：更新 `spec.md`、`plan.md` 与 `tasks.md`，确立 Migration 000023 规范、主订阅 `Subscription-Userinfo` 抓取/导出透传规范、策略组 `position` 排序与前后端编排契约。
- [x] **S409 [BACKEND]** 施工子机实现后端与数据库变更：
  - 创建 `migrations/000023_group_position_and_primary_subscription.sql`；
  - `internal/fetch/fetch.go`: 解析上游响应中的 `Subscription-Userinfo` 头部至 `fetch.Response`；
  - `internal/application/inventory` & `subscription`: 订阅拉取成功后持久化 `userinfo`；`PATCH /api/v1/subscriptions/{id}` 支持切换 `is_primary`（单主订阅互斥机制）；
  - `internal/transport/http/subscription_direct.go`: 客户端直链导出时，读取当前启用的主订阅 `userinfo` 并注入 `Subscription-Userinfo` 响应头；
  - 策略组排序：在 `domain.NodeGroup`、`types.go`、`sqlite/policy.go`、`consistent_facts.go` 中支持 `position`，查询与编译统一使用 `ORDER BY position ASC, created_at ASC, id ASC`；
  - `internal/resolver/graph.go`: 排序支持 `Position ASC, Name ASC, ID ASC`；
  - `internal/transport/http/policies.go`: 支持更新策略组 `position`；
  - 编写与更新后端单元测试。
- [x] **S410 [FRONTEND]** 施工子机实现前端交互与展示：
  - `SubscriptionsView.vue`:
    - 列表卡片增加“主订阅”徽标与“设为主订阅”操作（遵循 `.exp.md`，不塞入添加/编辑弹窗）；
    - 解析并展示主订阅的流量信息（已用、总计/剩余）与过期时间；
  - `PolicyView.vue` & `GroupCard.vue`:
    - 策略组卡片支持“上移”与“下移”调整顺序；
    - 策略组编辑弹窗支持维护排序序号；
  - 更新前端 Vitest 测试并确保 `vue-tsc` 检查通过。
- [x] **S411 [SELF-TEST & PREVIEW]** 施工子机在隔离容器内完成验证并就绪预览：
  - 隔离执行 Go 测试与前端测试；
  - 启动隔离预览服务（如 28081 端口），植入合成主订阅与有序策略组数据供验收；
  - 输出可复验的执行报告与预览清单。
- [x] **S412 [REVIEWER]** 委派独立 Reviewer 审查：
  - 独立审查主订阅单主互斥性、响应头注入、策略组排序保序及 `.exp.md` 规范遵从情况。
- [x] **S413 [CRITIC]** 委派 Critic 执行黑盒验收：
  - 在隔离预览环境中黑盒验证桌面与移动端 UI 体验；
  - 验证直链导出包含正确的 `Subscription-Userinfo` 响应头与有序 `proxy-groups`。
- [x] **S414 [DEPLOY]** 提交源码、推送 dev、官方 Actions 云构建与平滑更新生产：
  - 提交确证源码，推送到 `dev`；
  - 跟踪 GitHub Actions 构建不可变镜像；
  - 生产 SQLite 数据库一致性热备；
  - 平滑更新生产 Compose 并现场验证 `https://sub.einck.top`。
