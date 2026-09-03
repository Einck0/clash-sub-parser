## Why

现有 CSP 同时运行旧 CRUD、数字行 ID、旧 JSON 镜像、历史输出、`/api/v2` 与迁移桥接路径。旧 Compose 服务把应用和 SQLite 数据卷绑定为 `app` 与 `backend-data`，不能证明配置、库存、探测与发布只有一个真源。

本变更定义一个全新订阅控制平面，而不是继续演进当前单体。新系统从空白、独立的代码与数据边界启动；旧系统和既有重构 WIP 只能作为只读行为参考，绝不构成运行时依赖或兼容层。

## What Changes

- **BREAKING** 新系统固定为独立 `control-plane/` 根、`csp-control-plane` 应用服务和 `csp-control-plane-db` PostgreSQL 服务；仅使用 `csp-control-plane-db-data` 命名卷与 `csp-control-plane-net` 私有网络，不共享旧 `app`、`backend-data`、镜像层、挂载、数据库连接或运行时网络
- **BREAKING** 删除全部旧管理 API、v1/v2 分支、数字 ID、旧前端路由、`/yaml`、`/script`、Script.js、下载、快照与旧 JSON 镜像字段；新系统不会翻译、重定向或执行这些请求
- 建立唯一版本化逻辑配置真源。可移植 Bundle 仅包含策略、规则、DNS、发布选项和无节点载荷的 selector slot；跨实例导入产生未绑定草稿，必须在目的实例显式 rebind 并预检，不能导出节点、来源 URL、UUID、密码、私钥或 token
- 建立不可变来源修订、规范化节点库存、由已启用来源或 ProbeProfile 授权且受预算约束的真实节点出站观测，以及唯一确定性编译器
- 以同一语义图渲染 Clash、Mihomo、Stash、Shadowrocket。发布输出仅含该目标所需、已被当前修订选中的节点连接信息；管理 API、日志和 Bundle 永不回显节点或管理秘密
- 用 action-specific、目标绑定、一次性且会过期的确认记录门控生产初始化、离线导入、入口切换、旧容器停服和外部写入。此产品内授权不扩大 Hermes 在计划、实现和验证阶段的出站或生产写入权限

## Capabilities

### New Capabilities

- `clean-slate-control-api`: 无版本管理 API、logical ID、发布 endpoint、统一错误与负向旧路由契约
- `subscription-source-lifecycle`: 来源、不可变修订、安全规范化、加密秘密与预算授权刷新
- `node-capability-observation`: 经被测节点出口、按 ProbeProfile 授权的有界 Job 与不可变观测
- `policy-configuration-compiler`: 版本化策略图、portable selector slot、显式本地 rebind、确定性编译与四目标渲染
- `configuration-release-management`: 草稿、预检、原子发布、回滚、发布令牌、输出缓存与安全 Bundle
- `operational-safety`: 指定物理隔离、Alembic-only、可消费确认门、导入边界、legacy deny manifest 与恢复

### Modified Capabilities

- `node-capability-probe`: 以新控制平面的真实出站、logical ID、授权预算与不可变观测替换旧 name-key 探测
- `node-probe-persistence-and-filtering`: 以 Profile、时效与策略谓词替换旧缓存和策略组字段筛选
- `http-fetch-pipeline`: 将订阅拉取收束为已保存并启用来源的安全输入；历史资产下载不属于新产品
- `config-transfer`: 用安全逻辑 Bundle 与 rebind 草稿替代整表导入、导出、快照和固定表顺序重置

## Impact

- 实现阶段将创建新根、新服务与指定的 PostgreSQL 数据边界；本计划阶段不修改业务代码、测试、容器、数据库、环境、入口或工作树 WIP
- 当前 `backend/app/main.py` 注册的旧 CRUD Router、`/api/v2`、`/yaml`、`/script` 和 SPA catch-all 是 deny-list 的事实来源，不是可复用接口
- 当前未提交 bridge、legacy importer、shadow compiler、canonical compiler、v2 Router、模型与测试只能提供已脱敏验收案例或失败模式；新系统不得复制、导入、链接或部署它们
- 产品内已保存且启用的来源或 ProbeProfile 可以在其版本、状态和预算持续有效时执行预期只读 fetch/probe；禁用、取消、预算耗尽或配置变更会阻止后续出站。Hermes 自身仍须在用户确认边界内工作
- 创建生产服务、生产新库初始化、离线导入、入口切换、停旧容器与任何外部写入均须在执行前获得单独确认
