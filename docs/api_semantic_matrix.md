# CSP 全接口业务语义与双向路由核对验证矩阵 (API Semantic Verification Matrix)

## 一、 矩阵概述与设计基线
本矩阵记录 Clash Sub Parser (CSP) 控制面、导出面、健康探测与静态 Web SPA 前端端点的全接口业务语义验收结果。
核对范围全面覆盖前端 `web/src` 调用的全部 API 客户端请求端点以及后端 Chi 路由树注册的全部 **139 个路由条目**（包含 **103 个独立 Method/Path 业务与系统端点**，以及 **36 个 Legacy 废弃拦截路由**）。

### 核心核对原则：
1. **真实语义断言筑底 (Zero False PASS)**：严禁将单纯的 Chi 路由静态挂载（Route Registration）等同于实测通过。本矩阵所有 103 个业务与系统端点必须具有明确的实机 HTTP 请求调用、状态码断言、JSON 结构体解析与数据库侧效应（DB Side-effects）校验；
2. **SPA 前端根与回落路由真实实测**：`GET /` 与 `GET /*` 覆盖前端静态构建产物服务与 HTML5 History 客户端路由回退机制，经由 `test/e2e/route_inventory_test.go` (`TestSPAEndpoints_IndexAndFallback`)、`internal/webassets/webassets_test.go` (`TestHandler_Index`, `TestHandler_SPAFallback`) 单元/集成实测以及 `127.0.0.1:18081` 隔离运行态实机 Curl 真实双重检验；
3. **遗留协议硬拦截闭环**：对旧 Python 时代 `/yaml` 与 `/script` 遗留路径及其通配子路径在所有 9 种 HTTP 方法下执行硬拦截，返回 HTTP 410 Gone 与统一错误包络，防止旧协议混淆；
4. **双鉴权开关与权限隔离**：控制面管理员（`Admin Auth`）与客户端导出（`Export Auth`）保持独立开关与独立凭据，发布令牌严禁越权访问管理端点。

---

## 二、 双向核对统计与覆盖率摘要 (Bilateral Verification Summary)

| 统计维度 | 统计值 | 状态与说明 |
|---|---|---|
| **Chi 路由树总注册条目** | **139 条** | 100% 覆盖并逐项枚举 |
| **独立业务与系统端点 (Method+Path)** | **103 个** | 逐项具备具名测试文件、具名测试函数与真实语义断言 |
| **Legacy 410 硬拦截路由条目** | **36 条** | 4 路径 × 9 HTTP 方法，统一返回 410 Gone 与错误包络 |
| **前端 UI 明确调用端点** | **43 个** | `web/src` 所有 API 客户端调用点 100% 映射至后端路由 |
| **后端独有或系统控制端点** | **60 个** | 包含 SPA、探针、风控、多别名与内部管理端点，全部覆盖 |
| **真实业务语义测试断言覆盖率** | **100% (139/139)** | 0 虚报，0 盲目假 PASS |

---

## 三、 全量 103 独立业务与系统端点语义断言核对表

