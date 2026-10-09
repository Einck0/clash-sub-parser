# CSP 持续规范维护与演进指南 (Living Spec Maintenance Guide)

- **基线版本**：HEAD `1c5a604f9d4317dd7458710e30618dba13e26724`
- **编制日期**：2026-10-09
- **适用对象**：主脑编排者（Master）、施工机（Executor）、只读审查机（Reviewer）

---

## 1. 规范资产架构与布局

本项目统一采用**持续主干能力规范（Living Capability Specifications）+ 增量变更契约（OpenSpec Changes）**的双层驱动模式：

```
clash-sub-parser/
├── openspec/
│   ├── config.yaml                     # OpenSpec 全局项目配置
│   ├── specs/                          # 持续有效的主干能力规范（Living Spec 唯一事实源）
│   │   ├── README.md                   # 规范总目录与 FR-ID 映射表
│   │   ├── inventory/spec.md           # 订阅与节点资产领域
│   │   ├── policy/spec.md              # 策略组与图解析器领域
│   │   ├── probe/spec.md               # 主动探测引擎领域
│   │   ├── publication/spec.md         # 发布与编译器领域
│   │   ├── auth/spec.md                # 鉴权与安全模型领域
│   │   └── ui/spec.md                  # 控制台界面交互领域
│   └── changes/                        # 历史与进行中的增量变更包（26 个既有 + 新增）
│       └── <change-name>/
│           ├── .openspec.yaml
│           ├── proposal.md
│           ├── design.md
│           ├── tasks.md
│           └── specs/
├── .specify/templates/                 # 官方 Spec-Kit 模板参考副本与 provenance 存根
├── docs/decisions/                     # 架构决策记录唯一事实源（借鉴 DeepSeek Agent Notes 模式）
└── docs/specification/                 # 规范治理辅助工件
    ├── source-register.md              # 来源等级注册表 (四级证据确权)
    ├── conformance-matrix.md           # 规格符合度审计矩阵 (37 项主干能力)
    ├── verification-commands.md        # 实机自测验证命令目录 (CV-001 ~ CV-041)
    ├── historical-requirement-register.md # 历史需求逐条登记与契约覆盖表 (HR-001 ~ HR-259)
    ├── clarification-questions.md      # 核心业务待决问题清单 (6 组业务未决事项)
    ├── maintenance-guide.md            # 本维护指南
    └── upstream-provenance-and-coverage.md # 历史变更与规格覆盖溯源表
```

---

## 2. 规范编写与语法硬门禁 (Syntax & Format Invariants)

所有存放在 `openspec/specs/<domain>/spec.md` 的规范文件必须严格通过 `openspec validate --specs --strict` 校验：

1. **结构要素完整性**：
   - 必须包含顶层 `# <Domain> Capability Specification` 标题；
   - 必须包含 `## Purpose` 章节，且**字符数必须不少于 50 个字符**（严格门禁，不足将导致严格校验失败）；
   - 必须包含 `## Requirements` 章节。
2. **需求与场景格式规范**：
   - 每个功能需求必须以三级标题 `### Requirement: <名称> [FR-ID]` 开头；
   - 必须使用大写的 RFC 2119 规范动词：`SHALL`、`MUST`、`SHOULD`、`MAY`；
   - 每个 Requirement **必须包含至少一个** 四级标题 `#### Scenario: <场景名称>` 块；
   - 场景内容必须包含 `- **WHEN** ...` 与 `- **THEN** ...` 结构化行为定义。
3. **Spec-Kit 模式与 FR-ID 稳定编号**：
   - 为确保长期追踪与机器分析，所有需求必须分配全局唯一的稳定编号：
     - `FR-INV-xx`：订阅与节点资产需求；
     - `FR-POL-xx`：策略组需求；
     - `FR-PROB-xx`：探测引擎需求；
     - `FR-PUB-xx`：发布导出需求；
     - `FR-AUTH-xx`：鉴权需求；
     - `FR-UI-xx`：前端界面需求。
   - 对应可测成功标准分配稳定编号：`SC-INV-xx`、`SC-POL-xx` 等。

