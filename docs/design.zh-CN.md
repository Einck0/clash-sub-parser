# 设计说明

本文档记录 `clash-sub-parser` 的架构、数据模型、生成链路、鉴权模型和前端交互设计。具体规则明细、运行实例数据和线上运维记录不在本文档维护。

## 运行架构

```text
Browser / Clash Client
        |
        v
FastAPI single container :18080
        |-- /api/*                 REST API
        |-- /yaml                  YAML subscription output
        |-- /script                Script.js subscription output
        |-- /assets/*              frontend build assets
        `-- SPA fallback           Vue router fallback
        |
        v
SQLite / PostgreSQL
```

设计原则：

- 单服务优先，降低前后端双容器运维成本。
- 数据库是当前状态源；订阅、节点组、规则、DNS 和生成开关都以数据库为准。
- 前端负责编辑体验，复杂编辑使用草稿模式。
- 后端负责校验、引用解析、生成顺序和输出一致性。
- Token 保护是轻量访问控制；公网部署仍应配合 HTTPS 反代和网络访问控制。

## 后端分层

```text
backend/app/
├── main.py                  # FastAPI 入口、API 注册、鉴权中间件、静态资源
├── config.py                # 环境配置
├── database.py              # 数据库连接、Session、自动建表/补列
├── models/                  # SQLAlchemy 模型
├── schemas/                 # Pydantic 请求/响应模型
├── routers/                 # HTTP 路由
├── services/                # 业务逻辑
└── utils/                   # Clash 解析、鉴权、校验和辅助工具
```

路由层只处理 HTTP 协议和参数转换；核心逻辑放在 service 层。主要 service 职责：

- `subscription_service.py`：订阅拉取、解析、节点筛选、响应头保存、失败状态记录。
- `node_group_service.py`：节点组引用解析、动态正则匹配、循环引用检测和预览。
- `rule_service.py`：规则校验、CRUD 和排序。
- `rule_category_service.py`：分类排序、级联删除和统计。
- `dns_service.py`：DNS Raw YAML 读取和保存。
- `generate_service.py`：组合订阅、节点组、规则和 DNS，生成 YAML / Script.js。
- `security_settings_service.py`：运行时鉴权设置和 token hash 管理。

## 数据模型

### Subscription

保存订阅源、上游节点和拉取状态。

关键字段：

- `name`
- `url`
- `update_interval`
- `is_primary`
- `node_prefix`
- `filter_regex`
- `include_node_names`
- `exclude_node_names`
- `source_nodes`
- `raw_nodes`
- `fetch_comments`
- `last_fetched_at`
- `last_fetch_error`
- `fetch_failed_count`
- `subscription_userinfo`
- `profile_update_interval`
- `profile_web_page_url`

设计说明：

- `source_nodes` 保存上游原始节点，用于后续手动选择。
- `raw_nodes` 保存经过筛选、包含/排除和前缀处理后的最终节点。
- 订阅正文只保留真实 `#` 注释。
- 流量、到期和主页信息来自 HTTP headers。
- 主订阅 headers 会用于 `/yaml` 等订阅输出接口。

### NodeGroup

保存 Clash `proxy-groups`。

关键字段：

- `name`
- `kind`
- `group_type`
- `sort_order`
- `regex_rules`
- `include_entries`
- `exclude_nodes`
- `url_test_config`
- `load_balance_config`
- `fallback_config`
- `add_fallback`

设计说明：

- `include_entries` 统一表示静态节点、内建目标、节点组引用和节点组节点展开。
- `regex_rules` 表示动态匹配规则。
- 正则匹配结果默认不固化为静态节点；如需固化，必须显式冻结。
- `add_fallback` 用于在解析节点末尾追加 `REJECT` 兜底。历史迁移组默认 `false`；新建组默认 `true`。

### RuleCategory 与 Rule

规则分类只负责组织和排序，一条 `Rule` 记录对应一条 Clash rule。

`RuleCategory` 关键字段：

- `name`
- `sort_order`

`Rule` 关键字段：

- `name`
- `category`
- `type`
- `value`
- `proxy`
- `options`
- `sort_order`
- `enabled`

生成顺序先按分类顺序，再按分类内规则顺序。`MATCH` 类型不需要 `value`，其它类型通常需要完整的 `type/value/proxy`。

### DNS

保存当前 DNS Raw YAML 和启用状态。

关键字段：

- `raw_yaml`
- `enabled`

