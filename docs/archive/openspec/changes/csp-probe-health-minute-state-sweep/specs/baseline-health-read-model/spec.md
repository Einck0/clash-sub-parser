## Purpose

统一节点详情、台账和探测池的整体线路健康判断口径：整体健康只从足够新鲜且可判定的 baseline 观测得出，能力探针和安全/配置错误保持独立可见，不夸大可用或异常节点数。

## ADDED Requirements

### Requirement: 整体健康以新鲜 baseline 为唯一依据
系统 SHALL 仅使用新鲜且可判定的 baseline 结果判定节点整体健康；健康/可用需要 baseline 实际通过，线路不可用需要 baseline 真实连通性/传输失败的有效证据。baseline 缺失、过期、unknown/stale，或因安全策略、配置与探针能力阻断而无法完成检测时，整体状态 SHALL 为未知/无法检测，不得以其他类别的失败或成功代替，也不得把阻断计为“已证线路不可用”。

#### Scenario: 非 baseline 成功或失败但 baseline 缺失
- **WHEN** 流媒体或 AI 类别有成功或失败，baseline 未有有效观测
- **THEN** 整体健康未知；各能力结论仅在自身类别呈现

#### Scenario: baseline 过期而能力类别新鲜
- **WHEN** baseline 观测已过新鲜度阈值，但其他能力观测新鲜
- **THEN** 整体健康未知、标识 baseline 过期，不显示来自其他能力的整体线路延迟

#### Scenario: baseline 安全拒绝或配置失败
- **WHEN** baseline 由于不安全 TLS、私有目标或探针构建错误失败
- **THEN** 节点显示未知/无法检测及脱敏原因，既不视为健康也不视为已证异常

#### Scenario: baseline 可判定的连通结果
- **WHEN** 新鲜 baseline 观测证明成功或有真实连通性失败证据
- **THEN** 整体健康相应为健康/异常，线路延迟只在成功 baseline 有有效耗时数据时显示；其他能力错误不覆盖此判断

### Requirement: pool 与节点读模型同口径且兼容
系统 SHALL 使用同一健康判定规则产出节点视图和探测池统计，所有激活节点的互斥计数和 SHALL 等于总数；未知/无法检测 SHALL 计入现有未确认/未测类别而非可用或异常类别，并可从现有能力摘要解释原因；不得悄然增加不兼容的必填响应字段或改变现有订阅过滤语义。

#### Scenario: 多种观测混合聚合
- **WHEN** 激活节点同时包含健康 baseline、真实失败 baseline、阻断 baseline、过期 baseline、以及无 baseline
- **THEN** pool 的健康、降级、异常和未知计数与逐节点视图的相应整体状态一致且互斥；受阻节点不被统计为已证坏线路