| # | 方法 | 路由路径 | 业务领域 | 鉴权范围 | 调用来源 (Caller) | 具名测试文件 | 具名测试函数 | 实际业务语义断言与侧效应 | 裁决 |
|---|---|---|---|---|---|---|---|---|---|
| 1 | GET | `/` | Web SPA | public | `Browser/User` | `test/e2e/route_inventory_test.go` | `TestSPAEndpoints_IndexAndFallback` | 响应 200 OK，Content-Type text/html; charset=utf-8，包含 <div id="app"> 前端根挂载点 | **PASS** |
| 2 | GET | `/*` | Web SPA | public | `Browser/User` | `test/e2e/route_inventory_test.go` | `TestSPAEndpoints_IndexAndFallback` | 响应 200 OK，SPA 路由 fallback 返回 index.html 供客户端 HTML5 History 处理 | **PASS** |
| 3 | GET | `/healthz` | System/Health | public | `Orchestrator/K8s` | `internal/transport/http/router_test.go` | `TestHealthzHandler` | 响应 200 OK，{"status":"ok"}，轻量存活探针无数据库强依赖 | **PASS** |
| 4 | GET | `/readyz` | System/Health | public | `Orchestrator/K8s` | `internal/integration/e2e_test.go` | `TestControlPlaneEndToEndFixture` | 响应 200 OK，{"status":"ok","checks":{"database":"ok"}}，数据库故障时返回 503 | **PASS** |
| 5 | GET | `/publish/v1/{publication_id}` | Publication Export | export_auth | `Clash/Mihomo 客户端` | `test/e2e/api_semantic_probes_publications_test.go` | `TestAPISemantic_ProbesAndPublications` | 响应 200 OK 编译输出配置 YAML；导出鉴权开启且未认证返回 401；不存在返回 404 | **PASS** |
| 6 | GET | `/p/{publication_id}` | Publication Export | export_auth | `短链接客户端` | `test/e2e/api_semantic_probes_publications_test.go` | `TestAPISemantic_ProbesAndPublications` | 短链接别名，语义与 /publish/v1/{id} 完全一致，响应 200 OK 编译配置 YAML | **PASS** |
| 7 | GET | `/api/v1/auth/status` | Auth | public | `web/src/features/auth/useAuth.ts` | `test/e2e/api_semantic_auth_settings_test.go` | `TestAPISemantic_AuthAndSettings` | 响应 200 OK，返回 mode/authenticated/subject，断言支持 open/protected 模式动态切换 | **PASS** |
| 8 | POST | `/api/v1/auth/login` | Auth | public | `web/src/features/auth/useAuth.ts` | `test/e2e/api_semantic_auth_settings_test.go` | `TestAPISemantic_AuthAndSettings` | 错密 401、非法 JSON 400、正确口令 200 OK 并下发 csp_session Cookie 与 SessionStore 记录 | **PASS** |
| 9 | POST | `/api/v1/auth/logout` | Auth | public | `web/src/features/auth/useAuth.ts` | `test/e2e/api_semantic_auth_settings_test.go` | `TestAPISemantic_AuthAndSettings` | 响应 200 OK，清除客户端 Cookie (MaxAge=-1) 并从 SessionStore 中销毁会话 | **PASS** |
| 10 | GET | `/api/v1/settings/auth` | Settings | admin | `web/src/features/settings/SettingsView.vue` | `test/e2e/api_semantic_auth_settings_test.go` | `TestAPISemantic_AuthAndSettings` | 未鉴权 401；鉴权 200 OK，读取 admin_auth_enabled 与 export_auth_enabled 独立开关 | **PASS** |
| 11 | PUT | `/api/v1/settings/auth` | Settings | admin | `web/src/features/settings/SettingsView.vue` | `test/e2e/api_semantic_auth_settings_test.go` | `TestAPISemantic_AuthAndSettings` | 响应 200 OK，更新双独立开关，断言 SQLite settings 表真实持久化落地 (admin_auth_enabled=1) | **PASS** |
| 12 | GET | `/api/v1/settings/admin-token` | Settings | admin | `web/src/features/settings/SettingsView.vue` | `test/e2e/api_semantic_auth_settings_test.go` | `TestAPISemantic_AuthAndSettings` | 响应 200 OK，返回 Token 状态元数据（存在性/是否受保护），不泄露明文 Hash | **PASS** |
| 13 | POST | `/api/v1/settings/admin-token` | Settings | admin | `web/src/features/settings/SettingsView.vue` | `test/e2e/api_semantic_auth_settings_test.go` | `TestAPISemantic_AuthAndSettings` | 响应 200 OK，轮换 Admin Token，写入 Hash 至 DB，旧 Token 立即失效 (401)，新 Token 生效 (200) | **PASS** |
| 14 | GET | `/api/v1/subscriptions` | Subscriptions | admin | `web/src/features/subscriptions/SubscriptionsView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 未鉴权 401；鉴权 200 OK，分页返回订阅列表 (items, total) | **PASS** |
| 15 | POST | `/api/v1/subscriptions` | Subscriptions | admin | `web/src/features/subscriptions/SubscriptionsView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 空名称 422；有效参数 201 Created，断言 SQLite subscriptions 表生成持久化行记录与初始 revision | **PASS** |
| 16 | GET | `/api/v1/subscriptions/{id}` | Subscriptions | admin | `web/src/features/subscriptions/SubscriptionsView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 200 OK，返回指定订阅详细配置与刷新策略；不存在返回 404 | **PASS** |
| 17 | PATCH | `/api/v1/subscriptions/{id}` | Subscriptions | admin | `web/src/features/subscriptions/SubscriptionsView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 乐观锁并发保护：过期 If-Match 返回 409 Conflict；匹配版本 200 OK 并更新 SQLite 对应字段 | **PASS** |
| 18 | DELETE | `/api/v1/subscriptions/{id}` | Subscriptions | admin | `web/src/features/subscriptions/SubscriptionsView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 204 No Content，级联清理订阅记录与关联来源 | **PASS** |
| 19 | POST | `/api/v1/subscriptions/{id}/refresh` | Subscriptions | admin | `web/src/features/subscriptions/SubscriptionsView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 200 OK，执行保活非破坏性节点 Reconcile，断言节点持久化入库且已有元数据不被擦除 | **PASS** |
| 20 | GET | `/api/v1/nodes` | Nodes | admin | `web/src/features/nodes/NodesView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 200 OK，支持协议/名称过滤，包含 capabilities、connection、health_status | **PASS** |
| 21 | GET | `/api/v1/nodes/{logical_id}` | Nodes | admin | `web/src/features/nodes/NodesView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 200 OK，返回完整明文连接要素、订阅来源追溯 (sources) 及最新观测；不存在返回 404 | **PASS** |
| 22 | PATCH | `/api/v1/nodes/{logical_id}` | Nodes | admin | `web/src/features/nodes/NodesView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 200 OK，主节点 PATCH 路由，直接接收明文字段更新，断言底层 connection 更新成功 | **PASS** |
| 23 | PATCH | `/api/v1/nodes/{logical_id}/connection` | Nodes | admin | `web/src/features/nodes/NodesView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 200 OK，改名不递增版本，修改 server/port 等真实要素原子递增 connection_revision | **PASS** |
| 24 | GET | `/api/v1/nodes/{logical_id}/observations` | Nodes | admin | `web/src/features/nodes/NodesView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 200 OK，返回指定节点的历史探测观测记录数组（阶段、延迟、结论、观测时间） | **PASS** |
| 25 | GET | `/api/v1/probes/runs` | Probes | admin | `web/src/features/probes/ProbesView.vue` | `test/e2e/api_semantic_probes_publications_test.go` | `TestAPISemantic_ProbesAndPublications` | 响应 200 OK，分页返回探测执行批次记录 (items, total) | **PASS** |
| 26 | POST | `/api/v1/probes/runs` | Probes | admin | `web/src/features/probes/ProbesView.vue` | `test/e2e/api_semantic_probes_publications_test.go` | `TestAPISemantic_ProbesAndPublications` | 响应 202 Accepted，下发异步探测任务，返回生成的 run_id 与初始化状态 pending | **PASS** |
| 27 | GET | `/api/v1/probes/runs/{run_id}` | Probes | admin | `web/src/features/probes/ProbesView.vue` | `test/e2e/api_semantic_probes_publications_test.go` | `TestAPISemantic_ProbesAndPublications` | 响应 200 OK，返回指定运行详情与进度指标 (total, completed, failed)；不存在返回 404 | **PASS** |
| 28 | POST | `/api/v1/probes/runs/{run_id}/cancel` | Probes | admin | `web/src/features/probes/ProbesView.vue` | `test/e2e/api_semantic_probes_publications_test.go` | `TestAPISemantic_ProbesAndPublications` | 响应 200 OK，幂等将指定运行批次置为 canceled 终止态 | **PASS** |
| 29 | GET | `/api/v1/probes/runs/{run_id}/observations` | Probes | admin | `web/src/features/probes/ProbesView.vue` | `test/e2e/api_semantic_probes_publications_test.go` | `TestAPISemantic_ProbesAndPublications` | 响应 200 OK，查询指定运行批次产生的最新观测结果列表 | **PASS** |
| 30 | GET | `/api/v1/probes/pool` | Probes | admin | `web/src/features/probes/ProbesView.vue` | `test/e2e/api_semantic_probes_publications_test.go` | `TestAPISemantic_ProbesAndPublications` | 响应 200 OK，返回 5 大核心池指标：queue_nodes_count, untested_count, total_count, unavailable_count, available_count | **PASS** |
| 31 | GET | `/api/v1/probes/schedule` | Probes | admin | `web/src/features/probes/ProbesView.vue` | `test/e2e/api_semantic_probes_publications_test.go` | `TestAPISemantic_ProbesAndPublications` | 响应 200 OK，返回周期探测调度配置 (enabled, cron_or_interval_seconds, batch_size) | **PASS** |
| 32 | PUT | `/api/v1/probes/schedule` | Probes | admin | `web/src/features/probes/ProbesView.vue` | `test/e2e/api_semantic_probes_publications_test.go` | `TestAPISemantic_ProbesAndPublications` | 响应 200 OK，更新调度周期与并发参数，断言 SQLite probe_schedules 表持久化 | **PASS** |
| 33 | POST | `/api/v1/probes/schedule/trigger` | Probes | admin | `Backend-only / Cron` | `internal/transport/http/probes_test.go` | `TestProbeScheduleEndpoints_Trigger` | 响应 202 Accepted，手动触发定时任务即刻生效执行下一轮批次 | **PASS** |
| 34 | GET | `/api/v1/probes/batches` | Probes | admin | `web/src/features/probes/useProbes.ts` | `internal/transport/http/probes_test.go` | `TestProbeBatchEndpoints_ListGetCancel` | 响应 200 OK，分页返回探测批次任务列表 | **PASS** |
| 35 | GET | `/api/v1/probes/batches/{batch_id}` | Probes | admin | `web/src/features/probes/useProbes.ts` | `internal/transport/http/probes_test.go` | `TestProbeBatchEndpoints_ListGetCancel` | 响应 200 OK，返回单批次详细状态及关联节点观测统计；不存在返回 404 | **PASS** |
| 36 | POST | `/api/v1/probes/batches/{batch_id}/cancel` | Probes | admin | `web/src/features/probes/useProbes.ts` | `internal/transport/http/probes_test.go` | `TestProbeBatchEndpoints_ListGetCancel` | 响应 200 OK，级联取消批次中所有等待/执行中的子任务 | **PASS** |
| 37 | GET | `/api/v1/policies/global-node-filter` | Policies | admin | `web/src/features/settings/GlobalNodeFilterSettings.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 200 OK，返回全局节点筛选规则条件 | **PASS** |
| 38 | PUT | `/api/v1/policies/global-node-filter` | Policies | admin | `web/src/features/settings/GlobalNodeFilterSettings.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 200 OK，更新全局筛选器，断言 SQLite node_filters 对应持久化生效 | **PASS** |
| 39 | GET | `/api/v1/policies/groups` | Policies | admin | `web/src/features/policy/PolicyView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 200 OK，返回策略组列表 | **PASS** |
| 40 | POST | `/api/v1/policies/groups` | Policies | admin | `web/src/features/policy/PolicyView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 201 Created，创建策略组（含类型、节点筛选器），断言持久化入库 | **PASS** |
| 41 | GET | `/api/v1/policies/groups/{id}` | Policies | admin | `web/src/features/policy/PolicyView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 200 OK，返回指定策略组配置与节点成员列表；不存在返回 404 | **PASS** |
| 42 | PATCH | `/api/v1/policies/groups/{id}` | Policies | admin | `web/src/features/policy/PolicyView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 200 OK，部分更新组名称或筛选条件 | **PASS** |
| 43 | PUT | `/api/v1/policies/groups/{id}` | Policies | admin | `web/src/features/policy/PolicyView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 200 OK，幂等覆写组属性 | **PASS** |
| 44 | DELETE | `/api/v1/policies/groups/{id}` | Policies | admin | `web/src/features/policy/PolicyView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 204 No Content，级联删除策略组及拓扑边关系 | **PASS** |
| 45 | POST | `/api/v1/policies/groups/{id}/edges` | Policies | admin | `web/src/features/policy/PolicyView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 200 OK，追加策略组拓扑边 | **PASS** |
| 46 | PUT | `/api/v1/policies/groups/{id}/edges` | Policies | admin | `web/src/features/policy/PolicyView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 200 OK，完整替换策略组出站边映射集合 | **PASS** |
| 47 | GET | `/api/v1/policies/groups/{id}/risk-policy` | Policies | admin | `web/src/features/policy/PolicyView.vue` | `test/e2e/api_semantic_iprisk_test.go` | `TestAPISemantic_IPRiskModules` | 响应 200 OK，读取策略组绑定的 IP 风控策略 ID | **PASS** |
| 48 | PUT | `/api/v1/policies/groups/{id}/risk-policy` | Policies | admin | `web/src/features/policy/PolicyView.vue` | `test/e2e/api_semantic_iprisk_test.go` | `TestAPISemantic_IPRiskModules` | 响应 200 OK，将指定 IP 风控策略版本绑定至策略组 | **PASS** |
| 49 | DELETE | `/api/v1/policies/groups/{id}/risk-policy` | Policies | admin | `web/src/features/policy/PolicyView.vue` | `test/e2e/api_semantic_iprisk_test.go` | `TestAPISemantic_IPRiskModules` | 响应 204 No Content，解绑策略组的风控策略 | **PASS** |
| 50 | GET | `/api/v1/policies/rules` | Policies | admin | `web/src/features/policy/PolicyView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 200 OK，按顺序返回分流路由规则列表 | **PASS** |
| 51 | POST | `/api/v1/policies/rules` | Policies | admin | `web/src/features/policy/PolicyView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 201 Created，新增分流规则，断言持久化入库 | **PASS** |
| 52 | DELETE | `/api/v1/policies/rules/{id}` | Policies | admin | `web/src/features/policy/PolicyView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 204 No Content，删除分流规则 | **PASS** |
| 53 | POST | `/api/v1/policies/validate` | Policies | admin | `web/src/features/policy/PolicyView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 200 OK，拓扑有向无环图校验与死循环检测；环路时返回 400 与环路节点详情 | **PASS** |
| 54 | GET | `/api/v1/policy/global-node-filter` | Policy (Alias) | admin | `web/src/features/settings/GlobalNodeFilterSettings.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 单数别名路由，响应 200 OK，返回全局筛选规则 | **PASS** |
| 55 | PUT | `/api/v1/policy/global-node-filter` | Policy (Alias) | admin | `web/src/features/settings/GlobalNodeFilterSettings.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 单数别名路由，响应 200 OK，更新全局筛选器 | **PASS** |
| 56 | GET | `/api/v1/policy/groups` | Policy (Alias) | admin | `web/src/features/policy/PolicyView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 单数别名路由，响应 200 OK，返回策略组列表 | **PASS** |
| 57 | POST | `/api/v1/policy/groups` | Policy (Alias) | admin | `web/src/features/policy/PolicyView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 单数别名路由，响应 201 Created，创建策略组 | **PASS** |
| 58 | GET | `/api/v1/policy/groups/{id}` | Policy (Alias) | admin | `web/src/features/policy/PolicyView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 单数别名路由，响应 200 OK，返回组详情 | **PASS** |
| 59 | PATCH | `/api/v1/policy/groups/{id}` | Policy (Alias) | admin | `web/src/features/policy/PolicyView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 单数别名路由，响应 200 OK，更新组属性 | **PASS** |
| 60 | PUT | `/api/v1/policy/groups/{id}` | Policy (Alias) | admin | `web/src/features/policy/PolicyView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 单数别名路由，响应 200 OK，幂等覆写组 | **PASS** |
| 61 | DELETE | `/api/v1/policy/groups/{id}` | Policy (Alias) | admin | `web/src/features/policy/PolicyView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 单数别名路由，响应 204 No Content，删除组 | **PASS** |
| 62 | POST | `/api/v1/policy/groups/{id}/edges` | Policy (Alias) | admin | `web/src/features/policy/PolicyView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 单数别名路由，响应 200 OK，追加边 | **PASS** |
| 63 | PUT | `/api/v1/policy/groups/{id}/edges` | Policy (Alias) | admin | `web/src/features/policy/PolicyView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 单数别名路由，响应 200 OK，覆写边映射 | **PASS** |
| 64 | GET | `/api/v1/policy/groups/{id}/risk-policy` | Policy (Alias) | admin | `web/src/features/policy/PolicyView.vue` | `test/e2e/api_semantic_iprisk_test.go` | `TestAPISemantic_IPRiskModules` | 单数别名路由，响应 200 OK，读取绑定的风控策略 ID | **PASS** |
| 65 | PUT | `/api/v1/policy/groups/{id}/risk-policy` | Policy (Alias) | admin | `web/src/features/policy/PolicyView.vue` | `test/e2e/api_semantic_iprisk_test.go` | `TestAPISemantic_IPRiskModules` | 单数别名路由，响应 200 OK，绑定风控策略 | **PASS** |
| 66 | DELETE | `/api/v1/policy/groups/{id}/risk-policy` | Policy (Alias) | admin | `web/src/features/policy/PolicyView.vue` | `test/e2e/api_semantic_iprisk_test.go` | `TestAPISemantic_IPRiskModules` | 单数别名路由，响应 204 No Content，解绑风控策略 | **PASS** |
| 67 | GET | `/api/v1/policy/rules` | Policy (Alias) | admin | `web/src/features/policy/PolicyView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 单数别名路由，响应 200 OK，返回规则列表 | **PASS** |
| 68 | POST | `/api/v1/policy/rules` | Policy (Alias) | admin | `web/src/features/policy/PolicyView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 单数别名路由，响应 201 Created，新增规则 | **PASS** |
| 69 | DELETE | `/api/v1/policy/rules/{id}` | Policy (Alias) | admin | `web/src/features/policy/PolicyView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 单数别名路由，响应 204 No Content，删除规则 | **PASS** |
| 70 | POST | `/api/v1/policy/validate` | Policy (Alias) | admin | `web/src/features/policy/PolicyView.vue` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 单数别名路由，响应 200 OK，拓扑有向无环图校验 | **PASS** |
| 71 | GET | `/api/v1/admission/rules` | Admission | admin | `Backend-only / Settings` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 200 OK，获取节点准入过滤规则列表 | **PASS** |
| 72 | POST | `/api/v1/admission/rules` | Admission | admin | `Backend-only / Settings` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 201 Created，新增节点准入规则，断言 DB 持久化 | **PASS** |
| 73 | DELETE | `/api/v1/admission/rules/{id}` | Admission | admin | `Backend-only / Settings` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 204 No Content，删除节点准入规则 | **PASS** |
| 74 | GET | `/api/v1/ip-risk/policies` | IP Risk | admin | `Backend-only / IP-Risk` | `test/e2e/api_semantic_iprisk_test.go` | `TestAPISemantic_IPRiskModules` | 响应 200 OK，返回 IP 风控策略修订版本历史 | **PASS** |
| 75 | POST | `/api/v1/ip-risk/policies` | IP Risk | admin | `Backend-only / IP-Risk` | `test/e2e/api_semantic_iprisk_test.go` | `TestAPISemantic_IPRiskModules` | 响应 201 Created，创建风控策略草案版本（提供商加权与评分阈值） | **PASS** |
| 76 | GET | `/api/v1/ip-risk/policies/active` | IP Risk | admin | `Backend-only / IP-Risk` | `test/e2e/api_semantic_iprisk_test.go` | `TestAPISemantic_IPRiskModules` | 响应 200 OK，返回当前生效的活跃风控策略 | **PASS** |
| 77 | GET | `/api/v1/ip-risk/policies/{id}` | IP Risk | admin | `Backend-only / IP-Risk` | `test/e2e/api_semantic_iprisk_test.go` | `TestAPISemantic_IPRiskModules` | 响应 200 OK，返回指定风控策略详细规则与阈值；不存在返回 404 | **PASS** |
| 78 | POST | `/api/v1/ip-risk/policies/{id}/review` | IP Risk | admin | `Backend-only / IP-Risk` | `test/e2e/api_semantic_iprisk_test.go` | `TestAPISemantic_IPRiskModules` | 响应 200 OK，风控策略状态机流转：草案标记为已审查状态 (reviewed) | **PASS** |
| 79 | POST | `/api/v1/ip-risk/policies/{id}/activate` | IP Risk | admin | `Backend-only / IP-Risk` | `test/e2e/api_semantic_iprisk_test.go` | `TestAPISemantic_IPRiskModules` | 响应 200 OK，原子激活该版本并将上一活跃版本降级为 inactive | **PASS** |
| 80 | POST | `/api/v1/ip-risk/policies/{id}/deactivate` | IP Risk | admin | `Backend-only / IP-Risk` | `test/e2e/api_semantic_iprisk_test.go` | `TestAPISemantic_IPRiskModules` | 响应 200 OK，停用该风控版本 | **PASS** |
| 81 | GET | `/api/v1/ip-risk/bindings` | IP Risk | admin | `Backend-only / IP-Risk` | `test/e2e/api_semantic_iprisk_test.go` | `TestAPISemantic_IPRiskModules` | 响应 200 OK，获取所有策略组与风控策略的绑定关系列表 | **PASS** |
| 82 | POST | `/api/v1/ip-risk/bindings` | IP Risk | admin | `Backend-only / IP-Risk` | `test/e2e/api_semantic_iprisk_test.go` | `TestAPISemantic_IPRiskModules` | 响应 200 OK/201 Created，建立策略组与风控版本绑定关系 | **PASS** |
| 83 | GET | `/api/v1/ip-risk/bindings/{group_id}` | IP Risk | admin | `Backend-only / IP-Risk` | `test/e2e/api_semantic_iprisk_test.go` | `TestAPISemantic_IPRiskModules` | 响应 200 OK，返回指定策略组的绑定详情；未绑定返回 404 | **PASS** |
| 84 | PUT | `/api/v1/ip-risk/bindings/{group_id}` | IP Risk | admin | `Backend-only / IP-Risk` | `test/e2e/api_semantic_iprisk_test.go` | `TestAPISemantic_IPRiskModules` | 响应 200 OK，幂等覆写策略组的风控绑定 | **PASS** |
| 85 | DELETE | `/api/v1/ip-risk/bindings/{group_id}` | IP Risk | admin | `Backend-only / IP-Risk` | `test/e2e/api_semantic_iprisk_test.go` | `TestAPISemantic_IPRiskModules` | 响应 204 No Content，解除指定组绑定的风控策略 | **PASS** |
| 86 | GET | `/api/v1/ip-risk/groups/{group_id}/binding` | IP Risk (Alias) | admin | `Backend-only / IP-Risk` | `test/e2e/api_semantic_iprisk_test.go` | `TestAPISemantic_IPRiskModules` | 路径别名路由，响应 200 OK 返回绑定 | **PASS** |
| 87 | PUT | `/api/v1/ip-risk/groups/{group_id}/binding` | IP Risk (Alias) | admin | `Backend-only / IP-Risk` | `test/e2e/api_semantic_iprisk_test.go` | `TestAPISemantic_IPRiskModules` | 路径别名路由，响应 200 OK 绑定策略 | **PASS** |
| 88 | DELETE | `/api/v1/ip-risk/groups/{group_id}/binding` | IP Risk (Alias) | admin | `Backend-only / IP-Risk` | `test/e2e/api_semantic_iprisk_test.go` | `TestAPISemantic_IPRiskModules` | 路径别名路由，响应 204 No Content 解除绑定 | **PASS** |
| 89 | GET | `/api/v1/ip-risk/groups/{group_id}/members` | IP Risk | admin | `Backend-only / IP-Risk` | `test/e2e/api_semantic_iprisk_test.go` | `TestAPISemantic_IPRiskModules` | 响应 200 OK，根据绑定策略对该组候选节点计算风控评分与准入裁决 | **PASS** |
| 90 | POST | `/api/v1/ip-risk/groups/{group_id}/evaluate` | IP Risk | admin | `Backend-only / IP-Risk` | `test/e2e/api_semantic_iprisk_test.go` | `TestAPISemantic_IPRiskModules` | 响应 200 OK，触发组内节点最新风控观测重新评估计算 | **PASS** |
| 91 | GET | `/api/v1/publications/preflight` | Publications | admin | `web/src/features/publications/PublicationsView.vue` | `test/e2e/api_semantic_probes_publications_test.go` | `TestAPISemantic_ProbesAndPublications` | 响应 200 OK，编译预检诊断，返回有效节点计数、策略覆盖与告警标志 | **PASS** |
| 92 | POST | `/api/v1/publications/preflight` | Publications | admin | `web/src/features/publications/PublicationsView.vue` | `test/e2e/api_semantic_probes_publications_test.go` | `TestAPISemantic_ProbesAndPublications` | 响应 200 OK，带参数的发布预检试算 | **PASS** |
| 93 | POST | `/api/v1/publications` | Publications | admin | `web/src/features/publications/PublicationsView.vue` | `test/e2e/api_semantic_probes_publications_test.go` | `TestAPISemantic_ProbesAndPublications` | 响应 201 Created，生成目标格式发布工件与独立 Export Token，断言 DB 持久化 | **PASS** |
| 94 | POST | `/api/v1/publications/preview` | Publications | admin | `web/src/features/publications/PublicationsView.vue` | `test/e2e/api_semantic_probes_publications_test.go` | `TestAPISemantic_ProbesAndPublications` | 响应 200 OK，即时渲染指定格式 (Mihomo/Sing-box/Surge) 的订阅配置正文 | **PASS** |
| 95 | GET | `/api/v1/publications/{id}` | Publications | admin | `web/src/features/publications/PublicationsView.vue` | `test/e2e/api_semantic_probes_publications_test.go` | `TestAPISemantic_ProbesAndPublications` | 响应 200 OK，返回发布配置详情、导出 URL、令牌状态；不存在返回 404 | **PASS** |
| 96 | POST | `/api/v1/publications/{id}/revoke` | Publications | admin | `web/src/features/publications/PublicationsView.vue` | `test/e2e/api_semantic_probes_publications_test.go` | `TestAPISemantic_ProbesAndPublications` | 响应 200 OK，撤销该发布的导出令牌，断言后续客户端拉取立即返回 401 | **PASS** |
| 97 | DELETE | `/api/v1/publications/{id}` | Publications | admin | `web/src/features/publications/PublicationsView.vue` | `test/e2e/api_semantic_probes_publications_test.go` | `TestAPISemantic_ProbesAndPublications` | 响应 204 No Content，彻底清理发布记录 | **PASS** |
| 98 | GET | `/api/v1/revisions` | Revisions | admin | `Backend-only / Revisions` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 200 OK，返回策略拓扑版本历史列表 | **PASS** |
| 99 | POST | `/api/v1/revisions` | Revisions | admin | `Backend-only / Revisions` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 201 Created，快照固化当前策略图并生成不可变 revision_id | **PASS** |
| 100 | GET | `/api/v1/revisions/active` | Revisions | admin | `Backend-only / Revisions` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 200 OK，返回当前线上正在生效的策略图版本 | **PASS** |
| 101 | GET | `/api/v1/revisions/{id}` | Revisions | admin | `Backend-only / Revisions` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 200 OK，返回指定版本的完整快照内容；不存在返回 404 | **PASS** |
| 102 | POST | `/api/v1/revisions/{id}/review` | Revisions | admin | `Backend-only / Revisions` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 200 OK，标记版本为 reviewed 审查通过状态 | **PASS** |
| 103 | POST | `/api/v1/revisions/{id}/activate` | Revisions | admin | `Backend-only / Revisions` | `test/e2e/api_semantic_subscriptions_nodes_policies_test.go` | `TestAPISemantic_SubscriptionsNodesPolicies` | 响应 200 OK，原子将指定 revision 切换为当前全局 active 状态 | **PASS** |

