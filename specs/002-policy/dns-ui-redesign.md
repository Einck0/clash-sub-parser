# DNS 页面重设计：规格与模块方案（2026-10-25）

Run `pi_run_20261025_123522_590576` / change `change_pi_run_20261025_123522_590576` / package `whole_task` / epoch 1。当前 Feature 仍为 002-policy，由 `.specify/feature.json` 和官方 check-prerequisites 确认。本文件是 spec.md、plan.md 当前 DNS 增量的组成部分；历史已结束任务、生产 DNS 恢复授权不继承。

## 用户结果与授权

独立设计实施更好的 CSP DNS 前端，并提供高级配置。用户未指定服务商、出口策略或网络安全方案，本方案不引入这些未经确认的偏好。日常正常路径为进入 DNS → 修改主解析服务器/解析模式 → 按需要展开高级配置 → 查看服务端规范化草稿 → 校验保存 → 回读。配置作用于导出的 Mihomo 客户端，不修改 CSP 宿主 DNS。开发提交推送 dev 与云测已授权，生产程序部署及生产配置写入未授权。

入口 HEAD d0cee8ae89a8248d78a582b84810211280cba68b。非任务 dirty 已保存 `/tmp/csp-dns-590576/baseline/entry.patch`，SHA256 ed73cc6ca9402aa75e2a3e6f8b1abc4a914412088c029a609c278400d5446416，禁止夹带。DNS 源文件入口干净。graphify-out/graph.json 和 CLI 均不存在，不安装不伪称更新。

## 已核实问题与取舍

当前 DnsView.vue 将六个列表、所有开关、两段 JSON 同时平铺，主 nameserver 不突出； enhanced-mode 缺省被显示成 fake-ip，布尔缺省显示 false，容易造成错误理解。cache-algorithm 清空写空字符串而非删除。页面称 YAML 保留注释，但后端 NormalizeDNSSettings 重新序列化并不保留注释。useDnsDraft.save 校验前后可编辑，异步回读可能覆盖后续草稿；JSON 解析接受数组/null，不符合 mapping 字段契约。已有 CAS409、未知键保全、切换校验与离开保护必须保留。

后端 domain/dns.go 已支持 prefer-h3、direct-nameserver-follow-policy、proxy-server-nameserver-policy；不需要新增 API、数据表或改编译器。使用现有 Vue3/TS、DaisyUI、Heroicons 与 uiText，零新增依赖。官方参考 https://wiki.metacubex.one/config/dns/ 已取得（本轮副本 `/tmp/csp-dns-590576/mihomo-dns.html`）；不把 CSP 的结构校验称作 Mihomo 运行时或 DNS 连通性测试。

## 功能规格

- **FR-DU1 / P1 清晰主路径**：不恢复重复页内大标题/横幅。紧凑工具栏展示启用、编辑模式、未保存/已保存状态、重载。基础区域优先主解析服务器 nameserver、多行说明及增强模式，明确未指定值不代表强制默认。用中文业务名＋次级技术键名和中英说明；避免全屏微小文本。禁用 DNS 不清空草稿，并说明导出 enable=false。
- **FR-DU2 / P1 高级配置**：按“连接与引导”（default/proxy/direct nameserver、listen、respect-rules、direct-nameserver-follow-policy、prefer-h3）、“Fake-IP 与缓存”（range、filter、cache）、“回退与域名策略”（fallback、nameserver-policy、fallback-filter、proxy-server-nameserver-policy）、“Hosts”（use-hosts、use-system-hosts）分组折叠，基础 ipv6 可置主路径。折叠仅改变显示，不能卸载/丢弃无效输入或数据。已配置数量/说明便于发现高级项。mapping 使用有示例/格式提示的 JSON 对象编辑器，不新增平行 YAML 解析器。未知字段完整保全并显示键名/Raw 编辑入口，不自动删除。新开关仅覆盖真实后端支持字段；不提供会偷偷覆盖已有配置的服务商预设。
- **FR-DU3 / P1 单草稿无损**：可视与 Raw 共用草稿，通过原 validate 接口转换。显式“未指定”或重置操作删除键，布尔应可区分未设置/true/false；未触碰的键和未知键完全保持。标准已知选项以外值不能自动替换，显示实际值或提示 Raw 编辑。JSON 对象不接受数组/null/scalar，保留无效输入并阻止保存、切换和预览。Raw 说明：服务端规范化，注释/格式可能变化，不承诺注释保留。全部字段可恢复为未指定。
- **FR-DU4 / P1 可信预览保存**：显式“预览 YAML”调用原 validate，显示规范化后的当前草稿及 enable，以只读区域呈现；没有持久写入。编辑后立即失效/隐藏旧预览并标注重新生成，避免把旧结果称为当前。预览不是实时 DNS 测试或已发布 artifact。保存顺序 validate → PUT expected_revision → GET，409/422/网络失败保留草稿、不自动覆盖冲突。所有异步操作期间禁用页面编辑/启用及动作；状态逻辑仍用 generation/draftRevision 防止过期结果应用。成功显示来源、revision、时间，不把默认 source 误称已保存。重载脏草稿先确认，离开保护保留。
- **FR-DU5 / P2 响应与可访问性**：遵循应用主题与自适应布局，桌面有层次、移动单列无横向溢出，操作栏不遮挡末项。不新增全局样式/导航。真实 label、aria-expanded/controls、明确错误、键盘可操作。中英文完整，业务说明解释 bootstrap、回退、respect-rules 等，不宣传隐私/防泄露或连通性已验证。

