# Data model

现有 Settings/Auth/DNS 保既有字段与端点，不迁移或重置。DNS draft 为 raw YAML 与解析对象的单轨编辑会话；无效字段编辑原文独立保留至纠正，不能用上次有效值静默替代保存。known/unknown 的嵌套对象都属于配置，不属组件可任意丢弃的 UI 状态。

新增绑定集合及行模型见 contracts/chain-api.md。稳定引用与可读名称分开，显示名从现库存/组派生。绑定 revision 与配置 version 对应两个并发范围，用户无需手动维护。订阅/组动态扩展不持久化节点清单；失效引用保留供修复，不 cascade 删除绑定。绑定效果和最终 dialer 字段仅属于解析快照，不修改原节点或其凭据/连接revision。

状态：new draft→preview(valid/invalid/stale)→CAS save→persisted row；编辑中的409留draft，明确重载或保留后rebase；disable不参与解析但留所有字段，delete先确认且检查集合合法性。排序/启用/引用变动必须改变输入指纹；最终快照包含winningassignment及skip原因。
