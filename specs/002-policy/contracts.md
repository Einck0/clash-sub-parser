# API & Integration Contracts: Policy Capability (Unified Member Source Operations)

> **治理状态**: `READY` (Sequential Add/Remove Pipeline Confirmed)  
> **关联工件**: `specs/002-policy/data-model.md`, `specs/002-policy/research.md`  
> **运行态绑定**: `run_id`: `pi_run_20261011_190715_2256208`, `change`: `002-policy`

---

## 1. RESTful API 契约

### 1.1 获取策略组列表 (`GET /api/v1/policies/groups`)
- **端点**: `GET /api/v1/policies/groups`
- **查询参数**:
  - `page`: 整数，页码，默认 1
  - `page_size`: 整数，每页数量，默认 50，最大 100
  - `search`: 字符串，按组名不区分大小写模糊搜索
- **响应状态码**: `200 OK`
- **响应体 Schema**:
```json
{
  "groups": [
    {
      "id": "01a0b9af-c116-7806-9740-66b8b070ba2f",
      "name": "选择节点",
      "group_type": "select",
      "empty_fallback_pass": false,
      "revision": 3,
      "sources": [
        {
          "id": "src-uuid-1",
          "operation": "add",
          "kind": "group_ref",
          "position": 0,
          "target_group_id": "01a0b9af-c116-75ee-9e58-b4a163f17cbc"
        },
        {
          "id": "src-uuid-2",
          "operation": "add",
          "kind": "group_nodes",
          "position": 1,
          "target_group_id": "01a0b9af-c116-7c5f-8a2e-c5363cc72b4d"
        },
        {
          "id": "src-uuid-3",
          "operation": "remove",
          "kind": "regex",
          "position": 2,
          "pattern": ".*(高倍率|过期|官网).*"
        },
        {
          "id": "src-uuid-4",
          "operation": "add",
          "kind": "manual_node",
          "position": 3,
          "node_logical_id": "01a0b9af-c116-70d1-a027-nodebackup1"
        }
      ],
      "created_at": "2026-09-20T10:00:00Z",
      "updated_at": "2026-10-11T12:00:00Z"
    }
  ],
  "page": 1,
  "page_size": 50,
  "total": 29
}
```

### 1.2 保存策略组 (`PUT /api/v1/policies/groups/:id`)
- **端点**: `PUT /api/v1/policies/groups/{id}`
- **语义**: 单事务原子更新元数据、全量替换该组的所有 `sources` 项（支持按顺序加减的有序列编排），并通过乐观锁 `expected_revision` 防止并发脏写。已彻底移除废弃的全局 `constraints` 字段。
- **请求体 Schema**:
```json
{
  "name": "选择节点",
  "group_type": "select",
  "empty_fallback_pass": false,
  "expected_revision": 3,
  "sources": [
    {
      "operation": "add",
      "kind": "group_ref",
      "position": 0,
      "target_group_id": "01a0b9af-c116-75ee-9e58-b4a163f17cbc"
    },
    {
      "operation": "add",
      "kind": "group_nodes",
      "position": 1,
      "target_group_id": "01a0b9af-c116-7c5f-8a2e-c5363cc72b4d"
    },
    {
      "operation": "remove",
      "kind": "regex",
      "position": 2,
      "pattern": ".*(高倍率|过期|官网).*"
    },
    {
      "operation": "add",
      "kind": "manual_node",
      "position": 3,
      "node_logical_id": "01a0b9af-c116-70d1-a027-nodebackup1"
    }
  ]
}
```
- **成功响应**: `200 OK`，返回最新策略组实体及自增后的 `revision: 4`。
- **错误响应**:
  - `409 Conflict` (并发版本冲突):
    ```json
    {
      "code": "revision_conflict",
      "message": "Policy group revision mismatch. Current revision is 4.",
      "current_revision": 4
    }
    ```
  - `422 Unprocessable Entity` (拓扑环路检测拦截):
    ```json
    {
      "code": "cycle_detected",
      "message": "Recursive dependency cycle detected in policy graph",
      "cycle_path": [
        "01a0b9af-c116-7806-9740-66b8b070ba2f",
        "01a0b9af-c116-7c5f-8a2e-c5363cc72b4d",
        "01a0b9af-c116-7806-9740-66b8b070ba2f"
      ]
    }
    ```
  - `422 Unprocessable Entity` (来源字段域校验失败):
    ```json
    {
      "code": "invalid_source_entry",
      "message": "Source kind 'group_ref' requires non-empty target_group_id and pattern must be empty",
      "field": "sources[0].target_group_id"
    }
    ```