## 模块边界与冻结接口

A 草稿状态：`web/src/features/dns/useDnsDraft.ts`、`dnsTypes.ts`、`useDnsDraft.test.ts`。负责单草稿转换、字段删除/类型校验、预览、并发防护、API错误。保留现有 API，新增 `removeKey(key: string): void`、`preview(): Promise<void>`、`previewYaml: Ref<string>`（仅当前有效结果，任意实际编辑/启用变更后清空）、`previewing: Ref<boolean>`；`busy` 包含预览。保留全部现有公开 getter/setter、save/load/switchMode。enabled 的直接 v-model 变化也必须触发预览失效/版本计数。已知键集合补充三个上述高级字段。JSON 空输入删除该字段；非空必须普通对象。setString 空值删除；列表清空可保持显式 []，删除用 removeKey。异步任务捕获 enabled、revision、draft/generation，不提交混合版本，不覆盖请求发出后变更。保存中 UI 禁改，状态层也防过期响应。状态模块不读/改 View 或 UI 模块。

B DNS 呈现：`web/src/features/dns/DnsView.vue`、`DnsView.test.ts`，可新建同目录 `Dns*` 组件及 `dnsFields.ts`（不可动 A 的文件）。使用 A 冻结接口，负责上述信息架构、文案、折叠字段、未指定控制、只读预览、busy 禁用。字段定义在本模块单点维护。可只读 useDnsDraft.ts/dnsTypes.ts 和 useUnsavedChanges.ts、api/client.ts、ui/PageToolbar.vue、locales/redesign.ts、同目录现有测试；不扩搜其他模块。两个模块写集不相交、无共享生成物，同时施工。不新增依赖或修改 package/CI/global CSS。需要模块外事实 caller_ping 主脑。

集成验证/预览准备：主脑核对差异后协调 dev 开发提交与推送；施工机续接云端结果，运行同 SHA 预构建镜像，纯合成 DB、loopback 隔离端口、禁外联/调度；提供 image digest、OCI revision、readyz 与 URL-ready 合同及回收责任。可以新建本轮 `/tmp/csp-dns-590576/` 工件，不挂载生产数据。审查机独立核对当前 SHA Actions，工程审查后再派 critic。

## 具体场景与验收

1. 合成 default/revision0 仅 enable/nameserver：首屏能改主服务器；增强模式与高级布尔显示未指定，不隐式补 fake-ip/false。预览只 POST，source/revision 不变；保存后回读 saved/revision+1，再次进入字段一致。
2. 合成已有复杂 mapping、三个新增高级字段和未知 x-custom：改 nameserver，折叠/展开，Raw往返并保存；所有未触碰键 deep-equal。字段设未指定后键消失，不是空字符串或 false。
3. nameserver-policy 输入 `[`、`null`、`[]`：保留输入，就地报错，不能预览/切换/保存；折叠后不丢，修正为对象恢复。非法 YAML/422 同样保留。
4. 生成预览→修改任意输入/enable：旧预览不再作为当前结果；慢 validate 后改草稿不能被旧响应覆盖。保存期间字段/开关/切换不可编辑，409 真双客户端冲突保留本地内容；重载取消仍保留草稿。
5. 1440桌面、392移动视口及英文/暗色可操作，键盘折叠/表单/保存可达，无重复标题横幅、横向滚动或操作覆盖。实际成品由 critic 亲读截图并真实编辑保存验收；不宣称物理手机验证。
6. 纯合成隔离实例当前导出 DNS 与保存 map 一致；enable=false 正确导出。不修改生产库、不探测真实上游或修改宿主 DNS。

## Spec Kit 一致性与宪章

本轮是已有 Feature 的受影响章节增量，不执行会覆盖 plan 的 setup-plan，不改 constitution 和旧任务结论；无 extensions.yml/hooks。按官方 skills specify/plan/tasks/analyze 工作流维护。需求覆盖 DU1→D25-02；DU2→D25-01/02；DU3→D25-01/02/04；DU4→D25-01/03/04；DU5→D25-02/04。所有当前需求有任务、所有任务有需求或横切门禁归属。模块契约明确，无未决业务问题、无新增外部API/迁移。资产保全、领域解耦、极简存储、活规范保持。当前设计分析无 CRITICAL/HIGH，READY FOR IMPLEMENTATION；验收状态以 tasks.md 当前 D25 系列为准。
