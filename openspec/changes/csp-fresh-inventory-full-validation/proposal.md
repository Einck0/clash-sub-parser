## Why

本 Change 已完成库存清空、两源 Token 修复、协议身份扩展、维护 CLI、规则管理及历史生产切换，现有 A/B 实现必须保留。2026-10-08 最新只读预检证明新比较尚未公平覆盖 ALL90、stage-first 执行器及跨启动预算未闭环、最后源码没有全量成功门禁；本次按用户已接受的分阶段实验修订同一 Change，不重做历史操作、不以标题计数相等替代缺陷解释。

## What Changes

- **保留特性 A 与既有 B**：显式空组 PASS 默认严格、非 DIRECT、真实节点计数不变；PCRE 兼容与规则校验/UI 语义保持原合同。只补最终版本回归和必要整改，不 blanket-enable 生产用户 group flag，不 reset/import、重复修 Token 或删除规则。
- **完成一个 stage-first 比较整改包**：`tools/probe-comparison/run.py` 先执行 CSP 与独立标记的 same-engine Mihomo v1.19.32 comparator 的 ALL90 baseline AB/BA 四侧，然后在四侧 baseline 交集上执行平台阶段。共同资格之外的 discordant 节点保留逐节点诊断，不把未执行、取消或短时失败改写为永久 dead。
- **native 与归一比较分离**：真实 upstream 固定 commit `3c320fd58aff5235e16218c050ec5b8ce587e233`，native v1.19.31 `Check` 方法保持真实原生配置与默认差异，测速关闭。same-engine comparator 的版本、modfile/传递依赖 diff 与 build provenance 单列。默认只跑主比较四侧；仅在首次请求前冻结且不损害主覆盖的同一总盘子中允许额外两侧 native baseline-only 控制，不强制六侧。
- **新批次无测速且硬限额**：alive8/media2/speed0；没有 speed URL 或测速请求。实际 baseline runner 每 node×side 响应 body 上限 64 KiB，四侧最坏预留 22.5 MiB；所有 launches 共用原子 reservation/refund ledger、同一起点与截止，整批应用响应 body ≤128 MiB、整批含清理 ≤30 分钟。平台剩余额度四侧公平相等、逐节点参与有界；不保证原生全部 media 能在上限内完成，准确列出未尝试阶段，不漏算非 collector 平台调用，不增加重试、512 MiB 或自动扩额。
- **真实终态顺序与现存授权**：离线全部代码自测（含 native/same-engine/reference fixtures）→独立最终 Reviewer→已批准的实网 staged batch→最终源码绑定的刷新隔离预览与 Critic→正确 COPY flag 同步嵌入资产→仅授权 CSP Git commit→一致性热备/rollback 演练/app-only runbook 部署与健康验证。7200 秒观察独立于 rollout 完成，不能以健康轮询冒充。发布依据解释清楚的缺陷与适用门禁，不要求 headline 数字一致。

## Capabilities

### Modified Capabilities
- `policy/rules-validation`：保留显式空组 PASS、过滤保真、统一校验输入及 UI 刷新/编辑合同；新 UI 改动仍须最终独立审查和成品验收。
- `maintenance/reset-node-inventory-cli`：补齐 ALL90 优先、四侧 same-engine/native 分离、无测速、整批跨启动预算/时间、固定 manifest 和逐节点阶段归因。
- `inventory/clean-slate-node-reset`、`subscription/source-token-repair`：保留已完成合同，不新增重复执行授权。

## Impact

本次规划只修改官方 CLI 解析出的同一 Change 的既有 proposal/design/tasks 和 maintenance delta spec，不写业务、测试、生产配置，不派工、不执行实验、构建、部署、提交或 QQ 通知。后续整改写集合及拟定精确 CLI 接口见 design/tasks；已有 A/B 工作树、失败历史与私有证据均保留。

最新证据来源为指定预检 session `2026-10-08T05-50-14-467Z_879111db-a2c3b028-3b1a3e40-48b9.jsonl` 的最终报告 `rep_exec_csp_continuation_preflight_1`。其报告当前 run 为 `pi_run_20261008_134941_3738865`；本规划会话环境无不同的 run_id，不杜撰新 run。READY 只代表此修订计划完整、严格校验通过，不代表施工或发布门禁 PASS，也不重复索取已存在的实施/发布授权。
