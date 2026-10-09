# CSP 持续主干能力规范总览 (Living Capability Specifications Index)

- **基线版本**：HEAD `1c5a604f9d4317dd7458710e30618dba13e26724`
- **最后更新**：2026-10-09
- **治理状态**：ACTIVE LIVING SPECIFICATION（唯一形式化持续合同事实源）

本项目吸纳 GitHub Spec-Kit 架构范式（User Scenarios 优先级场景、稳定功能需求 FR-ID、可度量成功标准 SC-ID），并严格适配 repo-local OpenSpec CLI 规范（`openspec validate --specs --strict` 零报错），将系统划分为 6 大领域主干能力规范。

> **规范继承与治理契约声明**：
> 1. **双层规范契约**：本主干规范集合（Living Specs）定义当前持续演进的核心基线，与既有 26 个变更集（63 份分项规范）共同构成完整契约体系。未在主干 37 项稳定功能需求中明确合并的细分条款，继续作为『尚未消解的历史需求存根』保留在各自变更集中，严禁单方面宣称历史需求已 100% 自动合并或全量达标；
> 2. **工具链选型清晰声明**：本项目采纳 Spec-Kit 官方模板结构（参考副本位于 `.specify/templates/`）与 LivingSpec 规范理念，但多智能体执行、门禁验证与变更管理**严格保持既有 repo-local `openspec` CLI 框架**，未在运行环境中安装外部 SpecKit 运行时 CLI，亦未修改 Pi/Hermes 治理底座；若需切换全球执行框架必须另行请示用户单独批准。

---

## 领域能力规范索引表

| 领域目录 | 规范文件路径 | 核心职责与范围 | 稳定功能需求编号 (FR-IDs) | 状态 |
|---|---|---|---|---|
| **Inventory** | [`inventory/spec.md`](inventory/spec.md) | 受控订阅拉取、节点传输身份规范化、CoW 连接覆写、多来源关联台账与作用域隔离 | `FR-INV-01` ~ `FR-INV-07` | **ACTIVE** |
| **Policy** | [`policy/spec.md`](policy/spec.md) | 策略组图解析器、双模式成员支持（直接组引用 vs 解包叶节点）、交错排序、空组 PASS 兜底与动态全池解析 | `FR-POL-01` ~ `FR-POL-07` | **ACTIVE**<br>*(含 [NEEDS CLARIFICATION] 待决项)* |
| **Probe** | [`probe/spec.md`](probe/spec.md) | 原生 Mihomo 探针执行引擎、多层级调度周期解耦、单节点优先插队、证据分类与健康基线判定 | `FR-PROB-01` ~ `FR-PROB-07` | **ACTIVE** |
| **Publication** | [`publication/spec.md`](publication/spec.md) | 原生 Mihomo 完整分流拓扑导出、Sing-box/Surge/QX 纯节点导出、免密公开导出与统一令牌复用 | `FR-PUB-01` ~ `FR-PUB-06` | **ACTIVE** |
| **Auth** | [`auth/spec.md`](auth/spec.md) | 管理端与导出链接解耦独立开关、统一系统主令牌共享、明文敏感配置模型与控制面会话凭据 | `FR-AUTH-01` ~ `FR-AUTH-04` | **ACTIVE** |
| **UI** | [`ui/spec.md`](ui/spec.md) | 控制台紧凑弹性布局、真实视口可用性、健康状态筛选与分页、抽屉式编辑工作台与双语国际化 | `FR-UI-01` ~ `FR-UI-06` | **ACTIVE** |

---

## 交叉索引与辅助工件

- 来源等级判定表：`docs/specification/source-register.md`
- 实机代码符合度矩阵：`docs/specification/conformance-matrix.md`
- 业务待决问题清单：`docs/specification/clarification-questions.md`
- 规范维护与扩展指南：`docs/specification/maintenance-guide.md`
- 历史变更 26 Changes 覆盖溯源：`docs/specification/upstream-provenance-and-coverage.md`
- 官方参考模板存根：`.specify/templates/`
- 架构决策记录：`docs/decisions/proposed/architecture/2026-10-09-living-spec-consolidation.zh.md`
