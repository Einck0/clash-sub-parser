## 1. Schema Migration & Domain Model

- [x] 1.1 新增 Migration `000016_node_source_history.sql`，建立独立不可变历史账本表 `node_source_history`，定义联合唯一索引与 `ON DELETE SET NULL` 外键级联；运行单测验证 migration runner 顺利应用且具有幂等性。
- [x] 1.2 在 `internal/domain/` 中新增 `NodeSourceHistory` 实体、`RelationState`、`AttributionCause` 与 `AttributionStatus` 类型及常量；以单元测试验证实体字段校验与 JSON 脱敏规范。
- [x] 1.3 在 `internal/repository/sqlite/` 中实现 `NodeSourceHistoryRepository`，包含单事务批量插入、按节点查询历史及按订阅查询历史方法；编写集成单测验证外键置空与观测时间倒序排序。

## 2. 运行时写链保全改造

- [x] 2.1 改造 `internal/application/inventory/service.go` 的订阅刷新剪枝逻辑，在 `DELETE FROM node_sources` 物理移除前原子写入快照至 `node_source_history`（`cause = 'refresh_removed'`）；编写针对性单测验证节点下线时历史事实被固化且节点 `active` 保持不变。
- [x] 2.2 完善抓取失败、启用/停用与用户覆盖生命周期隔离：验证抓取失败时不删除实时成员亦不伪造历史；订阅状态切换不触发冗余历史；用户覆写（`node_overrides`）不污染上游来源血统；编写覆盖单测。
- [x] 2.3 改造订阅删除生命周期处理：在删除订阅前快照留存该订阅下全部节点的关联记录至 `node_source_history`（`cause = 'subscription_deleted'`）；编写针对性测试验证订阅删除后历史记录保留脱敏标签但 `subscription_id` 为 NULL。

## 3. 运维恢复 CLI 工具 (csp recover-source-history)

- [x] 3.1 在 `cmd/csp/` 与 `internal/application/inventory/` 中实现 `csp recover-source-history --manifest <path> [--dry-run]`，只读解析带 SHA-256 校验的脱敏清单并批量原子写入 `node_source_history`；编写单测验证 dry-run 0 写入与正式回填的幂等性。
- [x] 3.2 在恢复工具中实现严格的资产不变量前置与后置断言门禁（总节点 1014、活跃节点 31、`node_sources` 31、Scope E 不变）；编写单测验证任何状态漂移或凭据篡改时立即触发 Fail-Closed 事务回滚。
- [x] 3.3 实施丢弃碰撞凭据记录的隔离逻辑，确保首选记录写入 verified，被丢弃异凭据记录仅作为丢弃证据留存不附着当前节点；以单测验证碰撞隔离。

## 4. 只读审计 API 与前端节点抽屉呈现

- [x] 4.1 在 `internal/transport/http/` 中实现 `GET /api/v1/nodes/{id}/source-history` 端点，返回当前所属源与历史归属列表及综合 `attribution_status`；编写 HTTP Handler 单测验证鉴权、脱敏及响应结构。
- [x] 4.2 在 `web/src/` 的节点详情 Drawer 中增设“来源归属与历史”只读面板，展示当前源与历史观测事实（含证据类型与观测时间），并确保不暴露敏感凭据与原始 URL；运行前端 `npm run type-check` 与组件测试。

## 5. 全链路集成验证与工程构建门禁

- [x] 5.1 编写全链路端到端集成测试，使用沙箱数据库验证脱敏清单回填、刷新剪枝归档、订阅删除级联以及 API 审计完整闭环；验证前后库存 Scope E 严格为 31。
- [x] 5.2 运行全量后端测试 `go test ./...` 与静态编译门禁 `go build ./...`，确保零报错退出码严格为 0。

## 6. 来源归属身份、删除语义与幂等回填根因整改闭环 (Remediation Root Cause Rectification)

- [x] 6.1 来源严格完整 URL 规范化与名称回退废除：改造 `CanonicalSourceURL` 仅规范化 scheme/hostname 与默认端口，严格保留 query (token/signature)、userinfo 与 path 大小写；废除现代订阅映射对名称的任何 fallback，URL 未匹配一律保留为 unmapped 历史记录。
- [x] 6.2 订阅删除与未关联 DTO 语义解耦：修复 `inventory/service.go` 中 `subscription_id == nil` 误标 `source_deleted`，新增 `source_unmapped: boolean`；前端节点抽屉对历史未关联订阅展示“未关联当前订阅”黄色标签，仅对真实删除事件或外键级联清空展示“已删除”；全量前端单测与类型检查通过。
- [x] 6.3 幂等性预查与不可变证据防伪计数：改造 `recover_history.go` 历史插入逻辑，采用同事务前置存在性预查与 `INSERT ... ON CONFLICT DO NOTHING`，准确上报 `newly_inserted`、`existing_records`、`total_records`，杜绝假增；历史已存在证据若内容篡改触发 Fail-Closed 回滚；实机副本验证首轮回填 998，次轮回填 0 增量、998 已存。
- [ ] 6.4 独立审查机复验（Reviewer Reverification）：由独立 Reviewer 会话对照同一 Change 与整改 diff 审查代码安全性与架构规范。
- [ ] 6.5 成品黑盒验收（Critic Verification）与上线投产闭环：在隔离环境完成视觉与交互验收，待审查与验收全部通过后授权上线。

