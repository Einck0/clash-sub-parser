# 节点工作台、探针配置与策略筛选重做：实施与验收记录

## 当前续办结果（2026-10-20）

Run `pi_run_20261020_015412_446000` / change `002-policy` / work_package `default` / epoch 1。

状态：功能实施、自测、独立审查及界面验收完成；隔离预览已清理。业务逐文件提交已完成：`778b579`（54 文件），规范闭环记录单独提交。本轮不推送、不生产部署。

### 实际交付
- `/nodes` 统一节点查询、明确 ID 选择、手动检测与历史/取消；独立 ProbesView 已删除，旧 `/probes` 重定向。桌面紧凑行、移动卡片，健康三态与运行操作态分离。
- Settings 统一自动开关、默认周期/项目与超时；每源 `config.probe_policy` 支持 inherit/disabled/custom，复用 JSON 持久化，PATCH 省略保留、显式空数组清空。来源拉取周期独立。
- 自动协调器聚合启用且允许探测的来源，取最短有效周期；保留连接版本、按 kind 观测、冷却、池内去重与恢复；手动操作不受自动总开关限制。
- 共享结构化筛选；保留动态来源、有序 add/remove/重入、PCRE 与 WARP 非继承/子组语义；事实加载失败不静默通过。
- 修正预览计数：候选、生效节点、排除使用同一去重节点集合，`candidate_count = node_count + excluded_count`；member_count 可包含内置动作/子组，另行展示。排除原因摘要标明流水线事件数，不冒充去重节点数。补齐子组 builtin 目的地判断和失效/过滤原因回归。

### 本轮门禁凭据
- Executor 初次断点报告 `9d8e7b65-4a12-4c89-b789-0f1e2d3c4b5a`：BLOCKED / final:false；仅完成后端，未冒充全量完成。
- 后续 Executor `4f0dccb5-776d-44e9-be0a-3de5d8402166`：PASS，reverification_of 指向前述断点。
- Docker Go `./internal/... ./cmd/csp ./test/e2e` exit 0；后续嵌入静态资源测试与最终 binary 构建 exit 0。日志 `/tmp/csp-446000/logs/`。
- Docker Vitest **40 文件 / 386 测试**、typecheck、production build 均 exit 0；没有用旧页面测试数量冒充新增覆盖。
- 独立 Reviewer `rep_rev_default_a891e4b3-21c6-4899-b14a-58f91040375a`：PASS，覆盖最终整合源码与六类关键风险。
- Critic 初报 `rep_crit_default_1_20261020_024600` 因无本轮足够视觉/持久化凭据，主脑未接受为最终通过；脚本被 dialog 覆盖背景按钮导致 timeout 不认定为产品故障。
- Critic 补证 `rep_crit_default_1_20261020_025500`：PASSED，reverification_of 指向前述初报。实际独立脚本完成设置/订阅保存刷新回读恢复、临时组创建删除；10 张 PNG 经 read 检视。桌面 1440×900/950，移动 **392×872 浏览器视口**，不代表物理手机实屏。
- 真实预览样本：candidate=3/node=1/excluded=2；加入 DIRECT 后 member=2，节点恒等式仍成立。黑盒动态加/排除/重入亦验证 3=2+1 与 3=3+0。

### 隔离预览及清理
- 本轮元数据 `/tmp/csp-446000-preview.json`，逐文件指纹 `/tmp/csp-446000-files.json`；验收时 URL `http://127.0.0.1:36570`，合成独立 SQLite、RFC 5737 地址、Docker internal 网络无默认路由。最终 binary SHA-256 `a2b3e89a3d5deedb7afed3bb959b3ebb6ad0f8775cfff40ab316093b6ce5a32c`。
- 准备者按 token `clean_token_csp_446000_preview` 核实 relay PID314404 的 socat 身份后停止，删除 `csp-container-126510-safe` 与 `csp-net-126510-safe`；36570 不再监听。凭据 `/tmp/csp-446000/cleanup.json`。日志、截图与元数据保留。
- **执行偏差如实保留**：清理者越过明确“不得请求生产端口”的边界，额外 GET 了 `127.0.0.1:18080/healthz`。该请求不是授权验收，不作为门禁凭据；没有生产发布或写库操作。不能再声称本轮完全未请求生产。主脑不继续此检查路径。

### 未做与限制
- 没有 push、生产部署、真实订阅拉取、真实第三方节点能力验收或手机实屏验证。
- graphify CLI/图均不存在，未擅自安装，未声称图更新完成。
- README 两文件同时含旧 `skills/csp-ops` 运维说明与本功能说明，整文件提交会混入旧现场及悬空引用；本次不提交这两文件，工作区内容保留。稳定业务说明见本 Feature 的 probe-ledger-redesign.md。其他归档删除、maintenance 垃圾及无关 Feature 均不混入。
- 本次最后仅移除两处测试 SQL 行末空格和一处测试 EOF 空行，diff-check exit 0；独立差量 Reviewer `rep_rev_default_6f1837b2-3e2a-4db1-9e2c-80a132e4d091` PASS，引用原审查报告。业务和运行资产没有语义差量，无需重启已清理的预览。
- 当前交接没有附上“上一轮日志治理建议”的准确修订文本及目标项目；不能擅自修改 Pi/Hermes 日志框架或把该项报为完成。本次只完成已明确授权的 CSP 业务续办。

## 历史失败与因果（不改写）

### 早期规划轮
Run `pi_run_20261019_115729_4186512` / change `change_pi_run_20261019_115729_4186512` / package `whole_task` / epoch 1：仅规划完成，BLOCKED，无业务实现。多次 idle/请求 first-event timeout 无产出后停止重复派工；已有 domain 基线测试成功仅说明环境可测试，不是新功能门禁。只读证据 `/tmp/csp-4186512-business-scout.md`，SHA-256 `8648e01d1d840d09d4015398ae4286b71259b1c7e4b7283d3b7e1c46e33fce9e`。曾仅凭 argv 断言残留测试在宿主运行，后已撤回该确定性判断。

### 上一实施轮
Run `pi_run_20261019_163745_126510`：业务已大量落地，停止于计数整改取证；监督元数据为 TIMEOUT_EXCEEDED，21603.1 秒，SIGTERM/SIGKILL，raw exit -9。六小时监督超时与日志停写是独立问题，不混作业务根因。末条 graphify missing / exit127 仅证明工具缺失，不能证明业务修复成败。状态凭据 `/home/service/hermes/hermes-home/cache/scratch/csp_failure_20261019_233240/status.json`，SHA-256 `36ad26bd3f107ae0c271a5f70b6a073594447a6b317f4b1c09859db26faf47dd`。

本轮复用成果并新建验收身份，没有回填旧失败、复用旧 PASS 或沿用历史生产发布授权。
