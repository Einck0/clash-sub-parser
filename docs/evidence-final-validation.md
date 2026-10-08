# CSP 全新库存重置、策略修复与受控上线实证（历史与本次分开）

## 2026-10-08 真实 AB/BA 与 B 离线整改补充：PARTIAL

以下不覆盖历史部署证据。当前源码HEAD `8b26f079e852d707c4cc2f74bbac6fb202c7ada4` 加原有 A+B 工作树；reference 为 commit `3c320fd58aff5235e16218c050ec5b8ce587e233` 干净git archive，真实 Check/CreateClient/platform，缓存untracked测试未使用。Mihomo CSP1.19.32、primary upstream1.19.31、历史CLI1.19.30，未执行same-core网络control。

只读 SQLite backup 快照 integrity ok/FK0；90 active节点全部属于enabled-current scope，未reset/import/source reload。AB/BA为CSP→upstream→upstream→CSP，alive8/media2/speed1；两侧speed请求固定1MiB，15s客户端超时、3s测速，每侧32MiB、共享总128MiB/15min deadline，无追加网络。

| 侧 | body bytes | 秒 | baseline executed/90 | baseline available |
|---|---:|---:|---:|---:|
| CSP A1 |33554432|315.728|90|67|
| upstream B1 |33554432|133.136|34|27|
| upstream B2 |33554432|139.386|34|27|
| CSP A2 |6874430|272.323|90|66|

累计 **107537726 bytes / 102.56MiB**；子进程860.573s，总900.027s，1620 node×stage记录。前三侧预算耗尽，第四侧共同deadline到期；CSP speed未执行，upstream speed分别16/17执行、6/7available。不是full enabled-stage completion，更不是公平parity。双方失败不能解释为永久死亡。body不含headers/TLS/NIC；41节点原代理配置skip-cert-verify，目标站点TLS与代理层TLS须分开披露。

私有证据句柄：`/tmp/csp-real-comparison-20261008T045932Z/`（0700、根层文件0600）。`SUMMARY.zh-CN.md`含逐节点脱敏失败ID/名称；`attribution.redacted.csv`完整分母1620条；private JSON保留status/target digest/平台原始字段；库存、mapping、SQLite、stderr不公开。摘要SHA256 `adcc2a3fc4cf7f602a916129f7c5d68fcc4221b6d69a9ad16a2e825f8d014927`。

根因：native pipeline speed与media/alive重叠，早期吞吐吞掉预算；缺失记录末态budget/deadline遮蔽dependency；reference粗错误/只collector导出平台字段不足；引擎与HTTP2/redirect/算法差异未控制。离线correction新增fixed stage-first计划、禁用network入口、CSP dependency优先及typed errors、reference独立stage allocation/原方法baseline-speed错误、独立samecore.mod1.19.32控制。**共同eligibility执行器、reference完整平台错误与队列状态、精确samecoreengine标识仍未闭环，不ready-for-approved-run。**

整改证据 `/tmp/csp-b-correction-plUdtk/`：Python黄金分配exit0；Go build ./... exit0；targeted platform+application/probe race exit0（首轮旧断言失败exit1已修）；真实reference build/fixture race exit0；samecore离线build初次要求mod更新exit1，独立modfile -mod=mod后exit0。全量go test ./... exit1为共享cache e2e linker对象缺失；go test -race ./...超时、没有exit0，不宣称全量race通过。旧12.5/13.4已撤销，14.2/9.2仍未勾选。代码变更触发Reviewer重新审查；未来512MiB/30min仅REQUESTED未授权，本轮无新增网络/生产写入/部署/提交。


## 1. 部署与停写备份确权 (Backup & Deployment Evidence)
- **Git Commit (HEAD)**: `414a6b7` (`fix(csp): rebuild inventory safely and validate routing policies`)
- **运行容器**: `clash-sub-parser` (`a430e97a76d6`)
- **发布镜像**: `clash-sub-parser-app:latest` / `clash-sub-parser-app:v1-release-20261008-414a6b7` (`sha256:c77c1f6c321e...`)
- **容器内二进制 SHA-256**: `02b2c9f28d6ae342cf7205ba7b5596dea7210cadbcaedfd9f1984882d2f386d8` (剥离符号 Alpine 静态构建，内嵌 `index-CrGDGkKG.js` / `index-CbE9lBWs.css`)
- **宿主机维护 CLI SHA-256**: `5d978a0c94c6e78063a035a12990ff18fa4c774b0fa3bd5ae8e0d14dcc4e7960` (Debian 宿主编译)
- **停写一致性备份**: `/home/service/backups/csp-v1-backup-20261008_post_stop_offline_consistent.db`
  - 权限: `0600`
  - 真实 SHA-256: `b339439fd76786316873f0f9d85dd85977f0a33c9cbabd0871062b9e4dd35e8d`
  - 库状态核验: `integrity_check: ok`, `foreign_key_check: 0`
  - 数据基线: 包含停写前原始数据：1014 节点 (31 活跃)、31 `node_sources`、998 `node_source_history`、163 规则、0 组过滤器，确认保留完整旧库快照
