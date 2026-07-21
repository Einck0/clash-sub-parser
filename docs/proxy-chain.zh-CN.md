# 链式代理设计（Proxy Chain）v2

> 状态：**设计中（v2）** — 旧 P0 实现已回退（`f776049`）  
> 代码回滚点：`pre-proxy-chain-design` @ `d0988c4`（TCP 探活 + 统一减组）  
> 旧设计问题：链绑在「订阅编辑中途」、只能订阅/节点两级、UI 手打最终名难用  
> 新目标：在**一切设置结束后**再配置链式；支持订阅 / 策略组 / 节点三级；跳板可以是节点或策略组

---

## 1. 你要的心智（产品语言）

先把订阅、筛选、重命名、策略组都配完，得到一份「已就绪」的世界：

```text
最终节点列表（所有订阅处理后）
策略组列表（按 include_entries 解析完）
规则 / DNS …
```

**然后**再单独配链式代理：

| 我要给谁挂链 | 例子 |
|---|---|
| 某个**订阅**里所有最终节点 | 7li 整包默认先走「香港」组 |
| 某个**策略组**里解析出的成员 | 「美国」组里的节点都先走「香港」 |
| 某个**单独节点** | 某一个美国节点先走某一个香港节点 |

| 跳板（dialer）可以是 | 例子 |
|---|---|
| **节点** | `[ss]香港\|Yxvm\|…` |
| **策略组** | `香港` / `手动选择` |

生成结果仍是 Clash Meta：落地侧写 `dialer-proxy: <跳板名>`。

```text
本机 → [跳板：节点或策略组] → [被挂链的落地节点] → 网站
```

---

## 2. 本质三问

### 解决什么？

在**节点集合与策略组已稳定之后**，给部分出口统一/单独指定建连前置跳板。  
不是规则分流，不是策略组「选哪个节点」，是 **proxy 怎么拨号出去**。

### 属于哪一类？

生成期的 **outbound dialer 覆盖**（后置 pass），挂在完整 `proxies` + `proxy-groups` 快照之上。

### 更一般的抽象？

不要三套互不相干的 UI 状态机。统一成：

```text
ChainBinding {
  target:  谁被挂链   — subscription | node_group | node
  dialer:  跳板是谁   — ref → node_name | group_name
  # P0 单跳；P1 再扩展 dialer 为有序 hop 列表
}
```

优先级（从高到低，后写覆盖前写）：

```text
节点绑定  >  策略组绑定  >  订阅绑定  >  无链
```

同一落地被多条规则命中时，取最高优先级；同级冲突以更具体为准（节点 > 组 > 订阅）。

**明确不做：** 链上再嵌套解析另一条 chain binding（防环爆炸）。若跳板节点自己也有 dialer，交给 Clash 原生叠层，产品层只警告。

---

## 3. 为什么旧 P0 不对

| 旧做法 | 问题 |
|---|---|
| 链字段塞进订阅表单中段 | 和筛选/重命名搅在一起，心智是「编辑订阅时顺便链」 |
| 只有订阅默认 + 节点 map | 不能「整个策略组统一入口」 |
| 手打最终名 | 7li 长名不可用；缺「处理后节点台账」 |
| 设计写死「NodeGroup 不做第三套」 | 和你要的「策略组也能挂链」冲突 |

v2：**链式是独立后置配置面**；订阅编辑只负责产出最终节点，不再塞链编辑。

---

## 4. 数据模型（建议）

### 4.1 独立表 `proxy_chain_bindings`（推荐）

比继续往 `subscriptions` 塞 JSON 更贴「后置全局配置」：

```text
id
target_type     # subscription | node_group | node
target_id       # subscription_id / node_group_id；node 时可为 null
target_name     # node 时用最终节点名；组/订阅也可冗余名方便展示
dialer_type     # node | node_group
dialer_ref      # 最终节点名 或 策略组名
enabled         # bool
sort_order      # 可选，同级展示
note            # 可选
```

P0 单跳：`dialer_*` 一个即可。  
P1 多跳：可加 `dialer_hops JSON` 有序列表，导出时再包装中间节点。

### 4.2 备选（不推荐当主方案）

继续 `Subscription.proxy_chain` + `node_proxy_chains` + `NodeGroup.proxy_chain`  
— 分散三处，优先级与全局 UI 难做，违背「设置结束后再配」。

### 4.3 旧列

线上 SQLite 可能仍残留 `proxy_chain` / `node_proxy_chains` 列（代码已不读）。  
实现 v2 时可忽略或 migration 丢掉，不读旧 JSON。

---

## 5. 生成顺序（关键）

