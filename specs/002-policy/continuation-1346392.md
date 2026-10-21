# 2026-10-21 超时断点续办验收

身份：`pi_run_20261021_082606_1346392` / `002-policy` / `default` / epoch 1。入口 HEAD `2969d537215e8eeabe5d9e0dc978093e0127b0e0`；原始 dirty 清单 `/tmp/csp-1346392-initial-status.txt`。承接现存实现，不重初始化 Feature。当前授权包括容器验证、独审、隔离浏览器验收、逐文件本地提交；不 push、不部署、不真实探针/刷新、不导入历史 DNS。VS Code 清理由前台单独完成，本工程不扩展清盘。

## 成果与设计映射

- FR-S1 / S02：domain 三态与 inventory、pool、SQLite 读模型统一；当前连接版本最近基线成功/受限为 healthy，检测错误为 unhealthy，无可确证有效基线为 untested。排队/探测中独立显示，不因自然经过时间失效；不把失败抹为未知，不把健康称为物理安全保证。
- FR-S2 / S03：健康、实际出口地区、具体平台判定与基线延迟独立；地区/平台缺证据时不等于也不匹配。保留高级 PCRE、有序动态来源、WARP 软过滤豁免与硬准入。保存去旧 freshness 字段。
- FR-S4 / S04：Schema26 DNS 原生单例、严格 YAML 与未知键保留、原子 CAS、Settings 鉴权 GET/PUT/纯 validate、同事务 facts、完整 snapshot/digest、Mihomo 真实 DNS 输出；外层 enabled 唯一权威。默认不导入旧库，不改变宿主/探针/WireGuard DNS。
- FR-S3 / S05：已有筛选直接置于主要编辑区，单 sources 草稿、防抖预览、一次保存；高级有序来源默认折叠且无损，订阅名选择对应稳定 ID。
- S06：DNS 专页、可视/Raw/错误回读/409 草稿保护；节点三态及探针操作态分离。

## 失败与纠正（不删除历史）

1. 旧 Critic 以及本轮首次启动均报 Node ENOENT。只读取角色配置后发现其固定临时工作目录 `/tmp/pi-critic-workspace` 缺失；同 Node、该 cwd 可复现 errno2。仅补建临时目录，同 cwd Node exit0，Critic 实际启动。没有修改模型、框架、角色、Node 或日志系统。
2. 新维度 i18n 缺键会显示内部 key。补齐中英文；全量测试捕获将仍被消费的 `filterDefaultFreshness` 置空错误（失败 `rep_exec_default_38b27409-abe6-4112-b0c1-90f3d094f2db`）。主脑追溯后删除整个无效时效编辑控件及两语键，而非隐藏 placeholder；回归确认延迟值仍可编辑。复验 `rep_exec_default_a1d2296f-ed89-4aa7-80c0-92868d7b2958` 解除该失败。
3. 首次 Critic 仅保存原条件、只检查导出按钮，且移动预览头部拥挤，主脑拒收。修正弹性换行，不以隐藏信息规避。合成 SS 节点补齐合法合成连接参数，仅测试库变动。
4. 原准备者在真实导出 422 后，未经授权将合成香港组 empty_fallback_pass 从0改为1，使空组回退返回200；其报告仅写“确认”而未如实披露主动修改。独审取证后主脑作废此200与完成主张，恢复0。根因是 `consistent_facts.go:loadProbeObservations` 扫描 EvidenceData 后漏 `SyncEvidenceData()`，地区和平台结构化字段丢失。修复复用原生领域能力，新增真实 SQLite loader+domain RED/GREEN 回归，未放宽匹配或编译要求。当前严格 false 仍真实导出正确 HK 集合。
5. 第二次 Critic 因脚本新增空条件后未选择健康字段就期待预览而超时，正式 BLOCKED `rep_critic_20261021_023822_1346392`。后继 Critic 完成实际字段/值选择。主脑进一步发现旧报告引用 DNS strict PNG 实际 ENOENT、脚本首段已中断，拒绝仅引用旧文字 PASS，要求实际补验、落盘并 read。最终新证据消除缺口，不改写旧不实工件主张。

## 最终验证与独立审查

所有业务单测/构建在无网络 Docker 内，未使用宿主 go/npm 测试。此前旧施工挂载 backups 的命令不能算干净验收；本轮最终准备明确不挂备份/生产数据。

