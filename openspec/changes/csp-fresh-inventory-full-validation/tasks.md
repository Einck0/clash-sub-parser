## 1. 节点库存清空闭包核心与仓储实现

- [x] 1.1 在 SQLite 仓储层实现原子节点重置闭包、受影响绑定捕获与前置/后置外键完整性校验，并通过单元测试验证
- [x] 1.2 在 inventory 应用服务层实现 ResetNodeInventory 业务逻辑、不可变发布解耦与旧抓取指针切断，保持抓取审计事实，并通过服务测试验证

## 2. 订阅源 Token URL 精准修复

- [x] 2.1 实现基于冷归档稳定身份证据的 7li 与魔戒 URL 精准恢复、幂等性跳过与脱敏审计，并通过单元测试验证
- [x] 2.2 验证其余 7 个订阅源状态与配置严格不变，且日志与输出完全脱敏（包含路径 Token 掩码）

## 3. 运维 CLI 命令与服务复用

- [x] 3.1 在 cmd/csp 中新增 reset-node-inventory 维护子命令，支持 --db, --dry-run, --apply, --confirm-backup, --backup-file, --restore-source-tokens，并通过 CLI 测试验证
- [x] 3.2 验证 dry-run 模式只读不写与 apply 模式的备份硬确认、原子提交与结构化 JSON 报告输出

## 4. 协议扩展与多凭据连接身份区分

- [x] 4.1 实现 HTTP, SOCKS5, VLESS, AnyTLS 协议扩展与 Mihomo 适配支持
- [x] 4.2 实现 domain.ComputeConnectionLogicalID 区分同端点多凭据节点，通过 fresh_private_test.go 门禁与单元测试验证

## 5. 隔离应用工作流自测与门禁闭环

- [x] 5.1 实现 same-service 隔离刷新工作流测试（涵盖 success/empty/fail/notice/manual/multi-credentials 与 last-good 保持）
- [x] 5.2 运行 go build ./...、全量测试（含 -race）与前端 type-check/tests，确保退出码均为 0

## 6. 全新订阅源全量拉取与失败根因归因分析

- [x] 6.1 对 4 个已启用订阅源（7li、魔戒、Dogegg、einck-qzz）执行全新抓取，验证节点解析入库（抓取 95 个候选节点，去重入库 90 个全新有效节点，全部标记 active=1，5 个禁用源严格未触碰）
- [x] 6.2 针对任何抓取失败或异常响应输出详尽根因归因分析与诊断记录（7li 返回空内容，归因诊断为订阅内容为空，如实记录失败而未伪造旧节点；魔戒 44、Dogegg 18、einck-qzz 33 成功入库）

## 7. 公平探针评测与发布候选筛选

- [x] 7.1 在隔离环境中对全部新节点执行公平 Benchmark 探针连通性与延迟评测（全量 90 节点完成 540 项有界任务：69 存活，385 成功，50 失败，105 跳过，耗时 5m54s，下载 54.5MiB 严格控制在 128MiB 预算内）
- [x] 7.2 依据策略规则与准入规则筛选 publication candidates 候选节点集（候选节点按规则流向汇入香港、台湾、日本、新加坡、美国、加拿大及其他组）

## 8. 预览发布生成与端到端成品验收

- [ ] 8.1 基于全新节点库存生成隔离预览发布快照（Preview Publication）
  - 真实局限说明：当前完整发布受阻于“低延迟”策略组在 90 真实节点中匹配数为空（缺失 EPL 节点），且作为“自动切换”必需 fallback 分支无法被可选修剪；“便宜”策略组原正则含 PCRE 负向零宽断言 `(?![\\d.])` 在 Go 正则引擎下属 unsupported，按真实保真策略不作临时猜测重写，故必需 fallback 分支空仍未验证，保持阻断事实呈报。
- [ ] 8.2 Critic 验收机对隔离预览 URL 执行多视口视觉与黑盒检验

## 9. 生产发布切换与有界稳定观察

- [x] 9.1 执行生产环境服务发布与节点重置切换（完成旧 1014 节点清空、两源 Token 修复、15 策略组正则过滤恢复、1 条 kliq 规则精准删除，生产容器已成功滚动至 414a6b7 最新镜像并恢复健康双端口服务）
- [ ] 9.2 实施有界时间窗口的生产服务运行态与性能稳定性观察（生产容器健康双端口运行已取证，但真实 7200s 周期与浏览器端前台实际操作超出本阶段预算，且与上游相同 stage 的配对效率基准对照未完成，保持 pending）

## 10. 规则只读校验增强与分流规则管理

- [x] 10.1 后端 Policy 校验与 Resolver 增强：在 resolver 与 policy 服务中将直接被分流规则指向的空组标记为 error（无论是否有 filter），输出结构化 issues（包含 rule_id, position, type, value, target_group_id, target_group_name, severity, message）并返回 revision_id，保持既有 valid/errors 字段兼容
- [x] 10.2 规则精准删除与事务安全测试：针对规则 01a0b9af-c118-72f9-9949-d94b49fa6ec2（指向“其他”组）编写精准删除与验证测试，确保 1->0 规则扣减、新 revision 生成、策略组保留与历史不可变发布快照隔离
- [x] 10.3 前端 PolicyView 增设分流规则 Tab、手动校验按钮、自动校验触发（初始/刷新/修改后）及防竞态守卫（AbortController + 请求序列），并通过类型与构建验证

