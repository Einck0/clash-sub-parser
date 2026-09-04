# 功能等价性矩阵

本文档将受保护 SQLite 快照中的既有能力映射到控制平面实现与可重复验收。证据只引用代码、迁移夹具和自动化测试，不记录订阅地址、令牌、节点载荷或其它敏感数据。

| 能力 | Legacy 证据 | 目标模块 | 验收用例或命令 | 状态 |
| --- | --- | --- | --- | --- |
| 节点协议解析 | `app/utils/clash_parser.py` 的订阅和链接解析 | `source_refresh_reconciler.py`、`node_normalizer.py`、`node_dedup_policy.py` | `test_utils.py`、`test_node_normalizer.py`、`test_source_refresh_reconciler.py` | 已验收 |
| 订阅拉取与手工节点 | `subscriptions` 表的 `raw_nodes`、`source_nodes`、`manual_nodes` | `routers/subscriptions.py`、`services/subscription_service.py`、`models/source.py` | `test_subscriptions.py`、`test_legacy_inventory_migrator.py` | 已验收 |
| 安全 HTTP 拉取 | 现有订阅刷新请求与失败记录字段 | `services/http_fetch.py`、`services/source_refresh_reconciler.py` | `test_http_fetch.py`、`test_source_refresh_reconciler.py` | 已验收 |
| sing-box 隔离探测 | `node_probe_results` 历史记录和 `probe_config` | `services/probe/runner.py`、`services/probe/converter.py`、`routers/probe.py` | `test_probe_converter.py`、`test_tcp_probe.py`、`test_probe_routes.py` | 已验收 |
| NodeLedger 与节点来源 | `nodes`、`node_source_links`、`source_revisions` | `repositories/node_repository.py`、`routers/v2.py`、`routers/proxy_chains.py` | `test_inventory_service.py`、`test_v2_api_contracts.py`、`test_proxy_chain_v2.py` | 已验收 |
| 策略组和规则 | `node_groups`、`rule_categories`、`rules` | `services/legacy_config_importer.py`、`services/canonical_compiler.py`、`services/node_group_service.py` | `test_node_groups.py`、`test_rule_schema.py`、`test_canonical_compiler.py` | 已验收 |
| 跳板链路 | `proxy_chain_bindings`、订阅节点跳板字段 | `services/proxy_chain_service.py`、`routers/proxy_chains.py` | `test_proxy_chain_v2.py` | 已验收 |
| 历史探测与快照 | `node_probe_results`、`config_snapshots` | `models/probe_domain.py`、`services/snapshot_service.py`、`models/quarantine.py` | `test_probe_domain.py`、`test_snapshots.py`、`test_legacy_migration_preservation.py` | 已验收 |
| Canonical config graph | 策略组、规则、DNS 和生成设置记录 | `services/canonical_compiler.py`、`services/legacy_config_importer.py` | `test_canonical_compiler.py`、迁移审计的全量节点编译 | 已验收 |
| 五目标导出 | 历史 YAML 导出、节点链接与路由 | `services/compiler_target_adapters.py`、`services/generate_service.py`、`routers/generate.py` | `test_five_target_renderers.py`、迁移审计 | 已验收 |
| Clash、Mihomo、Stash 路由 | 旧 `/yaml` 与生成接口 | `app/main.py`、`routers/generate.py`、`utils/auth.py` | `test_five_target_renderers.py`、`test_v1_http_contract.py` | 已验收 |
| Shadowrocket 与 Sing-box 路由 | 节点链接和导出路由 | `app/main.py`、`routers/generate.py`、`services/compiler_target_adapters.py` | `test_five_target_renderers.py`、迁移审计 | 已验收 |
| Quick Export、订阅链接、Scheme、二维码 | 旧生成路由与订阅导出 | `routers/generate.py`、`services/generate_service.py`、`frontend/src/components/QuickExportModal.vue` | `test_five_target_renderers.py`、`frontend/tests/workbench.test.mjs` | 已验收 |
| SCRIPT 退役 | 历史 SCRIPT 路由与生成入口 | `routers/generate.py`、`app/utils/auth.py` | `test_five_target_renderers.py` 验证 SCRIPT 不在支持目标中 | 已验收 |

## 迁移审计

`python scripts/run_full_migration_audit.py` 以 SQLite 的只读 URI 打开保护快照，并通过 SQLite backup API 复制到 `tmp` 下的可写演练目标。审计要求：

- 演练前后保护源的 SHA-256 不变，且源未作为写入连接打开
- 源与目标的全部表结构和每张表行数完全相等，且每张含 `id` 的表保留同一主键序列，因此旧 ID 映射为可审计的 identity mapping，未知表或字段不会被静默丢弃
- 关键数据量不低于发布基线：4 个 sources、8 个 subscriptions、2,080 个 nodes、5,609 个 node_source_links、29 个 groups、470 个 rules、362 个历史 probe results 和 50 个 snapshots
- SQLite `foreign_key_check` 与 source revision、node link 的逻辑外键均为零孤儿记录
- 从迁移目标的完整 2,080 节点库存构建 `CanonicalGraphResolver`，并对 Clash、Mihomo、Stash、Shadowrocket、Sing-box 五种目标产生非空配置

每次成功演练产生非敏感 JSON manifest，记录源哈希、SQLite 版本、schema、表计数、关系校验和各目标输出字节数。它不写入保护副本，也不会在 manifest 中保存订阅 URL、节点数据或密钥。
