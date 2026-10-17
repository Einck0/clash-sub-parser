# Technical Plan: Minimal Client Subscriptions (Zero-Glue Clean Slate)

- Feature: `specs/004-publication/`
- Status: **IMPLEMENTING** — 2026-10-17 架构重塑与过度设计消除
- Reference: Einck 当前原话最高指令；彻底推平过度工程化模型，统一采用 Settings Token。

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
// 逻辑流程：
if holder != nil && holder.IsExportAuthRequired() && holder.AdminMode() == SecurityModeProtected {
    candidate := extractCandidateToken(r) // 从 ?token=... 或 Authorization: Bearer ... 提取
    if candidate == "" || !holder.Verify(candidate) {
        WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication credentials required or invalid")
        return
    }
}
// Open Mode 或 exportAuthEnabled == false 时直接免密放行
```
- **单一真源**：整个系统的密钥管理只有系统“设置”里的全局 Token，修改设置即全局生效；
- **免密无缝**：Open Mode 下无需提供任何 Token，在本地回环或受信任局域网中直接裸拉。

### 2.3 现场动态编译链路 (Live On-Request Compilation)
在 `internal/application/publication/service.go` 中新增/精简导出方法：
```go
func (s *Service) GenerateLiveTargetArtifact(ctx context.Context, target domain.CompilerTarget) (*Artifact, error)
```
- 请求到达时现场通过 `factsLoader.LoadConsistentFacts(ctx, "", target, domain.NowUTC())` 读取已提交的最新节点、分组、规则与准入事实；
- 通过 `BuildCurrentArtifact(ctx, facts, target)` 在内存中执行策略解析与目标编译渲染；
- 保留并发信号量（`concurrencySem`）保护服务器资源；
- 支持标准 `ETag` 与 `If-None-Match`，匹配时返回 `304 Not Modified`。

### 2.4 前端极简化改造 (`web/src/features/publications/`)
- 彻底移除 `usePublications.ts` 中对 `subscription_links` CRUD 接口的调用；
- 移除“添加订阅链接”“轮转 Token”“吊销”“掩码”“公网基准地址强制配置”等全部伪需求逻辑；
- `PublicationsView.vue` 仅保留 4 张卡片（或 4 行）：
  - 目标：Mihomo/Clash（完整配置）、Sing-box（节点订阅）、Surge（节点订阅）、Quantumult X（节点订阅）；
  - 直链地址：结合 `window.location.origin` 与客户端路由（如 `${origin}/clash`）；如果系统处于保护模式，自动从 `localStorage.getItem('csp_token')` 拼接 `?token=${storedToken}`；
  - 按钮：“复制订阅链接”、“下载配置”；
  - 剪贴板失败时提供手动选择全选框。

---

## 3. 兼容性与安全约束

1. **旧历史路由保全**：
   - 历史只读快照 `/publish/v1/{publication_id}` 与 `/p/{publication_id}` 依然保留支持；
   - 遗留路由 `/yaml` 与 `/script` 维持 410 Gone；
2. **安全防护**：
   - 导出端点强制设置 `Cache-Control: no-store` 与 `Referrer-Policy: no-referrer`；
   - Nginx 已有的 CSP 专属安全日志（去 query 参数）继续提供反代层防泄漏保护；
3. **资产零丢失**：
   - 数据库中不需要增加新表，原有业务表 100% 保持；
   - 筛选算法 `cheap = all - expensive - other` 严格不变。

---

## 4. 验证与上线规划

1. **执行子机自测**：
   - 编写针对 `/clash`、`/singbox`、`/surge`、`/qx` 及 `/sub/{target}` 路由的单元测试与 Settings Token 鉴权测试；
   - 前端 Vitest 单元测试与 vue-tsc 类型检查全绿；
2. **独立 Reviewer 审查**：核对是否彻底消除了过度设计与多余的数据库状态机；
3. **Critic 真机多模态验收**：在隔离预览端口执行端到端流程与多视口 PNG 视觉审视；
4. **Dev 云构建与生产上线**：提交代码至 `dev`，由官方 Actions 构建镜像，在线热备数据库，更新生产容器并做公网 TLS 域名健康核验。
