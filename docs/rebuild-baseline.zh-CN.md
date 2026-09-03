# CSP 遗留系统基线

- 采集时间：2026-09-03T08:13:28+08:00
- 范围：`rebuild-subscription-domain-architecture` 重构开始前的只读取证
- 基线提交：`8aeed3cddb0335e4858186607e4db5e0458acdae`
- 分支：`dev`，跟踪 `origin/dev`
- 数据保护：本文只记录结构、数量和状态；不包含订阅 URL、节点参数、认证 token、代理地址、生成配置或数据库内容

## 工作树

重构开始时，工作树已有 21 个已修改文件、12 个未跟踪的 probe 实现或测试文件，以及两组已归档但尚未跟踪的 node-probe OpenSpec 工件。它们属于用户既有 WIP，必须在后续设计/实现中逐项复核，不能被当作已验收基线，也不得被覆盖或清理。

当前变更中规模最大的已有改动是 `frontend/src/views/NodeLedger.vue`，其 diff 为 2146 行新增、289 行删除。当前重构 OpenSpec 工件也处于未跟踪状态，直至独立审查和提交流程完成。

## 运行状态

- Compose 服务：单个 `app` 服务，容器名 `clash-sub-parser`
- 容器状态：运行 14 小时且 Docker healthcheck 为 healthy
- 本机端口绑定：`127.0.0.1:18080` 与 `127.0.0.1:17000`
- 只读 HTTP 检查：`/ready` 返回 `{"status":"ready"}`，`/health` 返回 `{"status":"ok"}`
- 本次未重启容器、未运行数据库写操作、未读取业务数据内容

## 数据库与迁移状态

Alembic 当前仅声明 head `8f5c2d1a7b90`。运行数据库没有 `alembic_version` 表，但已经存在 11 张业务表（config_snapshots / dns_config / generate_config / node_groups / node_probe_results / probe_config / proxy_chain_bindings / rule_categories / rules / security_settings / subscriptions）

精确 schema（列名、类型、可空性、默认值、主键）和索引的完整元数据记录在 `backend/tests/fixtures/legacy-schema-fingerprint-v1.json`，其 SHA-256 指纹为：

```
cee4078941a1471bc3f908dc7e6c018d4c10e6f68c88d03435e03ac617e4c5b3
```

指纹由 manifest 的规范化 JSON（`sort_keys=True, indent=None`）的 SHA-256 生成，`backend/tests/test_schema_fingerprint.py` 对此进行确定性校验

实机与当前 ORM 的差异：
- `subscriptions` 表含 ORM 已移除的 `proxy_chain`（JSON）和 `node_proxy_chains`（JSON）两列
- ORM 声明的 `ix_rules_category`、`ix_rules_id`、`ix_subscriptions_enabled`、`ix_subscriptions_is_primary` 四个索引在实机中不存在

当前启动代码仍执行 ORM `create_all`，随后调用 `_bootstrap_schema` 直接补列。该行为与仅含两条 revision 的 Alembic 历史不一致，是第一阶段必须消除的 schema authority 冲突

## 已确认的迁移前不变量

- 不得对当前 SQLite 卷执行 reset、restore、导入或实验性 upgrade
- 后续所有数据迁移必须先针对其副本运行 preflight、备份校验、数据计数校验和输出 parity
- 任何新表、列、索引、约束只能由明确 Alembic revision 引入；应用启动不得修复 schema
- 订阅、节点、认证和代理链的任何真实值不得进入 fixture、日志、测试快照或本文档
- `/yaml`、`/script` 与现有管理 API 是兼容面，迁移必须通过 fixture 与 shadow compiler 验证后才可切流

## 基线核验命令

在只读上下文中，以下命令已成功执行：

```text
PYTHONDONTWRITEBYTECODE=1 python -m alembic -c alembic.ini heads

docker compose ps --all
curl --noproxy '*' "$APP_BASE_URL/ready"
curl --noproxy '*' "$APP_BASE_URL/health"
```

数据库结构与行数通过容器内 SQLite 只读连接采集。本文在写入前按路径和关键词复核，不包含 URL、token、密码、凭据、私钥或完整生成配置。
