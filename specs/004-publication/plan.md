# Technical Plan: Minimal Client Subscriptions (Zero-Glue Clean Slate)

- Feature: `specs/004-publication/`
- Status: **IMPLEMENTING** — 2026-10-17 架构重塑与主订阅/策略组顺序修复
- Reference: Einck 当前原话最高指令；彻底推平过度工程化模型，统一采用 Settings Token，修复主订阅流量透传与策略组顺序。

---

## 1. 架构定位反思与重塑

### 1.1 为什么之前的设计是“过度设计”？
- **错误假设**：误把 CSP 视为面向多租户的 SaaS 机场发卡系统，引入了 `subscription_links` 数据表、UUIDv7 路径、64-hex 独立 Token 哈希存储、Token 轮换（Rotate）、单链吊销（Revoke）、强制公网基准地址（Public Base URL）等复杂实体与状态机。
- **用户痛点**：生成的链接极其晦涩（如 `/subscribe/v1/01a14780-38e6-7a79-ad38-e1bbc9753c4e?token=sub_c919e...`），肉眼无法识别客户端类型；配置与使用极其繁琐，充斥着无意义的管理概念。
- **本质归位**：CSP 是 Einck 的个人私有高性能代理订阅控制平面与编译器。用户就是 Einck 自己，客户端就是 Einck 自己的设备。需要的是极简、优雅、直观的常规直链与现场动态编译。

---

## 2. 核心架构设计方案

### 2.1 极简直观导出路由 (Direct Target Routes)
在 `internal/transport/http/router.go` 中直接注册人类可读的客户端端点：
- `/clash` 和 `/mihomo` -> 目标 `domain.TargetMihomo`
- `/singbox` -> 目标 `domain.TargetSingBox`
- `/surge` -> 目标 `domain.TargetSurge`
- `/qx` 和 `/quantumult-x` -> 目标 `domain.TargetQuantumultX`
- 通用路由：`/sub/{target}`（方便通过 path 参数动态指定）

### 2.2 统一 Settings Token 鉴权中间件 / Handler
废除查询 `subscription_links` 表与校验专用哈希的逻辑。
在导出请求处理函数中直接集成 `AdminTokenHolder`：
```go
if holder != nil && holder.IsExportAuthRequired() && holder.AdminMode() == SecurityModeProtected {
    candidate := extractCandidateToken(r) // 从 ?token=... 或 Authorization: Bearer ... 提取
    if candidate == "" || !holder.Verify(candidate) {
        WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication credentials required or invalid")
        return
    }
}
```
- **单一真源**：整个系统的密钥管理只有系统“设置”里的全局 Token，修改设置即全局生效；
- **免密无缝**：Open Mode 下无需提供任何 Token，在本地回环或受信任局域网中直接裸拉。

### 2.3 现场动态编译链路 (Live On-Request Compilation)
在 `internal/application/publication/service.go` 中提供导出方法：
```go
func (s *Service) GenerateTargetArtifact(ctx context.Context, target domain.CompilerTarget) (*Artifact, error)
```
- 请求到达时现场通过 `ConsistentFacts` 读取已提交的最新节点、分组、规则与准入事实；
- 在内存中执行策略解析与目标编译渲染，支持 `ETag` 与 `If-None-Match`（304 Not Modified）；
- 严格设置 `Cache-Control: no-store` 与 `Referrer-Policy: no-referrer`。

### 2.4 主订阅与流量透传方案 (Primary Subscription & Userinfo Forwarding)
- **数据库扩展（Migration 000023）**：
  ```sql
  ALTER TABLE subscriptions ADD COLUMN is_primary INTEGER NOT NULL DEFAULT 0;
  ALTER TABLE subscriptions ADD COLUMN userinfo TEXT NOT NULL DEFAULT '';
  ```
- **拉取阶段（Fetch）**：
  - `internal/fetch/fetch.go`: 在 `fetch.Response` 中增加 `UserInfo string` 字段，提取响应中的 `Subscription-Userinfo` 头部；
  - `inventory.Service` / `subscription.Service`: 拉取成功后，将提取到的 `UserInfo` 存入 `subscriptions.userinfo`。
- **设置主订阅**：
  - `PATCH /api/v1/subscriptions/{id}` 接收 `{"is_primary": true}`；
  - 事务更新目标订阅 `is_primary = 1`，其他所有订阅 `is_primary = 0`；若传入 `false` 则置为 `0`；
  - 遵循 `.exp.md` 规范：只在列表卡片操作，不在添加/编辑弹窗里塞选项。
- **直链导出透传**：
  - `internal/transport/http/subscription_direct.go`: 导出配置时，查询 `SELECT userinfo FROM subscriptions WHERE is_primary = 1 AND enabled = 1 LIMIT 1`；
  - 若存在且不为空，注入响应头 `Subscription-Userinfo: <userinfo>`。

### 2.5 策略组顺序与保序方案 (Policy Group Ordering)
- **数据库扩展（Migration 000023）**：
  ```sql
  ALTER TABLE node_groups ADD COLUMN position INTEGER NOT NULL DEFAULT 0;
  ```
- **仓储层与一致性快照**：
  - `internal/repository/sqlite/policy.go`、`consistent_facts.go`、`revision_snapshots.go`:
    读取与保存 `position`；查询排序改为 `ORDER BY position ASC, created_at ASC, id ASC`。
- **解析器与编译器**：
  - `internal/resolver/graph.go`: 排序改为：
    ```go
    sort.SliceStable(sortedGroups, func(i, j int) bool {
        if sortedGroups[i].Position != sortedGroups[j].Position {
            return sortedGroups[i].Position < sortedGroups[j].Position
        }
        if sortedGroups[i].Name != sortedGroups[j].Name {
            return sortedGroups[i].Name < sortedGroups[j].Name
        }
        return sortedGroups[i].ID < sortedGroups[j].ID
    })
    ```
- **API 与前端**：
  - 后端：`PATCH /api/v1/policies/groups/{id}` 支持更新 `position`；提供顺序调整接口；
  - 前端：`GroupCard.vue` 支持“上移 / 下移”操作，直观调整顺序并保存。

---

## 3. 兼容性与安全约束

1. **旧历史路由保全**：
   - 历史只读快照 `/publish/v1/{publication_id}` 与 `/p/{publication_id}` 依然保留支持；
   - 遗留路由 `/yaml` 与 `/script` 维持 410 Gone；
2. **存量数据与筛选规则保护**：
   - 保护存量节点与订阅数据；
   - 核心筛选公式 `cheap = all - expensive - other` 严格保留，绝不破坏原有筛选契约；
3. **单轨发布约束**：
   - 宿主机禁止重型编译；通过 Git 提交推送到 `dev` 分支，由官方 GitHub Actions 云端构建不可变容器镜像。
