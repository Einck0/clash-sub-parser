# Technical Plan: Specification Governance Capability

- **Feature Directory**: `specs/007-specification-governance/`
- **Capability Domain**: `governance`
- **Status**: CANDIDATE (Spec Kit Native Architecture)

## 1. Architecture Overview <a id="plan-spec-gov-arch"></a>

本能力负责项目整体规范契约治理、历史 OpenSpec 双向追溯映射、用户原话确权分级、决策议题归档及生产安全不变量校验。通过集中化迁移台账 (`docs/specification/migration-ledger.json`) 与离线确定性审计工具 (`speckit-coverage-audit`) 构建完整的治理保障防线。

## 2. Invariants & Delivery Safety <a id="plan-spec-gov-invariants"></a>

1. **用户原话最高准则**：用户明确指示与实机验证事实高于一切助手推测；
2. **零代码污染**：文档整合与规范演进阶段，除受控规范文件外，严禁修改任何生产业务代码；
3. **确定性审计门禁**：通过离线变异注入测试保障审计工具可靠性，杜绝伪绿灯；
4. **决策待决不隐瞒**：6 组业务未决问题与 3 项显式冲突保持 pending 状态，如实呈报决策。

## 3. Authoritative Plan Contracts & Constraint Annex <a id="plan-spec-gov-contracts-index"></a>

This authoritative constraint annex provides individual retention and cryptographic content integrity for all active design invariants, architectural constraints, and active design decisions across the 6 business domains:
- **Authoritative Annex Document**: [`specs/007/plan-contracts.md`](plan-contracts.md) (canonical link authority: `specs/007-specification-governance/plan-contracts.md`)
- **Active Technical Contracts**: 261 Still-Applicable Constraints (`DOC-0001` ~ `DOC-0354` subset) preserved with exact fulltext, condition, and domain anchors (`#plan-doc-0001` ~ `#plan-doc-0354`).
- **Active Architecture Decisions**: 3 Canonical Architecture Decisions (`DEC-0807` ~ `DEC-0809`) preserved with exact fulltext and anchors (`#plan-dec-0807` ~ `#plan-dec-0809`).
- **Historical Inactive Evidence**: 58 superseded constraints, 35 undetermined pending constraints, and 452 historical completed tasks retained as non-executable archived reference records in `docs/migration/historical-contract-register.md`.