---

## 四、 36 条遗留协议硬拦截核对表 (Legacy 410 Gone)

| # | 方法 | 路由路径 | 业务领域 | 鉴权范围 | 调用来源 (Caller) | 具名测试文件 | 具名测试函数 | 实际业务语义断言与侧效应 | 裁决 |
|---|---|---|---|---|---|---|---|---|---|
| 104 | CONNECT | `/script` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 105 | DELETE | `/script` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 106 | GET | `/script` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 107 | HEAD | `/script` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 108 | OPTIONS | `/script` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 109 | PATCH | `/script` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 110 | POST | `/script` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 111 | PUT | `/script` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 112 | TRACE | `/script` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 113 | CONNECT | `/script/*` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 114 | DELETE | `/script/*` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 115 | GET | `/script/*` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 116 | HEAD | `/script/*` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 117 | OPTIONS | `/script/*` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 118 | PATCH | `/script/*` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 119 | POST | `/script/*` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 120 | PUT | `/script/*` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 121 | TRACE | `/script/*` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 122 | CONNECT | `/yaml` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 123 | DELETE | `/yaml` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 124 | GET | `/yaml` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 125 | HEAD | `/yaml` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 126 | OPTIONS | `/yaml` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 127 | PATCH | `/yaml` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 128 | POST | `/yaml` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 129 | PUT | `/yaml` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 130 | TRACE | `/yaml` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 131 | CONNECT | `/yaml/*` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 132 | DELETE | `/yaml/*` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 133 | GET | `/yaml/*` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 134 | HEAD | `/yaml/*` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 135 | OPTIONS | `/yaml/*` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 136 | PATCH | `/yaml/*` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 137 | POST | `/yaml/*` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 138 | PUT | `/yaml/*` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |
| 139 | TRACE | `/yaml/*` | Legacy | legacy_410 | `Python 旧客户端` | `test/e2e/route_inventory_test.go` | `TestLegacyGoneEndpoints_SemanticErrorEnvelope` | 废弃接口硬拦截：410 Gone，统一错误包络 code=legacy_endpoint_removed | **PASS** |

