## Purpose

定义新系统的固定物理隔离、Alembic-only、安全确认、离线导入和 legacy deny-list，使新控制平面可被验证为独立于当前单体

## ADDED Requirements

### Requirement: 固定物理边界
系统 SHALL 以 `csp-control-plane` 和 `csp-control-plane-db` 两个新服务运行；数据库为独立 PostgreSQL 实例 `csp_control_plane`，仅使用 `csp-control-plane-db-data` 命名卷和 `csp-control-plane-net` 私有网络。新镜像仅从 `control-plane/` 构建，且不得使用旧 app、`backend-data`、SQLite、旧网络、旧环境前缀、旧镜像层、旧包或旧前端

#### Scenario: 上线前物理切断验收
- **WHEN** 运维人员检查新服务的构建上下文、镜像层、Inspect 挂载/网络/环境和运行时数据库连接
- **THEN** 每项只指向指定新边界；任一旧对象、旧卷、旧网络或旧连接使验收失败

### Requirement: Alembic-only 与无副作用 readiness
系统 SHALL 仅通过受版本控制的 Alembic 迁移演进新库。启动、readiness、调度和 HTTP 请求不得执行 DDL。`/ready` 只报告新库连接、revision、活动修订、lease 与运行器就绪

#### Scenario: 模式版本不匹配
- **WHEN** 服务连接到低于要求的独立数据库模式
- **THEN** readiness 返回不可用及所需迁移信息，不创建或修改任何表

### Requirement: 可消费的生产确认
生产初始化、离线导入、入口切换、旧容器停服和外部写入 SHALL 在副作用前消费 `ExecutionConfirmation`。记录必须绑定 operator subject、`production` 环境、action、不可变 target descriptor/digest、15 分钟 expiry 和一次性状态，并记录不含秘密的审计 correlation id。执行器 SHALL 原子校验并消费记录

#### Scenario: 无效确认零副作用
- **WHEN** 确认缺失、目标不匹配、过期、已消费或被拒绝
- **THEN** 执行器返回 `confirmation_required`，不写生产/外部对象、不切流、不导入且不停旧容器

#### Scenario: 有效确认审计
- **WHEN** 操作员针对精确动作和目标创建有效确认且执行器成功消费
- **THEN** 仅该动作执行一次，并留下不含 token、URL 或节点载荷的审计记录

### Requirement: 离线旧数据导入边界
系统 SHALL 只允许显式离线工具在脱敏旧库副本上只读运行，并仅创建来源名称、策略、规则的未发布草稿。URL、节点载荷、UUID、密码、私钥、token 和其他秘密不得进入新库、报告或日志；工具不启动新服务、不修改活动修订

#### Scenario: 导入允许的旧逻辑内容
- **WHEN** 操作员对脱敏旧库副本执行已确认的离线导入
- **THEN** 工具仅创建待审查的来源名称、策略和规则草稿，并报告已导入和隔离项

### Requirement: 可再生旧路由 deny-list
实现 SHALL 从受控 legacy-reference fixture 的旧 FastAPI 路由表、`main.py`、全部 router、生成/下载/快照和旧前端路由生成带 hash 的 `(method,path)` 与 SPA path manifest。manifest 至少覆盖 `/api` 下 subscriptions、node-groups、probe、proxy-chains、rule-categories、rules、dns、generate、downloads、settings、snapshots，全部 `/api/v2/**`，根级 `/yaml`、`/script`、Script.js、下载、快照和旧 SPA 路径

#### Scenario: 旧请求切断
- **WHEN** 新服务收到 manifest 中任一旧 API 或前端路径
- **THEN** 返回 404、无 Location、无请求翻译、无兼容生成、无 DB/Job/网络副作用

#### Scenario: 旧路由基线改变
- **WHEN** legacy-reference 的路由或前端路径集合改变
- **THEN** manifest hash 变化使 CI 失败，直到人工审查新 deny 项
