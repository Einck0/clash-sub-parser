## Purpose

规范基于冷归档证据对受历史 `SanitizeSubscriptionURL` 参数剥离缺陷影响的订阅源执行精准 URL 恢复的规范，确保 `7li` 与 `魔戒` 两个订阅源恢复包含鉴权 Token 的合法完整 URL，更新修订版本并记录脱敏审计事件。

## ADDED Requirements

### Requirement: 基于冷归档稳定身份证据精准恢复 Token URL
系统 SHALL 仅针对已证实因历史缺陷导致持久化裸 URL 的订阅源（`7li` 与 `魔戒`），基于冷归档数据库（`/home/service/backups/csp-legacy-cold-archive-20260919.db`）中的稳定导入 ID 与完整归档身份进行精确匹配与恢复。系统 MUST 严格校验证据一致性，严禁依据源名称进行模糊猜测。系统 MUST 仅更新受影响的 2 个订阅源的 `source_url_secret_ref` 字段，严禁篡改其他 7 个订阅源的配置、URL 或启停状态（保持 4 enabled / 5 disabled）。

#### Scenario: 成功恢复 7li 与魔戒完整 URL
- **WHEN** 匹配冷归档中的稳定导入 ID 与无参数基准 URL 成功
- **THEN** 系统将目标数据库中 7li 与魔戒的 source_url_secret_ref 恢复为包含有效 token 的完整 URL，并递增 subscription revision

#### Scenario: 其余订阅源状态与配置绝对不被修改
- **WHEN** 执行源 Token 恢复操作
- **THEN** 除 7li 与魔戒外的其他 7 个订阅源（包括 5 个 disabled 源及 Dogegg、einck-qzz 等）配置及 enabled 状态保持不变

#### Scenario: 重复执行具备幂等性不产生虚假版本累增
- **WHEN** 针对已经成功恢复或已包含有效 Token 的数据库再次执行恢复操作
- **THEN** 系统跳过更新，不递增订阅 revision，不写入重复的 audit_events

### Requirement: 恢复操作日志与对外报告敏感凭据完全脱敏
系统 SHALL 在恢复订阅源完整 URL 时严格执行敏感信息脱敏与审计记录。系统 MUST 将修复事实记录为审计事件（AuditEvent），说明源 URL 已恢复。系统 MUST 严禁在控制台标准输出、日志输出、审计记录摘要或对外 JSON 报告中打印包含 raw token、password 或完整敏感参数的明文 URL，且对 URL 路径中的秘密 Token 片段（如 /sub/<secret>/...）亦必须脱敏掩码，仅输出布尔标记（`restored: true`）与脱敏后的基准地址。

#### Scenario: 恢复执行输出不包含明文 Token
- **WHEN** 执行源 Token 修复操作并输出执行结果
- **THEN** 输出中仅包含脱敏后的地址与布尔状态，未出现包含 ?token= 敏感参数或敏感路径片段的明文字符串