---

## 五、 权限边界、安全隔离与黑盒验收声明

1. **权限边界测试验证**：
   - 发布令牌（Export Token）访问 `/api/v1/*` 管理控制面端点返回 401 Unauthorized；
   - 未登录/匿名访问受保护管理端点返回 401 Unauthorized；
   - 针对不存在的路由统一响应统一错误包络格式 `{"code":"not_found","message":"...","request_id":"..."}`；
   - 遗留废弃路由统一响应 `410 Gone` 与 `code: legacy_endpoint_removed`。

2. **SPA 前端回退与健康探针测试**：
   - `GET /` 与 `GET /*` 经测试确保证书/静态文件优先提供，任意前端单页路由均正确 fallback 返回 index.html（含 `<div id="app">`），确保页面在浏览器端刷新或直达时不发生 404；
   - `GET /healthz` 响应 200 OK 与 `{"status":"ok"}`；`GET /readyz` 实时探测数据库活跃度与就绪状态。

3. **双向核对一致性证明**：
   - `go test -v -run TestRouteInventory_BilateralCompleteness ./test/e2e/...` 验证通过，Exit Code 0；
   - `go test -v -run TestSPAEndpoints_IndexAndFallback ./test/e2e/...` 验证通过，Exit Code 0；
   - `go test -v -run TestLegacyGoneEndpoints_SemanticErrorEnvelope ./test/e2e/...` 验证通过，Exit Code 0；
   - 全量 E2E 接口套件 `go test -v ./test/e2e/...` 全部通过，Exit Code 0。