```text
1. 收集 enabled 订阅 → 最终 proxies（筛选/前缀/重命名已完成）
2. 解析策略组 → proxy-groups（include_entries 等）
3. ★ 应用 proxy_chain_bindings（后置）
     - 展开每个 binding 的「落地集合」
     - 按优先级给落地节点写 dialer-proxy
     - 跳板名必须 ∈ 最终 proxy 名 ∪ 策略组名
4. 规则 / DNS
5. dump YAML
```

落地集合展开：

| target_type | 落地集合 |
|---|---|
| `node` | `{ target_name }` |
| `subscription` | 该订阅 `raw_nodes` 的最终名集合 |
| `node_group` | 该组 `resolve_entries` 后的**成员节点名**（只给叶子 proxy 写 dialer；成员若是子组名则继续解析到节点，或仅对「直接节点成员」生效——实现时选一种并写死测试） |

**推荐：** 策略组绑定 = 对该组**递归解析后的全部叶子节点名**写 dialer（与组预览一致）。  
子组本身不当「落地」写 dialer。

跳板：

- `dialer_type=node` → `dialer-proxy: <节点最终名>`
- `dialer_type=node_group` → `dialer-proxy: <策略组名>`（Clash 允许 group 作 dialer）

校验：

- 跳板存在；落地存在  
- 落地 ≠ 跳板（节点自指禁止）  
- 禁止 `DIRECT` / `REJECT` / `PASS` 当跳板  
- 可选：检测「组 dialer 成员是否包含落地」造成的逻辑环并警告  

---

## 6. UI（后置配置面）

### 独立页或生成页旁：**链式代理**

不是塞进订阅表单中间。

布局建议：

1. **绑定列表**（已有规则）  
   - 目标类型标签：订阅 / 策略组 / 节点  
   - 目标名、跳板名、启用  
2. **新增绑定**  
   - 目标：下拉（订阅列表 / 策略组列表 / 最终节点列表，可搜索）  
   - 跳板：下拉（最终节点 + 策略组，可搜索）  
3. 与 **节点台账**（若做）联动：在节点行快捷「设跳板」= 创建 `target_type=node` 绑定

### 和「节点管理」的关系

- **节点管理 / 台账**：看清「处理后有哪些最终节点」——链式的选人器依赖它  
- **链式代理页**：在台账/组/订阅之上挂 dialer  

可先做链式页 + 简单节点名下拉；台账增强（探活、批量）并行或稍后。

---

## 7. API 草图

```text
GET    /api/proxy-chains
POST   /api/proxy-chains
PATCH  /api/proxy-chains/{id}
DELETE /api/proxy-chains/{id}
POST   /api/proxy-chains/validate   # 可选：未知名、冲突预览
GET    /api/nodes/final             # 可选：所有最终节点名（供下拉）
```

生成仍走现有 `/api/generate`；bindings 在服务端生成时自动应用。

---

## 8. 分期

| 阶段 | 内容 |
|---|---|
| **P0** | 表 + API + 生成后置单跳；目标：订阅 / 节点 / 策略组；跳板：节点或策略组；独立简陋 UI |
| **P1** | 节点台账页（最终节点浏览/搜索/快捷设链）；冲突预览 |
| **P2** | 多跳 hop 列表 + 中间包装节点 `__chain/...` |
| **不做** | 自动猜入口、整链延迟、relay 双轨存储、订阅表单中途塞链 |

---

## 9. 验收例子（7li）

目标：美国某节点先走香港某节点。

1. 订阅/筛选/组都已 OK  
2. 链式页新增：  
   - 目标类型 = 节点  
   - 目标 = `[ss]美国|砖线|三网|x15`  
   - 跳板类型 = 节点  
   - 跳板 = `[ss]香港|Yxvm|砖线|三网|x15`  
3. 生成 YAML 中该美国节点含 `dialer-proxy: [ss]香港|Yxvm|砖线|三网|x15`

或：目标 = 策略组「美国」，跳板 = 策略组「香港」→ 美国组内叶子节点均 `dialer-proxy: 香港`。

---

## 10. 回滚与现状

| 项 | 状态 |
|---|---|
| 代码 | `f776049` Revert P0；`origin/dev` 已推；容器 healthy |
| 模型 | 不再含 chain 字段 |
| DB | 可能残留空 JSON 列，无害；v2 用新表 |
| 旧 tag | `pre-proxy-chain-design` @ `d0988c4` 仍可用 |

```bash
# 若需再退到 P0 之前的稳定线（一般不必，当前已是 revert 后）
git checkout pre-proxy-chain-design
docker compose up -d --build
```
