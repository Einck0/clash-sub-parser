## Context

当前生产环境出现订阅发布请求 404 与拉取鉴权故障。深入排查表明：
1. 生产环境数据库 `publications` 表当前有效记录数为 0 条，且请求使用了历史短路径 `/p/...`，而系统标准服务路径为 `/publish/v1/{publication_id}`，导致 Chi 路由返回 404 Not Found；
2. 现有导出端点 `publicationClientHandler` 在代码中硬编码强制要求提供令牌，且底层 `svc.ResolveAndServe` 仅校验 publication 自身的专属 `pub_<hex>` 令牌，既不支持免密公开拉取，也不支持使用管理端配置的全局统一系统令牌；
3. 管理端鉴权逻辑完全绑定于密码字符串是否存在（`admin_token != ""` 即 protected，反之 open），缺乏由用户独立控制的开关；用户明确要求“管理端与导出链接鉴权开关独立受控，但能复用同一个统一令牌”。

## Goals / Non-Goals

**Goals:**
- 实现数据库迁移 `000013_auth_settings.sql`，在 `settings` 表中解耦持久化 `admin_auth_enabled` 和 `export_auth_enabled`；
- 在领域层与仓储层实现开关状态的原子持久化与读取，确保默认值为 1（全受控）；
- 解耦 `DynamicTokenHolder`，提供由开关独立控制的 `AdminMode()` 与 `IsExportAuthEnabled()`；
- 改造 `publicationClientHandler` 导出端点，在关闭导出鉴权时免密放行，开启时支持专属发布令牌与全局统一系统令牌双重验证；
- 路由层支持 `/p/{publication_id}` 历史短路径，避免客户端 404 故障；
- 提供标准 `GET/PUT /api/v1/settings/auth` API，并保持旧有 API 向后兼容；
- 前端 `SettingsView.vue` 落地双开关与统一令牌维护；`PublicationsView.vue` 落地空状态向导与免密/受控动态直链呈现。

**Non-Goals:**
- 不构建多租户/多角色复杂 RBAC 鉴权体系，保持单管理员控制面极简架构；
- 不修改底层密码哈希体系（沿用 Bcrypt 与 SHA-256 预哈希）；
- 不改动不可变配置编译引擎的核心产出与签名摘要算法；
- 不在 OpenSpec 规划阶段直接触碰或修改业务代码。

## Decisions

### 1. 鉴权开关持久化与向后兼容迁移
- **设计选择**：编写 SQLite 增量迁移脚本 `migrations/000013_auth_settings.sql`：
  ```sql
  ALTER TABLE settings ADD COLUMN admin_auth_enabled INTEGER NOT NULL DEFAULT 1;
  ALTER TABLE settings ADD COLUMN export_auth_enabled INTEGER NOT NULL DEFAULT 1;
  ```
- **技术理由**：使用 `DEFAULT 1` 保证存量配置升级后默认处于受控状态，杜绝任何未预期的安全降级。`internal/domain/settings.go` 中 `Settings` 结构体增加 `AdminAuthEnabled bool` 与 `ExportAuthEnabled bool`；`internal/repository/sqlite/settings.go` 在 `Get` 与 `Update` 方法中无缝映射。

### 2. 内存鉴权模型 DynamicTokenHolder 解耦
- **设计选择**：在 `internal/transport/http/token.go` 的 `DynamicTokenHolder` 内部增设 `adminAuthEnabled` 与 `exportAuthEnabled` 状态变量及对应读写互斥锁：
  - `AdminMode() SecurityMode`：当 `adminAuthEnabled == false` 时，强制返回 `SecurityModeOpen`，管理端完全免密放行；当 `adminAuthEnabled == true` 且存在有效令牌时，返回 `SecurityModeProtected`；当开启但未配置令牌时，保持 Zero-Config 开放特性。
  - `IsExportAuthEnabled() bool`：直接反映导出端点是否受控。
  - `Verify(candidate string) bool`：恒定时间比对候选令牌与系统主令牌哈希。

### 3. 订阅导出端点双模态验证与统一令牌复用
- **设计选择**：改造 `internal/transport/http/publications.go` 中的 `publicationClientHandler`：
  - 服务层先查询发布实体并检查撤销状态，再通过鉴权回调决定是否交付相同的不可变编译产物；未知 ID 保持 404。
  - 当 `!tokenHolder.IsExportAuthRequired()` 时：已激活的发布内容直接放行，无论请求没有令牌还是携带无效 query/Bearer 令牌，均不因鉴权返回 401。
  - 当 `tokenHolder.IsExportAuthRequired()` 时：提取 query `token` 或 Bearer 令牌，校验 publication 专属 SHA-256 摘要或系统统一令牌 bcrypt 验证器；两者均未命中（含令牌缺失）则返回 401。`admin_auth_enabled` 不改变该结果。

### 4. 路由层兼容性修复与 `/p/...` 故障消除
- **设计选择**：在 `internal/transport/http/router.go` 中除保留 `/publish/v1/{publication_id}` 之外，同时挂载 `/p/{publication_id}` 路由，直接由相同处理函数接管或执行 307 Temporary Redirect（携带原始 QueryString），无缝化解历史客户端与外部订阅软件请求 `/p/...` 导致的 404 故障。

### 5. 鉴权管理 API 扩展与平滑过渡
- **设计选择**：
  - 新增 `GET /api/v1/settings/auth`：返回当前管理端与导出链接的开关状态、生效安全模态以及是否存在主令牌；
  - 新增 `PUT /api/v1/settings/auth`：支持原子更新开关状态与可选的主令牌；
  - 保留旧版 `POST/GET /api/v1/settings/admin-token`，底层无缝桥接至统一令牌更新逻辑；
  - 维护公共探测端点 `GET /api/v1/auth/status`，准确反映管理端当前运行模态（Open 或 Protected）。

### 6. 前端设置中心与发布工作台交互对齐
- **设计选择**：
  - `SettingsView.vue`：拆分为管理端保护开关、导出链接保护开关与统一系统令牌维护卡片，并明确标注“统一系统令牌可同时用于管理端登录与受控导出下载”；
  - `PublicationsView.vue`：当数据库 0 记录时，呈现醒目的未发布引导空状态卡片，提示用户点击发布；根据导出开关动态决定展示与复制的链接格式（免密直链为纯净 URL，受控链接携带 Token），并更新使用说明文案。

## Risks / Trade-offs

- **[风险 1：导出免密后节点配置暴露]** → 对策：数据库迁移默认值强制为 1（受控）；前端设置界面在关闭导出鉴权时弹出显式警告提示，说明此操作将使任何知晓发布 ID 的人均可拉取节点配置。
- **[风险 2：统一系统主令牌越权风险]** → 对策：管理端受控时，专属发布令牌 `pub_<hex>` 访问控制面返回 403 Forbidden `invalid_token_scope`；管理端显式开放或未设置主令牌时所有请求均以开放模式放行，不能宣称发布令牌在此模式下具备权限隔离。开启导出保护时，仅匹配该发布专属令牌或系统统一主令牌的请求可拉取配置。
- **[风险 3：既有自动化脚本与旧接口兼容性]** → 对策：既有 `/api/v1/settings/admin-token` 保持完整保留与向后兼容，所有旧接口均通过单元测试与回归测试保障功能不变。
