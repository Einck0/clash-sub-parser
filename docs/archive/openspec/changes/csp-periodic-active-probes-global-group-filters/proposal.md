## Why

当前探测主要通过手动 `/probes/runs` 提交，缺少面向仓库激活节点的可配置、可恢复周期任务；策略组仅保存显式成员引用，无法按节点属性和探测结果筛选，也没有先于策略组筛选的全局筛选。用户另报告“csp出错”，但未提供当次请求、时间与错误日志；已核实历史 `target mihomo cannot render PROCESS-NAME` 修复位于 `7289b4c`，不得将该旧错误冒充本次根因。

## What Changes

- 增加持久化、默认关闭的周期探测计划：仅面向执行时仍激活且有可用凭据的节点，复用现有安全探测 runner / 有界队列；显示执行批次、失败原因、最新观测与取消/停用状态，并支持重启恢复与受控关闭。
- 增加可持久化的两级节点筛选：**全局筛选 → 按策略组筛选**；支持名称、协议、真实来源、探测状态/延迟与观测新鲜度，明确定义未知/过期、显式引用与空组处理；策略预览与发布共用同一解析结果。
- 增加管理 API 与界面，用于调整探测计划、全局筛选和分组筛选、预览诊断及复核执行证据；无筛选条件保留既有配置效果。
- 扩展 SQLite 增量迁移及自动化验收，保留历史手动探测与旧策略配置；不包含生产部署和任何未复现错误的推测性“修复”。

## Capabilities

### New Capabilities

- `periodic-active-probes`: 周期探测计划、执行证据、恢复与生命周期。
- `global-group-node-filters`: 全局先于组级的节点筛选及预览/发布一致性。

### Modified Capabilities

无（当前 `openspec/specs/` 没有已发布的同名主规格；此次以新增能力建立完整契约）。

## Impact

`internal/domain/{probe,node,policy,ports}.go`、`internal/application/{probe,policy,publication}`、`internal/probe/queue`、`internal/resolver`、`internal/repository/sqlite`、`internal/transport/http`、`cmd/csp/main.go`、`migrations`、`web/src/features/{probes,policy}` 与 API 类型/测试。沿用 Go 标准 `context`/`time`、现有 `modernc.org/sqlite` 事务和索引、现有 sing-box runner 与有界公平队列，不引入第二套调度或表达式解析体系。上线、数据库备份和流量接入另需生产授权。