### 1.3 发布预览 (`POST /api/v1/publications/preview`)
- **端点**: `POST /api/v1/publications/preview`
- **请求体**:
```json
{
  "target": "mihomo"
}
```
- **响应体**:
```json
{
  "target": "mihomo",
  "content": "proxies:\n  - name: ...",
  "content_type": "application/yaml",
  "filename": "mihomo.yaml",
  "selection_digest": "a1b2c3d4e5f67890123456789abcdef0123456789abcdef0123456789abcdef0",
  "content_digest": "e5f6a7b8c9d01234567890abcdef1234567890abcdef1234567890abcdef1234"
}
```
- **一致性规范**:
  - `selection_digest`: 对图解析器输出的节点列表、组列表及规则列表的逻辑内容计算出的 SHA-256 摘要。所有编译目标共享同一个逻辑 `selection_digest`；
  - `content_digest`: 对特定编译目标渲染出的文本字节计算出的 SHA-256 摘要；
  - 纯节点导出目标（`sing-box`, `surge`, `quantumult-x`）不因空策略组的存在而报错，直接输出有效节点清单。

### 1.4 策略组草稿实时预览 (`POST /api/v1/policies/groups/preview`)
- **端点**: `POST /api/v1/policies/groups/preview`
- **语义**: 纯内存执行策略组草稿解析，不产生任何数据库持久化写入（数据库 revision 不变）。复用 Admin 鉴权与 CSRF 中间件。支持现有组覆盖或全新组模拟解析。
- **请求体 Schema**:
```json
{
  "group_id": "01a0b9af-c116-7806-9740-66b8b070ba2f",
  "expected_revision": 3,
  "name": "选择节点",
  "group_type": "select",
  "empty_fallback_pass": false,
  "sources": [
    {
      "operation": "add",
      "kind": "group_ref",
      "position": 0,
      "target_group_id": "01a0b9af-c116-75ee-9e58-b4a163f17cbc"
    },
    {
      "operation": "add",
      "kind": "manual_node",
      "position": 1,
      "node_logical_id": "01a0b9af-c116-70d1-a027-nodebackup1"
    }
  ],
  "target": "mihomo"
}
```
- **成功响应**: `200 OK`
```json
{
  "group_id": "01a0b9af-c116-7806-9740-66b8b070ba2f",
  "revision": 3,
  "selection_digest": "a1b2c3d4e5f67890123456789abcdef0123456789abcdef0123456789abcdef0",
  "content_digest": "e5f6a7b8c9d01234567890abcdef1234567890abcdef1234567890abcdef1234",
  "members": [
    {
      "kind": "group",
      "group_id": "01a0b9af-c116-75ee-9e58-b4a163f17cbc",
      "display_name": "自动选择",
      "position": 0
    },
    {
      "kind": "node",
      "node_logical_id": "01a0b9af-c116-70d1-a027-nodebackup1",
      "display_name": "香港 01 节点",
      "position": 1
    }
  ],
  "node_count": 5,
  "diagnostics": []
}
```
- **错误响应**:
  - `409 Conflict`: 预期版本不匹配 (`expected_revision != revision`)；
  - `422 Unprocessable Entity`: 拓扑环路 (`cycle_detected`) 或非法来源项 (`invalid_source_entry` / `invalid_regex`)。

### 1.5 既有 API 向新端点转换与类型契约 (Legacy Payload to New Endpoint Transition)
- **迁移转换路径**: 客户端在调用保存时，由前端适配器将旧式的 `edges` + `filter` 扁平化映射为统一的 `sources` 数组：
  - 旧 `edges` 中 `NodeLogicalID != nil` 转换为 `op: "add"`, `kind: "manual_node"`, `node_logical_id`；
  - 旧 `edges` 中 `ChildGroupID != nil` 转换为 `op: "add"`, `kind: "group_ref"`, `target_group_id`；
  - 旧 `filter` 独立存在的组转换为 `op: "add"`, `kind: "filter"` 或 `"regex"`（`inventory_query_source`）；
  - 旧 `edges` 与 `filter` 共存的组，将 `filter` 作为带约束谓词转换，保全原交集语义，严禁偷扩大；
- **单一事实源与发布物契约**:
  - `policy_group_sources` 成为唯一的系统权威，不维持长期双写；
  - 迁移后保持现有策略组 stable UUID (`id`)、`revision` 序列、`position` 与 COW 不可变发布物契约不变。