可视化编辑最终会同步为 YAML；Raw 模式允许保留高级字段。

### GenerateConfig

保存短链接使用的生成开关。

关键字段：

- `enabled`
- `subscriptions`
- `node_groups`
- `rules`
- `dns`
- `exclude_node_proxies`

`/yaml` 和 `/script` 使用该配置。完整下载接口仍可通过 query 临时覆盖。

### SecuritySettings

保存运行时安全设置。

关键字段：

- `auth_enabled`
- `protect_frontend`
- `protect_api`
- `protect_exports`
- `token_hash`
- `fetch_proxy_enabled`
- `fetch_proxy_url`

`.env` 中的 `CLASH_AUTH_ENABLED` 和 `CLASH_AUTH_TOKEN` 只作为首次初始化默认值；后续以数据库中的安全设置为准。

## 生成链路

```text
subscriptions
      |
      |-- filter_regex
      |-- include_node_names
      |-- exclude_node_names
      |-- node_prefix
      v
proxies

node_groups
      |
      |-- include_entries
      |-- regex_rules
      |-- group references
      |-- fallback
      v
proxy-groups

rule_categories + rules
      |
      |-- category sort_order
      `-- rule sort_order
      v
rules

dns.raw_yaml
      v
dns

final output: YAML / Script.js
```

关键约定：

- 正则为空表示订阅节点全选。
- 最终节点 = 正则命中节点 + 手动包含节点 - 手动排除节点。
- 节点组引用必须检测循环引用和无效引用。
- 规则不在计划文档维护长清单，具体内容以数据库为准。

## 鉴权设计

保护范围：

- 可选保护：Web UI、`/api/*`、`/yaml`、`/script`、`/api/generate/*/download`。
- 固定放行：`/health`、`/assets/*`、`/favicon.ico`、`/robots.txt`。

Token 来源：

- Web UI：`POST /api/settings/auth/login` 校验 token 后设置 HttpOnly `clash_auth_token` cookie。
- 管理 API：优先支持 cookie，也兼容 `X-Clash-Token` 和 `Authorization: Bearer ...`。
- 导出/订阅地址：支持 URL query `?token=...`，用于 Clash 客户端订阅。

CSRF 约束：

- 使用 cookie 认证的非只读管理 API 请求必须携带 `X-Clash-CSRF: 1`。
- 内置前端会自动添加该 header。
- 使用 header token 或 Bearer token 的脚本调用不依赖 cookie。

Token 存储：

- 后端保存 SHA-256 摘要，不保存明文 token。
- 前端不把 token 持久化到 localStorage。
- 前端仅在当前页面会话内保留用户刚输入的 token，用于生成带 query token 的导出链接。

部署建议：

- 暴露到局域网或公网时必须启用 token。
- 公开网络访问建议放在 HTTPS 反代后。
- HTTPS 场景建议设置 `CLASH_AUTH_COOKIE_SECURE=true`。
- 弱 token 仍可能被离线爆破；公开部署应使用随机长 token。

## 订阅拉取安全

默认安全边界：

- 只允许 `http` 和 `https`。
- 默认拒绝 localhost、私网、链路本地、多播、保留和未指定地址。
- 每次重定向前重新校验目标地址。
- 响应大小受 `CLASH_REQUEST_MAX_BYTES` 限制。
- 默认不信任宿主机代理环境变量。

需要拉取内网订阅时，必须显式设置：

```text
CLASH_ALLOW_PRIVATE_FETCH_URLS=true
```

订阅拉取代理通过 `CLASH_HTTP_PROXY`、`CLASH_HTTPS_PROXY`、`CLASH_ALL_PROXY` 映射到容器运行时代理变量，避免宿主机普通 `HTTP_PROXY` 被意外写入部署配置。

## 前端设计

```text
frontend/src/
├── App.vue                         # 全局布局与导航
├── auth.js                         # 页面会话 token、API header、导出 URL 拼接
├── api/index.js                    # API 封装与错误处理
├── components/
│   ├── UiState.vue                 # loading / empty / error / success 状态组件
│   └── NodePreviewList.vue         # 可搜索、可展开的节点预览组件
├── style.css                       # 全局样式
└── views/
    ├── Subscriptions.vue           # 订阅管理
    ├── SubscriptionForm.vue        # 订阅表单
    ├── NodeGroups.vue              # 节点组首页与预览
    ├── NodeGroupModal.vue          # 节点组编辑弹窗
    ├── Rules.vue                   # 规则分类首页和全局规则搜索
    ├── RuleCategoryDetail.vue      # 分类内规则编辑
    ├── DnsSettings.vue             # DNS 可视化和 Raw 编辑
    ├── Generate.vue                # 生成与下载
    └── Settings.vue                # 安全设置与 Token 管理
```

交互原则：

- 复杂列表编辑采用草稿模式和“保存全部”。
- 有未保存变更时显示提示，刷新或离开前确认。
- 删除分类、删除订阅、重置配置等危险操作必须二次确认。
- 主要管理对象用卡片展示，表格只用于高密度编辑。
- 移动端避免强制横向滚动，规则详情页使用卡片替代表格。
- 加载、空态、错误和成功提示使用统一状态组件或明确的 `role="alert"` / `role="status"`。

页面重点：

- 订阅页展示订阅状态、节点数量、流量、到期、拉取失败原因和节点预览。
- 节点组页常驻预览，帮助理解引用展开和动态正则匹配结果。
- 规则分类页只管理分类和全局搜索，分类内规则在详情页编辑。
- DNS 页同时提供可视化编辑和 Raw YAML。
- 生成页展示短链接、完整链接、YAML/Script 结果、复制和下载。
- 设置页管理鉴权范围、token、订阅拉取代理、配置导入导出和重置。

## API 摘要

订阅：

- `GET /api/subscriptions`
- `POST /api/subscriptions`
- `PATCH /api/subscriptions/{id}`
- `DELETE /api/subscriptions/{id}`
- `POST /api/subscriptions/{id}/fetch`
- `GET /api/subscriptions/{id}/nodes`
- `GET /api/subscriptions/nodes/all`

节点组：

- `GET /api/node-groups`
- `POST /api/node-groups`
- `PATCH /api/node-groups/{id}`
- `DELETE /api/node-groups/{id}`
- `POST /api/node-groups/reorder`
- `POST /api/node-groups/validate`
- `GET /api/node-groups/_preview`

规则与分类：

- `GET /api/rule-categories`
- `POST /api/rule-categories`
- `PATCH /api/rule-categories/{id}`
- `DELETE /api/rule-categories/{id}`
- `POST /api/rule-categories/reorder`
- `GET /api/rules`
- `POST /api/rules`
- `PATCH /api/rules/{id}`
- `DELETE /api/rules/{id}`
- `POST /api/rules/reorder`

DNS：

- `GET /api/dns`
- `PATCH /api/dns`

生成：

- `GET /yaml`
- `GET /script`
- `POST /api/generate/yaml`
- `POST /api/generate/script`
- `GET /api/generate/yaml/download`
- `GET /api/generate/script/download`
- `GET /api/generate/settings`
- `PATCH /api/generate/settings`

安全与配置：

- `GET /api/settings/security`
- `PATCH /api/settings/security`
- `POST /api/settings/auth/check`
- `POST /api/settings/auth/login`
- `POST /api/settings/auth/logout`
- `GET /api/settings/export`
- `POST /api/settings/import`
- `POST /api/settings/reset`

配置导入导出行为：

- 导出 JSON 包含 `version`、`exported_at`、`include_subscriptions` 和 `tables` 字段。
- 导出排除 `token_hash`，不泄露鉴权凭据。
- 导入按表清空后写入，datetime 字段自动从 ISO 字符串还原。
- 导入 `security_settings` 时保留当前 `token_hash`，避免因导入丢失鉴权。
- `POST /api/settings/reset` 清空所有数据表并恢复默认单例记录。

## 规则与 GEOSITE

规则分类原则：

- 分类服务于维护理解，不照搬脚本注释。
- 平台级可由 GEOSITE 表达的规则归入 `GEOSITE/平台`。
- 金融、出行、购物、本地生活、短视频等日常直连需求归入 `国内日常服务`。
- 系统组件、厂商工具、阅读器、遥控器、健康等归入 `移动应用/系统工具`。
- 分类不要过碎，避免维护负担过高。

GEOSITE 名称必须存在于所使用的数据源中。带属性名称如 `apple@cn`、`bilibili@!cn` 校验时先取 `@` 前面的基础名称。

不存在时的处理顺序：

1. 先找合法的等价 GEOSITE。
2. 如果没有等价项，再退回 `DOMAIN-SUFFIX`。
3. 修改后运行合法性校验，确保非法数量为 0。