- **已实测回滚镜像**: `clash-sub-parser-app:rollback-verified-84ed3` (`84ed3ba0baa5`)
  - 演练验证: `--network none` 挂载备份副本启动实测 `/healthz` 响应 `ok`
- **停机维护实测窗口**: **11 分 45 秒** (北京时间 2026-10-08 03:10:40 至 03:22:25 / UTC 2026-10-07 19:10:40 至 19:22:25)

## 2. 离线原子维护执行明细
- **reset-node-inventory**: 旧 1014 节点、1014 connection heads/versions、31 sources、998 history、10900 runs、14877 observations 全部清空为 0；从冷备恢复 7li 与魔戒带 token 的有效 URL。
- **restore-group-filters**: 从冷备归档通过 Canonical ID 守卫恢复 15 个丢失正则至 `group_node_filters`，保留 13 个静态边缘组与 1 个不支持语法组 (`便宜`，冷备原正则含 PCRE 负向断言 `(?![\\d.])`)；二次执行 15 组全跳过，验证严格幂等。
- **maintain-inventory delete-rule**: 精准删除规则 `01a0b9af-c118-72f9-9949-d94b49fa6ec2` (`PROCESS-NAME,tr.com.kliq.app`)，规则总数 163 扣减为 162，保留「其他」组，生成新配置版本 `01a117ca-a695-7ceb-91f5-0977d0a37317`。

## 3. 全新拉取与全量 6 阶段探针结果
- **4 启用源全新拉取**: 7li 返回空内容如实记录 failed（不伪造旧节点）；魔戒解析 44 有效 44；Dogegg 解析 18 有效 18 (含 3 条公告/信息节点)；einck-qzz 解析 33 有效 33；总去重入库 **90** 个全新活跃节点 (`active = 1`)。
- **全量有界探针 (maintain-inventory probe)**:
  - 调度总量: 90 节点全覆盖，540 项任务
  - 状态统计: 385 成功，50 失败，105 因依赖跳过 (baseline 未通)
  - 分阶段表现: alive 69 可用 21 失败；media 68 可用 1 错误；AI 58 可用 5 受限 5 未知 1 错误；risk 67 可用 2 错误；geo 63 可用 6 错误；speed 60 可用 (含 12 达到 budget 上限) 8 错误 1 未知
  - 耗时与流量: 5 分 54 秒，总下载量 54,592,622 字节 (52.06 MiB)，严格控制在 128 MiB 上限内
- **上游对照说明**: 21 个 baseline 失败节点通过独立驱动 Mihomo 适配层单测进行代码级原因确认，非复用第三方外部 subs-check 完整逻辑；成对公平性能评测待后续独立基准补齐。

## 4. 发布策略门禁与剩余阻断边界 (Publication Blocker)
- **门禁状态**: `maintain-inventory preview --omit-unavailable-optional-groups` 判定 `BLOCKED` (退出码 1)。
- **阻断诊断**: `proxy group "低延迟" has no proxies (required_nonempty)`。
- **原因剖析**:
  1. 「低延迟」策略组正则为 `EPL|epl`，在 90 个真实新节点中匹配数为 0；
  2. 该组被 fallback 策略组「自动切换」直接引用；作为必需 fallback 备选分支，系统安全策略禁止修剪；
  3. 保真原则：不对用户分流规则进行猜测性篡改或强行转为 DIRECT，因此阻止生成不可靠的 publication，保护生产订阅消费安全。

## 5. 前台交互路由与操作凭证建议
前台基于 `vue-router` Hash 模式构建，生产前台操作路径如下：
- **分流规则与校验**: `http://127.0.0.1:18080/#/policy` (页面中点击「分流规则」Tab，提供「手动校验」按钮与实时告警提示)
- **订阅管理**: `http://127.0.0.1:18080/#/subscriptions` (查看已启用 4 源状态及 7li 空内容告警)
- **节点视图**: `http://127.0.0.1:18080/#/nodes` (查看全新 90 节点与 3 条公告标签)
- **探针观测**: `http://127.0.0.1:18080/#/probes` (查看 6 阶段探针覆盖与延迟详情)
- **发布管理**: `http://127.0.0.1:18080/#/publications` (查看发布阻断状态及预览诊断)

## 6. 待后续跟进项 (Pending Boundaries)
1. **成对基准对照评测**: 完成与上游基准的成对网络效率与 RTT 分布评测；
2. **发布策略决定**: 针对「低延迟」组在无 EPL 节点下的政策取向（补充 EPL 节点、更新 fallback 规则或调整修剪策略）待用户明确决策；
3. **前台人工浏览器端到端回执**: 生产登录通道具备后获取真实浏览器操作截图与回执；
4. **长周期运行观察**: 完成 2 小时 (7200s) 后台周期调度稳定运行态连续监控。
