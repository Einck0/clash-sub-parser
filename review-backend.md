# Clash Subscription Parser — 后端代码审查报告

> 审查时间：2026-06-27  
> 审查范围：`backend/app/` 全部 Python 源文件  
> 审查方法：逐文件静态代码审查，聚焦逻辑缺陷、安全漏洞、功能优化、架构问题

---

## 目录

1. [🔴 严重问题](#-严重问题)
2. [🟡 中等问题](#-中等问题)
3. [🟢 建议改进](#-建议改进)
4. [问题汇总表](#问题汇总表)

---

## 🔴 严重问题

### 🔴-01 路由顺序导致 `/nodes/all` 端点不可达

**文件：** `app/routers/subscriptions.py` 第 69–81 行

**问题：** FastAPI 按定义顺序匹配路由。`/{subscription_id}/nodes`（第 69 行）定义在 `/nodes/all`（第 79 行）之前。当请求 `GET /api/subscriptions/nodes/all` 时，FastAPI 会先尝试匹配 `/{subscription_id}/nodes`，将 `subscription_id` 解析为 `"nodes"`。由于 `subscription_id` 类型为 `int`，FastAPI 直接返回 **422 Validation Error**，永远不会路由到 `/nodes/all`。

```python
# 第 69 行 — 先定义，会先匹配
@router.get("/{subscription_id}/nodes", response_model=list[dict])
async def get_subscription_nodes(subscription_id: int, ...): ...

# 第 79 行 — 永远不会被 /subscriptions/nodes/all 匹配到
@router.get("/nodes/all", response_model=list[dict])
async def get_all_subscription_nodes(...): ...
```

**影响：** `/api/subscriptions/nodes/all` 端点完全无法使用。

**修复建议：** 将 `/nodes/all` 路由定义移到 `/{subscription_id}/nodes` **之前**，或者将路径改为不与动态路径冲突的名称，如 `/all-nodes`。

---

### 🔴-02 Cookie 存储原始认证 Token（敏感信息泄露）

**文件：** `app/routers/settings.py` 第 97–111 行

**问题：** `login_auth_token_endpoint` 将用户提交的 **原始认证 token** 直接写入 cookie：

```python
response.set_cookie(
    AUTH_COOKIE,          # "clash_auth_token"
    payload.token,        # ← 原始 token，不是会话标识！
    httponly=True,
    secure=settings.auth_cookie_secure or request.url.scheme == "https",
    samesite="lax",
    max_age=60 * 60 * 24 * 30,  # 30 天！
)
```

这意味着：
- 原始凭据以明文形式存储在浏览器 cookie 中，有效期长达 30 天
- 如果发生 XSS 漏洞，攻击者可通过 JavaScript 读取 cookie 拿到原始 token
- 攻击者拿到 cookie 就直接拥有了永久认证能力（不是临时会话）

**修复建议：** 
1. 登录成功后生成一个随机会话 ID，将会话 ID 存入 cookie，将会话与用户的对应关系存在服务端（内存或数据库）
2. 会话应有独立的过期机制（如 24 小时），支持主动注销
3. 或至少将 token hash 而非原始 token 存入 cookie（但仍需配合服务端验证）

---

### 🔴-03 SSRF — 下载重定向未验证私有地址

**文件：** `app/services/download_service.py` 第 72–126 行

**问题：** `download_url` 函数使用 `httpx.AsyncClient(follow_redirects=True)`，但只在请求发起前调用 `validate_fetch_url` 验证初始 URL。如果初始 URL 是合法公网地址，但服务器返回 302 重定向到 `http://169.254.169.254/latest/meta-data/`（AWS 元数据端点）或 `http://127.0.0.1:6379/`（Redis），重定向会自动跟随，绕过 SSRF 防护。

对比之下，`subscription_service.py` 中的 `_fetch_subscription_text` 手动处理重定向并逐次验证目标地址（第 199–210 行），下载服务却没有这样做。

**修复建议：** 禁用自动重定向（`follow_redirects=False`），手动处理重定向并在每次跳转前调用 `validate_fetch_url` 验证目标地址。

---

### 🔴-04 SSRF — DNS 重绑定攻击可绕过地址验证

**文件：** `app/utils/validators.py` 第 20–50 行

**问题：** `validate_fetch_url` 通过 `socket.getaddrinfo` 解析域名为 IP 地址并验证是否为私有地址。但这是一个经典的 TOCTOU（Time-of-Check-Time-of-Use）漏洞：

1. 第一次解析：`evil.com` → `1.2.3.4`（公网 IP，通过验证）
2. 实际请求时：`evil.com` → `127.0.0.1`（DNS TTL 过期或攻击者控制的权威 DNS 返回不同结果）

攻击者只需让 DNS TTL 设为 0 或极短即可复现。

**修复建议：** 
1. 在 `httpx.AsyncClient` 中绑定解析后的 IP 地址（通过自定义 transport），确保连接时使用验证过的 IP
2. 或使用 HTTP 代理层（如自制 resolver）强制使用首次解析结果

---

### 🔴-05 并发下主订阅设置存在竞态条件

**文件：** `app/services/subscription_service.py` 第 32–34 行、第 90–93 行

**问题：** 设置主订阅的流程是：
1. `_clear_primary()` — 将所有 `is_primary=True` 的订阅设为 False
2. 新建/更新的订阅设置 `is_primary=True`
3. `commit()`

两个并发请求（A 和 B）可能同时执行步骤 1，然后各自将自己的订阅设为主订阅，最终导致两个 `is_primary=True` 的订阅并存。

```python
async def create_subscription(db, payload):
    if payload.is_primary:
        await _clear_primary(db)    # ← 请求 A 和 B 同时执行
    ...
    item = Subscription(**data)      # ← A 和 B 各自创建
    ...
    await db.commit()                # ← 两个都是主订阅
```

**影响：** 数据一致性被破坏，后续生成配置时可能产生异常行为。

**修复建议：** 
1. 使用数据库唯一约束 + 部分索引（如 PostgreSQL 的 `WHERE is_primary = TRUE` 唯一索引，SQLite 可用触发器）
2. 或用 `SELECT ... FOR UPDATE` 锁定当前主订阅记录后再修改

---

## 🟡 中等问题

### 🟡-01 `protect_frontend` 设置从未被强制执行

**文件：** `app/main.py` 第 46–69 行、`app/utils/auth.py` 第 64–68 行

**问题：** `SecuritySettings` 模型有 `protect_frontend` 字段，但认证中间件和 `request_needs_auth` 函数完全没有检查它：

```python
# auth.py
def request_needs_auth(path: str, security) -> bool:
    if not security.auth_enabled:
        return False
    if is_export_path(path):
        return security.protect_exports
    if is_api_path(path):
        return security.protect_api
    return False  # ← 前端路径永远不需认证
```

中间件中也直接跳过非 API/非 Export 路径：
```python
needs_auth = is_api_path(request.url.path) or is_export_path(request.url.path)
if not needs_auth:
    return await call_next(request)  # ← 前端资源直接放行
```

**影响：** 用户以为配置了前端保护，但实际上前端页面始终未受保护。

**修复建议：** 在 `request_needs_auth` 中增加前端路径判断，在中间件中检查 `security.protect_frontend`。

---

### 🟡-02 `datetime.utcnow()` 已弃用

**文件：** `app/services/subscription_service.py` 第 182 行

**问题：** 
```python
now = datetime.utcnow()  # Python 3.12+ 已弃用
```

**修复建议：** 替换为 `datetime.now(timezone.utc)`。

---

### 🟡-03 硬编码的 cron 策略和重入锁不可靠

**文件：** `app/services/scheduler.py` 第 12–25 行

**问题：** 使用全局 `_running` 变量作为重入保护：
```python
_running = False

async def _poll_subscriptions() -> None:
    global _running
    if _running:
        return
    _running = True
    ...
```

虽然在 asyncio 单线程上下文中能工作，但：
1. 如果 `fetch_due_subscriptions` 抛出未预期的异常（不在 `finally` 之前），`_running` 可能永远为 True，导致轮询永久停止
2. APScheduler 配置为每 1 分钟执行一次（第 31 行），但实际轮询间隔应该由 `update_interval` 控制。每 1 分钟的调度间隔意味着最短可配置间隔就是 1 分钟，但用户可能设 `update_interval=5`（5 分钟）然后困惑为什么不是精确的 5 分钟

**修复建议：** 
1. 将锁改为 `asyncio.Lock` 更安全
2. 考虑使用 APScheduler 的单实例模式 `max_instances=1` 而不是手动锁

---

### 🟡-04 规则分类删除时级联删除所有关联规则（无警告）

**文件：** `app/services/rule_category_service.py` 第 97–100 行

**问题：** 删除分类时直接删除该分类下的所有规则：
```python
async def delete_rule_category(db: AsyncSession, item: RuleCategory) -> None:
    await db.execute(delete(Rule).where(Rule.category == item.name))
    await db.delete(item)
```

这是一个破坏性操作且不可逆。API 没有任何参数让用户选择"保留规则并移到其他分类"或"删除规则"。前端可能也没有明确提示用户这会删除所有关联规则。

**修复建议：** 
1. 增加可选参数 `delete_rules: bool = True`，默认行为保持兼容
2. 当 `delete_rules=False` 时，将规则的 category 改为 `"default"` 或其他指定分类
3. 返回实际删除的规则数量给客户端

---

### 🟡-05 导入端点缺少请求体大小限制

**文件：** `app/routers/settings.py` 第 156–222 行

**问题：** `import_config_endpoint` 使用 `await request.json()` 读取整个请求体，没有大小限制。攻击者可以发送超大 JSON 导致内存耗尽。

**修复建议：** 在 FastAPI 中配置 `max_body_size`，或手动限制读取的字节数。

---

### 🟡-06 `ensure_rule_category` 存在竞态条件

**文件：** `app/services/rule_category_service.py` 第 28–37 行

**问题：** 先查询是否存在，不存在则创建。两个并发请求可能同时发现不存在，然后同时尝试创建，导致其中一个因唯一约束冲突失败。

```python
async def ensure_rule_category(db: AsyncSession, name: str) -> RuleCategory:
    result = await db.execute(select(RuleCategory).where(...))
    item = result.scalar_one_or_none()
    if item:
        return item
    # ← 此处另一个请求可能已创建同名分类
    item = RuleCategory(name=category_name, sort_order=...)
    db.add(item)
    await db.commit()  # ← IntegrityError!
```

**修复建议：** 捕获 `IntegrityError` 后重试查询，或使用 `INSERT ... ON CONFLICT DO NOTHING` 然后重新查询。

---

### 🟡-07 Pydantic Schema 缺少长度约束（与数据库列不匹配）

**文件及行号：**
- `app/schemas/subscription.py` — `SubscriptionBase.name: str`（无 max_length，DB 限制 120）
- `app/schemas/subscription.py` — `SubscriptionBase.url: str`（无 max_length，DB 为 Text 但应防滥用）
- `app/schemas/rule.py` — `RuleBase.type: str`（无 max_length，DB 限制 40）
- `app/schemas/rule.py` — `RuleBase.value: str`（无 max_length，DB 限制 512）
- `app/schemas/rule.py` — `RuleBase.proxy: str`（无 max_length，DB 限制 160）
- `app/schemas/node_group.py` — `NodeGroupBase.name: str`（无 max_length，DB 限制 120）

**问题：** 未在 Pydantic 层验证字段长度，超长数据会直接到达数据库，依赖 DB 的 `String(N)` 约束报错。这会产生不友好的错误信息（SQL 错误而非 HTTP 400）。

**修复建议：** 在每个字段上添加 `Field(max_length=N)` 与数据库列定义保持一致。

---

### 🟡-08 DNS 配置更新缺少 `raw_yaml` 大小限制

**文件：** `app/schemas/dns.py` 第 9 行、`app/routers/dns.py` 第 23–38 行

**问题：** `DnsConfigUpdate.raw_yaml: str` 没有长度约束。YAML 字段存储在数据库的 `Text` 类型列中，理论上无大小限制。恶意用户可提交超大 YAML 内容。

**修复建议：** 添加 `Field(max_length=...)` 约束。

---

### 🟡-09 `_refresh_selected_nodes` 回退逻辑可能导致数据不一致

**文件：** `app/services/subscription_service.py` 第 193–215 行

**问题：** 当 `source_nodes` 为空但 `raw_nodes` 不为空时，函数用 `raw_nodes` 作为"源"重新应用过滤：
```python
if not source_nodes and item.raw_nodes:
    source_nodes = item.raw_nodes
    fallback_to_current = True
```

`raw_nodes` 是已经经过前缀处理后的数据。当 `fallback_to_current=True` 时，虽然跳过了再次添加前缀，但过滤逻辑使用的是**已加前缀的名字**来匹配 `include_node_names`/`exclude_node_names`，而用户输入的是**原始名字**。这会导致过滤不匹配。

**修复建议：** 如果没有 `source_nodes`，应提醒用户需要先 fetch 获取节点，或在创建时就强制要求有数据源。

---

### 🟡-10 `import_config_endpoint` 验证端点不做任何写入但也不验证数据结构

**文件：** `app/routers/settings.py` 第 224–250 行

**问题：** `import_validate_endpoint` 只检查 table 名称和 rows 是否为数组，不验证每个 row 的字段是否合法。"验证"端点名不副实。

**修复建议：** 增加对每行数据的基本字段校验（列名、类型），返回详细的验证结果。

---

### 🟡-11 导出的 `security_settings` 不含 `token_hash`，但导入时逻辑有隐含依赖

**文件：** `app/routers/settings.py` 第 253 行（导出排除）和第 185–197 行（导入恢复）

**问题：** 导出时 `_serialize_model` 排除了 `token_hash`。导入时从当前数据库读取 `preserved_token_hash` 并恢复。逻辑正确，但如果用户导入到一个全新数据库（没有 security_settings 记录），`current_security` 为 None，`preserved_token_hash` 为空字符串。最终 `security_settings` 记录会被创建但 token 为空——auth_enabled 如果被导入为 True，会导致锁死（无法登录但需要认证）。

**修复建议：** 在导入后检查 `auth_enabled=True` 但 `token_hash` 为空的矛盾状态，抛出警告或自动关闭 auth。

---

### 🟡-12 `node_group_service.py` 中 `preview_node_groups` 中 `include_group_names` 类型可能不一致

**文件：** `app/services/node_group_service.py` 第 126–139 行

**问题：** 在 `_resolve_entries_from_group` 返回的 entries 中，`"group"` 和 `"group_nodes"` 类型的 `value` 已经被 `_normalize_entries` 转为 `int`。但在 `preview_node_groups` 的后续逻辑中：

```python
for entry in entries:
    entry_type = entry.get("type")
    entry_value = entry.get("value")
    if entry_type == "group" and entry_value in group_map:
```

`entry_value` 是 `int`，`group_map` 的 key 也是 `int`，所以比较本身没问题。但当回退到老数据（没有 `include_entries` 只有 `include_group_ids`），`_resolve_entries_from_group` 产生的 value 就是从 `include_group_ids` 取的 int。这种混合处理虽然目前能工作，但非常脆弱。

---

### 🟡-13 缺少全局错误处理中间件

**文件：** `app/main.py` 全局

**问题：** 没有全局异常处理器。未预期的异常会返回 FastAPI 默认的 500 错误响应（包含 Python traceback 信息），可能泄露内部实现细节。

**修复建议：** 添加 `@app.exception_handler(Exception)` 全局处理器，返回通用错误信息，同时将详细错误记录到日志。

---

### 🟡-14 CORS 默认配置过于宽松

**文件：** `app/config.py` 第 23–25 行

**问题：** 
```python
cors_allow_methods: list[str] = ["*"]
cors_allow_headers: list[str] = ["*"]
```

虽然 `cors_allow_origins` 默认为空列表（同源限制），但如果用户设置了 `CLASH_CORS_ALLOW_ORIGINS`，配合 `methods=*` 和 `headers=*` 将完全开放 CORS。

**修复建议：** 将默认值收紧为具体的方法和头列表，如 `["GET", "POST", "PATCH", "DELETE", "OPTIONS"]` 和 `["Content-Type", "x-clash-token", "x-clash-csrf"]`。

---

## 🟢 建议改进

### 🟢-01 大量重复代码 — 多处功能相同的去重、回退、节点解析函数

**涉及文件：**
- `app/services/generate_service.py` — `_dedup_names`、`_with_fallback`、`_resolve_entries`
- `app/services/node_group_service.py` — `_uniq`、`_with_fallback`、`_resolve_entries_from_group`
- `app/utils/dedup.py` — `deduplicate_nodes`

**建议：** 将 `_dedup_names` / `_uniq` 统一使用 `dedup.py` 中的函数；将 `_with_fallback` 和 `_resolve_entries` 提取到共享工具模块。

---

### 🟢-02 `MATCH_RULE_TYPES` 和 `VALUE_RULE_TYPES` 重复定义

**涉及文件：**
- `app/services/rule_service.py` 第 10–22 行
- `app/services/generate_service.py` 第 12–25 行

**建议：** 提取到 `app/utils/constants.py` 或 `app/models/rule.py` 中统一定义。

---

### 🟢-03 列表端点缺少分页支持

**涉及文件：** 所有 `list_*` 函数和对应路由

**问题：** 所有列表端点返回全部记录，当数据量增长时会导致：
- 响应体过大
- 数据库全表扫描
- 前端渲染卡顿

**建议：** 至少为 `subscriptions`、`rules` 添加 `skip`/`limit` 分页参数，返回总数和分页信息。

---

### 🟢-04 缺少请求速率限制

**涉及文件：** `app/main.py` 全局

**问题：** 没有任何速率限制，可能被暴力破解 auth token 或进行 DoS 攻击。

**建议：** 使用 `slowapi` 或类似库为关键端点（login、import、fetch）添加速率限制。

---

### 🟢-05 缺少日志配置

**涉及文件：** 全局

**问题：** 仅在 `subscription_service.py` 中使用了 `logging.getLogger(__name__)`，但没有全局日志配置（格式、级别、输出目标）。其他模块完全没有日志记录。

**建议：** 在 `main.py` 的 `lifespan` 中配置 logging，统一日志格式和级别。

---

### 🟢-06 `app/database.py` 中手动 schema 迁移应使用 Alembic

**文件：** `app/database.py` 第 29–130 行

**问题：** `_bootstrap_schema` 函数通过 `PRAGMA table_info` + `ALTER TABLE ADD COLUMN` 手动迁移 schema。这种模式：
- 难以维护（每次加列都要手写迁移代码）
- 不支持回滚
- PostgreSQL 分支使用 `IF NOT EXISTS` 但 SQLite 分支需要先查再改

**建议：** 引入 Alembic 管理数据库迁移，bootstrap 函数仅用于初始化默认数据。

---

### 🟢-07 `_parse_vmess` 缺少 WebSocket/gRPC 传输参数解析

**文件：** `app/utils/clash_parser.py` 第 163–178 行

**问题：** VMess 协议解析器没有处理 `path`（WS 路径）、`host`（WS Host 头）、`type`（传输层类型如 ws/grpc/h2）等字段。对比 `_parse_vless` 和 `_parse_trojan` 都处理了这些参数，vmess 解析器会丢失这些配置信息。

**建议：** 补充 vmess 的传输层参数解析：
```python
if data.get("net") == "ws":
    ws_opts = {}
    if data.get("path"):
        ws_opts["path"] = data["path"]
    if data.get("host"):
        ws_opts["headers"] = {"Host": data["host"]}
    if ws_opts:
        node["ws-opts"] = ws_opts
```

---

### 🟢-08 `dedup.py` 的签名函数忽略节点配置差异

**文件：** `app/utils/dedup.py` 第 11–21 行

**问题：** `_build_signature` 只使用 `type+server+port+uuid/password` 作为去重签名。如果同一个 server:port 有不同配置（如不同 TLS 设置、不同网络类型、不同 SNI），它们会被当作重复节点去重。

**建议：** 将 `tls`、`network`、`servername` 等关键字段也纳入签名。

---

### 🟢-09 `subscription_service.py` 中 `fetch_due_subscriptions` 每次轮询都查询所有订阅

**文件：** `app/services/subscription_service.py` 第 220–243 行

**问题：** 每分钟执行一次，每次都 `select(Subscription)` 加载所有订阅到内存再逐一判断是否到期。随着订阅数量增长，效率会下降。

**建议：** 在 SQL 查询中过滤出需要更新的订阅：
```python
WHERE update_interval > 0 
  AND (last_fetched_at IS NULL OR last_fetched_at < :cutoff_time)
```

---

### 🟢-10 `_fetch_subscription_text` 中的 `hasattr(client, "stream")` 检查冗余

**文件：** `app/services/subscription_service.py` 第 189 行

**问题：** `httpx.AsyncClient` 始终有 `stream` 方法，这个条件永远为 False，导致 `if not hasattr(client, "stream")` 分支永远不会执行（死代码）。

**建议：** 移除这个条件分支，直接使用流式读取逻辑。

---

### 🟢-11 下载文件服务中的同步 I/O 阻塞事件循环

**文件：** `app/services/download_service.py` 第 93 行

**问题：** 
```python
with tmp.open("wb") as fh:
    async for chunk in response.aiter_bytes():
        fh.write(chunk)  # ← 同步写入，阻塞事件循环
```

**建议：** 使用 `aiofiles` 或 `asyncio.to_thread` 包装文件写入操作。

---

### 🟢-12 `Settings` 模型的 `lru_cache` 无法响应运行时环境变量变化

**文件：** `app/config.py` 第 30 行

**问题：** `@lru_cache` 使 `get_settings()` 的返回值在进程生命周期内不变。如果通过环境变量动态修改配置（如 Kubernetes ConfigMap 更新），不会生效。

**建议：** 如果不需要运行时热更新，当前实现可以保留，但在文档中注明。

---

### 🟢-13 前端静态文件服务缺少缓存头

**文件：** `app/main.py` 第 98–113 行

**问题：** `frontend_files` 返回 `FileResponse` 但没有设置 `Cache-Control` 头。浏览器每次都可能重新请求静态资源。

**建议：** 为 `/assets/` 路径下的文件设置 `Cache-Control: public, max-age=31536000, immutable`（文件名通常含 hash），为 `index.html` 设置 `Cache-Control: no-cache`。

---

### 🟢-14 `generate_service.py` 中 script 生成使用字符串拼接而非模板

**文件：** `app/services/generate_service.py` 第 87–102 行

**问题：** 用多行字符串拼接生成 JavaScript 代码。难以维护且容易出错。

**建议：** 使用 `textwrap.dedent` + 模板字符串，或将 JS 模板放在外部文件中加载。

---

### 🟢-15 `download_service.py` 中 `_safe_filename` 限制 180 字符但未说明原因

**文件：** `app/services/download_service.py` 第 166 行

**问题：** 文件名截断为 180 字符。大多数文件系统支持 255 字节。180 这个值缺乏依据。

**建议：** 使用 200 或 255 并添加注释说明原因。

---

## 问题汇总表

| 编号 | 严重等级 | 类别 | 文件 | 简述 |
|------|----------|------|------|------|
| 🔴-01 | 🔴 严重 | 逻辑 Bug | `routers/subscriptions.py` | 路由顺序导致 `/nodes/all` 不可达 |
| 🔴-02 | 🔴 严重 | 安全漏洞 | `routers/settings.py` | 原始 token 存入 cookie |
| 🔴-03 | 🔴 严重 | 安全漏洞 | `services/download_service.py` | 下载重定向未验证 SSRF |
| 🔴-04 | 🔴 严重 | 安全漏洞 | `utils/validators.py` | DNS 重绑定可绕过 SSRF 防护 |
| 🔴-05 | 🔴 严重 | 逻辑 Bug | `services/subscription_service.py` | 主订阅设置竞态条件 |
| 🟡-01 | 🟡 中等 | 功能缺陷 | `main.py` / `utils/auth.py` | `protect_frontend` 从未强制执行 |
| 🟡-02 | 🟡 中等 | 代码质量 | `services/subscription_service.py` | `datetime.utcnow()` 已弃用 |
| 🟡-03 | 🟡 中等 | 架构问题 | `services/scheduler.py` | 全局变量重入锁不可靠 |
| 🟡-04 | 🟡 中等 | 数据安全 | `services/rule_category_service.py` | 删除分类级联删除规则无确认 |
| 🟡-05 | 🟡 中等 | 安全漏洞 | `routers/settings.py` | 导入端点无请求体大小限制 |
| 🟡-06 | 🟡 中等 | 逻辑 Bug | `services/rule_category_service.py` | `ensure_rule_category` 竞态 |
| 🟡-07 | 🟡 中等 | 代码质量 | 多个 Schema 文件 | Pydantic schema 缺长度约束 |
| 🟡-08 | 🟡 中等 | 安全漏洞 | `schemas/dns.py` | DNS YAML 无大小限制 |
| 🟡-09 | 🟡 中等 | 逻辑 Bug | `services/subscription_service.py` | 回退逻辑使用已处理数据作为源 |
| 🟡-10 | 🟡 中等 | 功能缺陷 | `routers/settings.py` | 导入验证端点未真正验证数据 |
| 🟡-11 | 🟡 中等 | 逻辑 Bug | `routers/settings.py` | 导入到新库可能导致 auth 锁死 |
| 🟡-12 | 🟡 中等 | 代码质量 | `services/node_group_service.py` | entry_value 类型处理脆弱 |
| 🟡-13 | 🟡 中等 | 架构问题 | `main.py` | 缺少全局异常处理中间件 |
| 🟡-14 | 🟡 中等 | 安全漏洞 | `config.py` | CORS 默认配置过于宽松 |
| 🟢-01 | 🟢 建议 | 代码质量 | 多个 Service 文件 | 大量重复代码（去重/回退/解析） |
| 🟢-02 | 🟢 建议 | 代码质量 | `rule_service.py` / `generate_service.py` | 常量重复定义 |
| 🟢-03 | 🟢 建议 | 功能缺陷 | 所有 Router 文件 | 列表端点缺分页 |
| 🟢-04 | 🟢 建议 | 安全漏洞 | `main.py` | 缺少速率限制 |
| 🟢-05 | 🟢 建议 | 代码质量 | 全局 | 缺少日志配置 |
| 🟢-06 | 🟢 建议 | 架构问题 | `database.py` | 手动 schema 迁移应使用 Alembic |
| 🟢-07 | 🟢 建议 | 功能缺陷 | `utils/clash_parser.py` | vmess 解析缺少传输层参数 |
| 🟢-08 | 🟢 建议 | 逻辑 Bug | `utils/dedup.py` | 去重签名忽略配置差异 |
| 🟢-09 | 🟢 建议 | 性能优化 | `services/subscription_service.py` | 轮询加载全表无过滤 |
| 🟢-10 | 🟢 建议 | 代码质量 | `services/subscription_service.py` | 死代码（`hasattr` 检查） |
| 🟢-11 | 🟢 建议 | 性能优化 | `services/download_service.py` | 同步文件 I/O 阻塞事件循环 |
| 🟢-12 | 🟢 建议 | 架构问题 | `config.py` | `lru_cache` 不支持运行时更新 |
| 🟢-13 | 🟢 建议 | 性能优化 | `main.py` | 静态文件缺缓存头 |
| 🟢-14 | 🟢 建议 | 代码质量 | `services/generate_service.py` | JS 代码用字符串拼接生成 |
| 🟢-15 | 🟢 建议 | 代码质量 | `services/download_service.py` | 文件名截断值缺乏依据 |

---

## 统计

| 严重等级 | 数量 |
|----------|------|
| 🔴 严重 | 5 |
| 🟡 中等 | 14 |
| 🟢 建议 | 15 |
| **合计** | **34** |

---

## 优先修复建议

### 立即修复（P0）
1. **🔴-01** 路由顺序 — 将 `/nodes/all` 移到 `/{subscription_id}/nodes` 之前
2. **🔴-02** Cookie token 泄露 — 改用会话机制
3. **🔴-03** 下载 SSRF — 手动处理重定向验证

### 短期修复（P1，1-2 周内）
4. **🔴-04** DNS 重绑定 — 绑定解析 IP
5. **🔴-05** 主订阅竞态 — 添加唯一约束
6. **🟡-01** 前端保护未执行 — 补充认证逻辑
7. **🟡-04** 删除分类级联 — 添加用户确认参数
8. **🟡-07** Schema 长度约束 — 补充 Field(max_length=...)
9. **🟡-13** 全局异常处理 — 添加 exception handler

### 中期优化（P2，1 个月内）
10. 重构重复代码（🟢-01, 🟢-02）
11. 引入 Alembic（🟢-06）
12. 添加分页（🟢-03）和速率限制（🟢-04）
13. 补充日志配置（🟢-05）
