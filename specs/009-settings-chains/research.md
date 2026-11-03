# Research and decisions

- 当前链功能缺失覆盖全栈：旧 ProxyChains.vue、proxy_chain_bindings、Python proxy_chain_service.py 在 Go 重写中未恢复；历史 v2 设计包含三类目标、两类跳板、覆盖优先级、预览和重合防御。当前无对应表/API/ResolvedNode.DialerProxy/compiler 字段，故只加入口不充分。
- 决定移植业务语义而不移植 Python/名称身份：复用现 Node logical_id、Group ID、Subscription ID、Go resolver 动态来源。旧同级排序覆盖含混，冻结较小 sort_order 后 ID 优先，UI 明示。
- 当前 publication.AssembleResolveInput 依赖 filterDependencies 按需取 NodeSources，新增订阅链需主动请求 sources；不能因无sourcefilter而漏数据。非Mihomo现为pure-node snapshot，故本轮有启用链明确拒绝，不能强迫所有无链导出验证不相关组。
- 环检复用单 resolver 算法，配置集合 CAS 防两个并发编辑分别合法、合并却成环；不新增用户管理的全局版本对象。新表与旧资料不自动关联。
- CSP Settings 当前把不同保存边界杂糅在大View；拆用途和状态模块但沿现auth/settings/filter/probe端点，不改后台安全模型。DNS现useDnsDraft/17键/unknown/CAS链有效，主要重做分组、逐项列表与高级映射YAML输入，不推倒既有可靠保存链。
- 本机无 graphify executable/graph.json，无法query/update，不安装工具或伪造成功。既有云工作流为唯一测试编译入口；保持Vue3/Tailwind4/Daisy5，不升级依赖。
