# Agent Note: GitHub Spec Kit 官方版本锁定与原生规范迁移决策

- Status: proposed (Phase: Pilot Migration & Schema Harmonization)
- Date: 2026-10-09
- Author: executor
- Scope: clash-sub-parser specification architecture and multi-root Spec Kit migration

## 背景与上下文

在上一轮规范整合中，项目引入了 GitHub Spec Kit 的参考模板（位于 `.specify/templates/`），并记录了 provenance 哈希，锁定了临时参考提交 `93989802b1401c99cc7de6e4ed76572f9cb662e8`。

用户在本轮会话中明确全量授权了跨 5 大根目录的正式 Spec Kit 迁移。为保证迁移的确定性与工程复现能力，主脑与施工机对官方 Spec Kit 进行了严格选型与环境验证。

## 决策内容

1. **版本锁定与升级变更说明 (Version Pinning & Provenance Transition)**：
   - **历史存根保留**：保留 `.specify/templates/provenance.json` 中对 `93989802b1401c99cc7de6e4ed76572f9cb662e8` 的历史记录，作为阶段演进客观凭证。
   - **全新官方锁定**：正式将工具链升级并锁定至官方正式发布版本 **`v1.1.2`**（发布日期 `2026-10-07T22:27:43Z`，commit **`959e866caa3618bf3dc290d5dca33394365af9c6`**）。
   - **变更理由**：`v1.1.2` 是 upstream 的最新稳定 tag，包含完备的 `PiIntegration`（支持 `.pi/prompts/` 原生斜杠命令）、`GenericIntegration`（支持任意技能与命令目录），且已封板发布，避免依赖 main 分支浮动提交。

2. **原生规范目录演进 (`specs/` vs `openspec/specs/`)**：
   - 遵照 Spec Kit 原生标准，在项目根建立编号能力规范目录：
     - `specs/001-inventory/`
     - `specs/002-policy/`
     - `specs/003-probe/`
     - `specs/004-publication/`
     - `specs/005-auth/`
     - `specs/006-ui/`
   - 每个能力目录下包含 `spec.md`（原生规格）、`plan.md`（技术方案与已知/未知事实）、`tasks.md`（分离迁移任务与业务待办）。
   - 现存 `openspec/specs/` 与 28 个历史变更集保持只读存在，严禁擅自删除或重命名，直到用户终态审查并确认切换。

3. **台账式全量追溯与未决问题保持 (Traceability Ledger & Retained Blockers)**：
   - 建立机器可读的 `docs/specification/migration-ledger.json`，全量覆盖 259 条历史需求、463 个场景及 37 项功能需求；
   - 6 组核心业务未决问题（解包展开规则、混合顺序、空组处理等）及 3 条显式历史冲突（HR-013, HR-098, HR-100）保持 pending 状态，不在此阶段擅自变更业务代码。

4. **工具链物理隔离**：
   - 官方 CLI 安装于 `/home/service/pi/migration-tooling/venv`，严禁在项目内执行 `git init`，严禁使用虚构的 `specify analyze` CLI。
