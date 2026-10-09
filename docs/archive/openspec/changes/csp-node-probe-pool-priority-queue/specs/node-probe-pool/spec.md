## Purpose

实现统一的节点池（Node Probe Pool）双端优先级队列：定时测速触发时将节点按序加入节点池队尾且对已在池中的节点自动去重不重复添加；手动发起检测时将目标节点插在节点池队列最前面优先执行（或将已在队列中的节点提升至队首）；为节点新增“检测中（`probing`）”实时状态，提供 `GET /api/v1/probes/pool` 实时返回「当前队列中的节点数、未测数、总数、不可用数、可用数」5 大核心指标，并在前端打造直观美观的节点池状态看板与“检测中”全链路交互体验。

## ADDED Requirements

### Requirement: 定时测速节点池去重入池（Periodic Probe Node Pool Deduplication）
当定时测速（`PeriodicCoordinator` 或 `POST /api/v1/probes/schedule/trigger`）触发时，系统 MUST 将当前活跃节点按序加入节点池队列尾部；如果某个节点已经在节点池中（无论处于排队等待 `queued` 还是正在检测 `probing`），系统 MUST 静默跳过该节点，绝不重复添加入池，且 MUST NOT 因 `ErrNodeConflict` 导致定时批次或探测任务报错失败。

#### Scenario: 定时测速遇到已在节点池中排队或正在检测的节点时静默去重跳过
- **WHEN** 节点 `node-hk-01` 已在节点池中（处于 `probing` 正在检测或 `queued` 排队等待），此时定时测速触发并对活跃节点 `["node-hk-01", "node-sg-02"]` 入池
- **THEN** 系统 MUST 跳过已在池中的 `node-hk-01`（不重复入队、不抛出 `ErrNodeConflict`），仅将不在池中的 `node-sg-02` 加入节点池队列尾部，并正常完成批次与观测持久化

### Requirement: 手动检测插在节点池队列最前面（Manual Probe Front-of-Queue Preemption & Promotion）
当用户手动触发检测（单节点重测、测速已选节点或一键全量测速）时，系统 MUST 将目标节点插在节点池队列最前面（队首）优先调度执行；若目标节点原本已在节点池等待队列后方（如定时测速排队中），系统 MUST 将其提升（Promote）至队列最前面并合并所需探测维度；若目标节点当前正处于 `probing`（检测中），系统 MUST 允许复用或紧接队首完成检测，绝不返回冲突错误。

#### Scenario: 定时测速队列排队期间手动触发单节点或批量检测插队到最前面
- **WHEN** 节点池队列中有定时测速任务 `["node-01", "node-02", ..., "node-20"]` 正在排队等待，用户手动触发对 `"node-18"` 与 `"node-99"` 的检测
- **THEN** 系统 MUST 将 `"node-18"`（从原队列位置提升）与 `"node-99"` 插在节点池队列最前面，使下一个空闲工作协程优先执行 `"node-18"` 与 `"node-99"` 的检测，再去执行其余定时排队节点

### Requirement: 节点状态支持“检测中（`probing`）”与节点池 5 大核心统计指标 API
后端 `GET /api/v1/nodes`、`GET /api/v1/nodes/{logical_id}` 与 `PATCH /api/v1/nodes/{logical_id}/connection` 返回的 `NodeView` MUST 包含 `probe_state`（`"probing"` | `"queued"` | `"idle"`），且当节点正在节点池中执行检测时将 `probe_state` 置为 `"probing"`、`health_status` 置为 `"probing"`。后端 MUST 提供 `GET /api/v1/probes/pool` 接口，实时返回节点池与全网活跃节点的 5 大核心统计指标：`queue_nodes_count`（当前队列中的节点数）、`untested_count`（未测数）、`total_count`（总数）、`unavailable_count`（不可用数）、`available_count`（可用数），以及 `probing_count`、`queued_waiting_count`、`healthy_count`、`degraded_count`、`probing_node_ids` 与 `queued_node_ids`。

#### Scenario: 查询节点池统计接口与节点列表实时反映 5 大指标及“检测中”状态
- **WHEN** 仓库共有 10 个活跃节点（其中 5 个可用、2 个不可用、3 个未测），当前有 2 个节点正在执行检测（`probing`）、3 个节点在池中排队等待（`queued`），管理员调用 `GET /api/v1/probes/pool` 与 `GET /api/v1/nodes`
- **THEN** `GET /api/v1/probes/pool` MUST 返回 `queue_nodes_count=5`（`probing_count=2`、`queued_waiting_count=3`）、`untested_count=3`、`total_count=10`、`unavailable_count=2`、`available_count=5`；且 `GET /api/v1/nodes` 中对应检测中节点的 `probe_state` 与 `health_status` MUST 为 `"probing"`

### Requirement: 前端节点池状态看板展示 5 大核心指标与节点“检测中”动态状态设计
探针引擎前端（`web/src/features/probes/ProbesView.vue`）顶部 MUST 设计直观、美观的节点池检测状态看板，醒目显示「当前队列中的节点数」、「未测数」、「总数」、「不可用数」、「可用数」5 项核心指标（并支持点击指标卡快速筛选对应状态节点）以及定时自动入池控制卡片。`ProbesView.vue` 与 `NodesView.vue` 的节点状态徽章、状态筛选器与详情抽屉 MUST 支持 **「检测中」**（`probing`，带动态呼吸/旋转指示）与 **「队列中」**（`queued`）状态展示，并在手动测速时即时反馈插队优先状态。

#### Scenario: 用户在探针工作台查看节点池 5 大指标看板并触发手动插队检测观察“检测中”状态
- **WHEN** 用户打开探针引擎页面查看顶部看板，并点击某节点的「立即重测」或顶部「一键全量测速 / 测速已选节点」
- **THEN** 顶部看板清晰展示「当前队列中的节点数、未测数、总数、不可用数、可用数」5 大指标；被触发节点立即显示「检测中」（或「队列中」）动态徽章及插队提示，状态筛选器可按「检测中」筛选，检测完成后自动刷新为最新可用/不可用状态与延迟毫秒数
