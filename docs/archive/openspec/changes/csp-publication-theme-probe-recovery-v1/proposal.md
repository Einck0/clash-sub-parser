## Why

本 Change 不能结案。最新独立审计 `VERDICT: REJECTED`：先前 reviewer 在主仓执行 `npm --prefix web run build`，`web/vite.config.ts` 的 `closeBundle` 默认 `fs.cpSync(web/dist, internal/webassets/dist, {recursive:true})`，写入用户未跟踪资产。主仓资产聚合 SHA-256 从已记录的 `bac5a0f424b263f8ad2f538368863200045978f58a0c9d8f9d89f1701866a655` 变为 `6ec421559324992339314b05071a0e44f3fc5c91ee8b3339852ea10953ddf0f6`；`/tmp/csp-critic-stage/original_dist` 有 18 个文件，审计报告原有 17 个资产与备份逐字节相同。当前目录额外包含 `index-CuRBRMN8.js`、`index-CY02MDnG.css`（均不在备份中）；它们可能是其他合法产物，**不能凭文件名直接删除或覆盖**。本次只规划取证与受控恢复，由有授权的施工者执行后再独立核验。

此前 critic 的 37 张原始 PNG 曾证明 Publications 按钮/targets 裁切、Policy/Subscriptions 溢出、Dashboard badge 刺边、Settings `Active Active`、dock 缺 Dashboard；先前 `CRITIC_FLOW_VERDICT: PASSED`、`CRITIC_VISUAL_VERDICT: FAILED`。即使后来有 reviewer 工程 PASS 和 critic FLOW/VISUAL PASSED，其发生时序也不能替代本 Change 要求的“所有特性包完成→同一最终 diff 全量门禁→独立 reviewer→同源隔离 critic”。`/tmp/csp-preview-refresh-20260924/instance.json` 声称隔离快照前后 SHA 一致及构建时主仓资产聚合 SHA 为 `6ec...`，只能证明**该快照在那次构建过程内**一致且未再次触碰主仓，不能证明现在的源代码、嵌入资产、服务进程、截图和最后审查 diff 同源。

原有六协议真实安全探针/race/负例、进程停服、移动页面可用性仍按原规格完成，不得以旧的局部测试、旧视觉 PASS 或 OpenSpec 规划完成替代实施验收。另一 Pi 治理 Change `remediate-phone-test-visual-verdict-gate` 的 reviewer PASS 不等于其 3.1–3.3 完成；用户已说明真机不可用，仅能走有证据的本地替代，本 Change 不修改其任务、不伪造物理验收。

## What Changes

- 先在授权边界内建立不可变证据：备份、当前未跟踪 `internal/webassets/dist`、构建产物、`index.html` 引用图及 Git/worktree 的路径、哈希、时间和归属。先判定额外两个文件的来源和保留需要（既有用户资产、引用链、构建历史），再提出逐文件恢复/隔离方案；禁止盲删、`reset/restore/clean`、全目录覆盖或操作生产卷。恢复后逐文件比对原有 18 文件（含 index）和任何应保留的新产物，并独立复核工作树未受损。无法确认归属时保留原状并向主脑升级证据/授权，不以 BLOCKED 勾结单。
- 固化安全构建规范：在非主仓隔离 workspace 或显式 `NO_COPY_WEBASSETS=1` 下构建，复验前后主仓资产集合+内容指纹相等；reviewer/critic 必须只读，审查禁止在主仓执行有副作用的 build。若要更改构建钩子，仅在确定性回归夹具证明保护用户产物且不破坏正常嵌入资产时由施工者实现；不为局部问题自造构建框架。
- 将当前业务实现状态拆分为“已有通过记录但最终版无证明”、“确实待实现”、“必须重跑”、“必须依序重新审查/验收”。原 tasks 1.3、2.1/2.1a–c、3.1–3.4、4.1、4.3、4.4 保持未完成；先前 4.2 的旧隔离预览不满足新同源门禁，撤销其完成标记，重新构建独立实例。保留有证据的局部 2.2/2.3 成果，但仍受最终 4.1 重跑与 reviewer/critic 的汇聚门禁约束。
- 复用仓库现有 Go 标准测试、Vue/Vitest、Playwright、Vite 环境开关、sing-box 安全客户端；每一验收记录固定最终源码指纹、源码所生资产、嵌入二进制、服务文件和本轮原图/trace 的可验证关联。禁止从污染前快照推断最新版通过。

## Capabilities

### New Capabilities

- `publication-theme-probe-recovery`: 发布恢复、移动与桌面可用视觉、主题、真实安全探针及停服生命周期、同源隔离预览与独立黑盒验收。

### Modified Capabilities

无。主规格未同步；OpenSpec 严格校验仅表明规划工件有效。

## Impact

规划与验收涉及 `web/vite.config.ts`、`web/dist`、`internal/webassets/dist`、`internal/webassets/embed.go`（如适用）、隔离工作区/预览 manifest、Go/web 测试及前端相关视图与测试。主仓 `internal/webassets/dist` 当前为未跟踪用户资产，施工/审查者必须保护原样直到来源取证和明确授权；不碰生产 17000/18080、生产卷、不部署、不提交。此次策略机只写同一 Change 的现有 OpenSpec 工件。
