## Purpose

规范 CSP 节点库存重置与源修复的一次回调运维 CLI 命令（`csp reset-node-inventory`），复用生产应用与存储层服务装配，支持 dry-run 预检与原子执行模式，提供结构化审计与报告输出。

## ADDED Requirements

### Requirement: 维护 CLI 命令参数与安全预检
系统 SHALL 在 `cmd/csp` 主二进制中提供 `reset-node-inventory` 维护子命令。该命令 MUST 支持 `--db`（指定目标数据库文件路径）、`--dry-run`（只读预检并输出将要变更的计数）、`--apply`（真实执行写入操作）、`--confirm-backup`（显式备份确认保护）、`--backup-file`（验证有效 SQLite 备份快照路径）以及 `--restore-source-tokens`（可选执行 7li/魔戒 URL 修复，且指定冷归档路径 `--archive-db`）。若未指定 `--apply` 亦未指定 `--dry-run`，命令 MUST 报错退出。在 `--apply` 模式下，若未提供 `--confirm-backup` 且未提供有效 `--backup-file`，命令 MUST 拒绝执行。在 `--dry-run` 与 `--apply` 执行前，系统 MUST 执行前置外键完整性预检，发现既有脏外键时提前阻断。

#### Scenario: Dry-run 模式只读预检不修改数据
- **WHEN** 执行 `csp reset-node-inventory --db <path> --dry-run`
- **THEN** 系统以只读方式检查当前数据库中的节点数、衍生表计数与外键关联，输出预检报告，数据库文件哈希与内容保持不变

#### Scenario: Apply 模式未提供备份确认时拒绝执行
- **WHEN** 执行 `csp reset-node-inventory --db <path> --apply` 且无 `--confirm-backup` 或 `--backup-file`
- **THEN** 命令退出码为 1，向 stderr 报告需要备份确认

#### Scenario: Apply 模式原子执行并输出报告
- **WHEN** 执行 `csp reset-node-inventory --db <path> --apply --confirm-backup`
- **THEN** 系统在单一原子事务中执行重置闭包与外键检查，提交后输出包含重置前后的各表计数、外键校验状态（0 违规）的结构化 JSON 报告

### Requirement: 复用生产 Service Wiring 与隔离验证
运维 CLI 命令 MUST 内部复用项目现有的 `internal/repository/sqlite` 与 `internal/application/inventory` 的服务与仓库装配，严禁新建独立的外部控制面、旁路 HTTP 桥接或绕过现有数据校验的脆弱代码。系统 SHALL 支持在通过 SQLite 官方只读 `.backup` 生成的临时隔离副本上完成全链路演练与测试。

#### Scenario: 隔离副本演练不影响生产数据库
- **WHEN** 针对隔离副本路径执行维护命令
- **THEN** 生产数据库保持正常服务不受干扰，隔离副本完成全部重置与自测验证
