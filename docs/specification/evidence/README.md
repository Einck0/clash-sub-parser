# CSP 规格审计与验证证据目录 (Specification Evidence Directory)

- **基线版本**：HEAD `1c5a604f9d4317dd7458710e30618dba13e26724`
- **治理声明**：本目录收录当前规范整合阶段的终态审查判定凭据与校准的非敏感自动化测试机读收据。
- **保密与脱敏边界**：严禁在此目录存放任何敏感配置、口令或原始终端日志；仅保留结构化统计摘要与完整原始 SHA-256 哈希值；完整原始执行日志与私有证据保全于受控临时存档 `/tmp/csp-living-spec-final-evidence-20261009/`。

---

## 证据文件清单 (Evidence Manifest)

| 文件名 | 类型 | 来源角色与阶段 | 原始 / 文件 SHA-256 | 说明 |
|---|---|---|---|---|
| `final-reviewer-receipt.json` | 审查裁决收据 | 独立只读审查机 (Reviewer · Epoch 3) | `838a270e824f82a093df1987ca807fab19fa19b70aa3a59f90d05d62520e05e9` | 独立审查终态 PASS 裁决证据，确认文档架构合规与用例无 0-case |
| `test-execution-summary-receipt.json` | 自动化自测摘要收据 | 施工机 (Executor · Epoch 3) | 原始收据 SHA: `14a67e565050ab68180bf31d3ce1f1bafc58e0cb55008190217098f7de0608fb` | 41 条命令 (CV-001 ~ CV-041) 实测 178 次 / 137 个唯一用例，退出码全 0 |

---

## 补充契约与场景元数据归属声明

1. **场景元数据修正**：源场景实际为 **504** 个，目标映射为 **519** 个独立场景与 **522** 个场景链接引用（包含 15 个显式新增迁移测试场景，源场景绝非 522）；
2. **现行规划约束权威留存**：809 项历史记录中，261 项现行设计约束与 3 项架构决策权威留存于 `specs/007/plan-contracts.md`（由 `specs/007-specification-governance/plan.md` 索引并赋予权威链接）；58 项淘汰约束、35 项待审约束与 452 项历史任务留存于 `docs/migration/historical-contract-register.md`（`verified=false`，不作为新验证凭证，杜绝虚构 809 项新工作）；
3. **工程闭环独立性**：452 项历史任务与业务未决问题保持真实状态，Spec Kit 规范迁移独立于业务实现闭环。