---

## 3. 日常维护与变更工作流 (Evolution Workflow)

### 3.1 提出新特性或架构整改
1. 由主脑使用 `openspec` 官方工具在 `openspec/changes/<change-name>/` 创建增量变更。
2. 在变更包内的 `specs/<domain>/spec.md` 中编写新增（`ADDED Requirements`）或修改（`MODIFIED Requirements`）的条款。
3. 施工机在完成代码实现与本地单元测试（退出码 0）后，将变更中的实现项同步合并至主干 `openspec/specs/<domain>/spec.md`。

### 3.2 更新符合度矩阵与历史需求存根 (`conformance-matrix.md` & `historical-requirement-register.md`)
本项目采用**双层规范与库存治理体系**：
- **第一层：持续主干能力矩阵 (`conformance-matrix.md`)**：聚焦当前系统 37 项全局核心功能需求（`FR-INV/POL/PROB/PUB/AUTH/UI`）的本地实机符合度。
- **第二层：历史需求库存 (`historical-requirement-register.md`)**：完整收录 26 个既有变更集 63 份规范中提取出的 259 项需求（`HR-001` ~ `HR-259`）与 463 个场景，维护其真实可证状态（`已映射待复验` / `历史冲突待裁定` / `未验证`）。

**演进与更新铁律**：
1. 当新功能交付或缺陷修复后，维护者必须同步更新 `conformance-matrix.md`（切换符合度、登记源码精确行号、更新测试收据）以及 `historical-requirement-register.md` 对应 `HR-ID` 的条目状态（如从 `已映射待复验` 变更为已验证闭环，或将已裁定的 `历史冲突待裁定` 变更为符合新决策）。
2. **严禁清空旧 Delta**：新变更集不得通过清空或删除历史变更的规范条款来制造“100% 整合”假象；未知或未纳入当前主干的历史需求必须持续保留为 `未验证` 历史存根。
3. **成功准则 (SC) 边界铁律**：成功准则（Success Criteria, SC）仅能针对本变更实际落地并经实机验证的代码与用例宣称通过，**绝严禁宣称全部 63 份历史规范或全部历史需求已全部验证**！必须诚实披露验证边界与未验证存根规模。

### 3.3 记录重大技术决策 (`docs/decisions/`)
当遇到方案选型、否决备选方案、打破历史假设或变更持久化结构时：
- 在 `docs/decisions/` 对应的生命周期目录下新增决策记录；
- 正在规划/等待用户批准的记录置于 `proposed/` 目录；
- 只有伴随代码交付且已上线的决策记录方可移入 `implemented/`；
- 必须详尽阐述“为什么如此决定”以及“否决了哪些备选方案”。

---

## 4. 验证与门禁检查清单 (Validation Checklist)

在任何规范修改交工前，必须在终端依次执行以下命令确保零错误：

```bash
# 1. 验证所有主干 capability 规格（严格模式，退出码必须为 0）
openspec validate --specs --strict

# 2. 验证所有既有变更集与新增变更集（严格模式，退出码必须为 0）
openspec validate --changes --strict

# 3. 验证文档站内交叉链接完整性与 FR-ID 唯一性
python3 -c "
import glob, re
frs = set()
for path in glob.glob('openspec/specs/**/spec.md', recursive=True):
    for m in re.findall(r'\[(FR-[A-Z]+-\d+)\]', open(path).read()):
        if m in frs: raise SystemExit(f'Duplicate FR ID: {m}')
        frs.add(m)
print(f'Validated {len(frs)} unique FR IDs across all living specs.')
"
```

---

## 5. 当前阶段维护状态与移交标记 (Maintenance & Handoff State)

- **当前生命周期标记**：`当前STOP_AWAITING_USER_DECISIONS，未业务实施；报告已准备等待QQ交付，未知不能假已送`。
- **任务推进边界**：文档整合阶段（Phase 1 ~ Phase 5）已闭环，独立审查 Reviewer 已给出 PASS；任务 6.1 等待用户裁决后启动，严禁擅自提前勾选用户决策任务或落地业务代码。