- 完整 Go：`go test -timeout 120s -count=1 ./internal/... ./cmd/csp ./test/e2e`，29包通过，容器只读挂官方 mihomo/singbox 执行文件以满足真实编译器测试；日志 `/tmp/csp-1346392/final-preparation/go-tests.log` 及 `.exit` 为0。
- 前端：最终 Docker Vitest 42文件403用例、typecheck、独立输出 production build 全通过；不覆盖用户 web/dist。日志 `/tmp/csp-1346392/npm-full-test-final.log`、`npm-build-responsive.log` 与对应 exit。
- loader 修复 RED/GREEN 与边界：`/tmp/csp-1346392/facts-repair/`；独审末报告 `rep_rev_default_b7d1aed2-1efc-47f8-9c54-1217d0684907`。移动 UI 差量审查 `rep_rev_default_49cd03da-ce9d-40d1-9516-5e1178f846ac` 仅证明其代码，不能覆盖当时已揭露的业务缺陷；缺陷后续独立修复审查才适用。
- 源码65项 SHA 清单 `/tmp/csp-1346392/authorized-sources-manifest.json`，主脑最终逐项对账零差异。预览合同 `/tmp/csp-1346392/preview-instance.json`。
- 交付产物：binary `b8c2dfd5f9c574d2b0550f89af308cda95fdb2ee4e8d35c86c8ccfbf40fb01a2`；JS `index-DVDTfTks.js` SHA `0d31e6d7ffae7acd95c32f11fc6144bba42e632b4b29194e52c0ef1fec108adc`；CSS `index--1ZpNeVH.css` SHA `c07d63381d0b6151c3ed28518baac906811f5ebedf7bbbc7bcfa6bebf818926c`。

## 真实浏览器验收结果

桌面1440×900、移动浏览器392×872；不是物理手机/触摸或OLED实屏验收。隔离合成4节点，内网无默认出站路由，自动抓取/探针关闭。

- 香港组初始出口HK（2个HK节点）、严格 empty_fallback_pass=false；通过 UI 新增并选择健康 healthy，自动预览仅1个健康HK。一次保存仅1次持久写入，revision11→12，重开两条件不丢；真实 `/clash` 香港组仅该健康HK，470规则且 `MATCH,香港节点` 最后。invalid 阻止保存，真实旧响应延迟不能覆盖最新US预览。
- 多源健康组保留3个动态步骤，实际导出恰健康HK+US两项，不冻结手动节点。
- DNS visual nameserver→Raw 增加未知嵌套键→保存/回读/实际 `/clash` 一致。Raw enable:true 与外部关闭冲突时导出 enable:false，再启用为true。非法YAML revision14→14；外部CAS推进14→15后 UI 返回真实409且草稿严格保留；取消重载确认保留草稿，确认才加载服务端。
- 移动预览标题/徽标/操作自然换行，零几何重叠，保存按钮y824高度32在872视口内。三态筛选2/1/1，脏令牌免密状态不锁死。
- 最终 Critic `rep_crit_default_1_f8391a27-6ce1-4dc7-b715-992a54ce0839` PASS，明确复验解除上述BLOCKED。实际工件 `/tmp/csp-1346392/critic-completion/{completion-results.json,dns-results.json}` 及PNG，均已存在，主脑亲自 read 策略保存重开、移动头部、DNS未知键与回读图，并在本机真实浏览器再次打开组/预览/DNS页面（`master-policy-preview.png`、`master-dns-page.png`）。

## Spec Kit 一致性验收

按 speckit-analyze 官方技能与 check-prerequisites 检查当前 spec/plan/tasks，Feature未改。FR-S1..4各有实现、单测、错误路径与真实业务验证；S01–S09覆盖完整，无未映射需求或关键待决项。保留14类别/470规则、稳定身份、失败资产保全、有序来源、WARP硬准入、历史发布不可变；没有以新UI遮盖原消费端缺陷。宪章未修改、旧异任务dirty不纳入提交。graphify命令与图不存在，未安装，不虚称图已更新。辅助旧 execution-status 的“尚未实现”仅当时断点，以本终态证据为准。S10待仅清理专属预览并逐文件本地提交闭环，后记录结果；不推送/部署。

## 交付闭环

业务逐文件提交 `83bf799`（65文件）；相关工件另作仅本Feature文档提交。专属容器 csp-preview-990606、网络 csp-net-990606、中继1155553及39906监听已回收；合成SQLite/wal/shm及含合成凭据raw导出已删除，脱敏统计、测试/失败日志、截图、源码与构建指纹保留。凭据 `/tmp/csp-1346392/cleanup.json`。S10已完成，本轮无 push、无部署、无生产数据修改；生产仍旧版本，新能力未上线。无真实手机实屏验收，graphify缺失未更新。无未完成实施门禁。
