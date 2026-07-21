# 链式代理设计（Proxy Chain）

> 状态：设计定稿，未实现  
> 回滚点：git tag `pre-proxy-chain-design` @ `d0988c4`  
> 目标：手动为**节点**或**整份订阅**配置完整前置代理链，生成时落到 Clash Meta 的 `dialer-proxy`。

---

## 1. 本质三问

### 这东西解决什么约束？

用户希望：**某些落地节点 / 某整份订阅的节点，建连时必须先经过一条可配置的前置链路**。  
例如：`本机 → 香港入口 → 日本中转 → 美国落地 → 目标`。

这不是规则分流（rules），也不是策略组选节点；是 **proxy 建连路径**本身。

### 属于哪一类？

**出站拨号器（outbound dialer）覆盖**。  
Clash Meta 原生表达是节点上的 `dialer-proxy: <name>`（name 可以是 proxy 或 proxy-group）。

### 有没有更一般的抽象？

不要为「订阅链」「节点链」「策略组 relay」各做一套。

统一成一条有序链：

```text
proxy_chain: [hop1, hop2, ..., hopN]
```

生成时**只**展开为最终落地节点上的 `dialer-proxy`（必要时生成中间虚拟节点/组名）。  
策略组 `type: relay` 作为可选导出形态，**不做**第二套编辑模型。

---

## 2. Prior Art（只借原理）

| 来源 | 可借鉴 | 不借鉴 |
|------|--------|--------|
| Clash Meta `dialer-proxy` | 节点级链式，落地最通用 | 不在 UI 暴露全部引擎细节 |
| Meta `relay` group | 固定多跳可视化 | 不当唯一存储模型；WG 等有限制 |
| 订阅 override / additional-prefix | 整订阅统一前缀/覆盖 | 不做完整 provider 子系统 |
| ProxyChainStudio 类工具 | 「链 = 有序列表」 | 不引入独立生成器仓库 |

**结论：** 产品模型用「有序 hop 列表」；导出优先 `dialer-proxy`。

---

## 3. 产品语义

### 3.1 两级配置（你要的）

| 作用域 | 含义 | 优先级 |
|--------|------|--------|
| **订阅级** `Subscription.proxy_chain` | 该订阅下所有最终节点默认走这条链 | 低 |
| **节点级** `node_proxy_chains` 映射 | 某个最终节点名覆盖自己的链 | **高** |

解析规则：

```text
effective_chain(node) =
  node_level_chain(node.name)   if 有
  else subscription.proxy_chain if 有
  else []                       # 不链
```

空链 `[]` = 明确关闭（节点级可用 `null`/缺省表示「跟随订阅」；用 `[]` 表示「不要链」）。

### 3.2 链里每一跳是什么？

每一跳是一个 **Clash 出站名**，必须是生成结果里已存在的：

- 某个 **proxy 节点名**（通常来自其它订阅/手动节点，常作入口）
- 或某个 **proxy-group 名**（如「香港」「手动选择」）

**不**在链里嵌套再解析另一条 `proxy_chain`（防爆炸）。若 hop 自身也有 dialer-proxy，以引擎原生语义为准（通常是再叠一层拨号），产品层校验时**警告**即可。

### 3.3 完整链如何落到 YAML？

Clash 原生每个 proxy 只有一个 `dialer-proxy` 字段。  
多跳链 ` [A, B, C] ` + 落地 `Exit` 的语义是：

```text
本机 → A → B → C → Exit → 目标
```

实现方式（**唯一推荐**）：

1. 生成虚拟中间节点名（确定性、可复现）：
   - `Exit` 的 dialer = 链上最后一跳 `C`（若 C 是组名则直接 dialer-proxy: C）
   - 若需要「A→B→C」且 A/B/C 都是叶子 proxy，则为中间 hop 生成**包装节点**或依赖 hop 自身已配置的 dialer
2. **简化落地（P0）**：产品链长度建议 1～N，导出时：
   - **单跳** ` [A] `：落地节点写 `dialer-proxy: A`
   - **多跳** ` [A, B, C] `：为中间跳生成确定性别名节点：
     - `__chain/{hash_or_path}/B` 拷贝 B 的配置，并设 `dialer-proxy: A`
     - `__chain/{hash_or_path}/C` 拷贝 C 的配置，并设 `dialer-proxy: __chain/.../B`
     - 落地 Exit 设 `dialer-proxy: __chain/.../C`（若最后一跳已是组名则不拷贝，直接 dialer 到组）

P0 也可先只支持「链 = 1 个 hop（节点或组）」——覆盖 80%「机场入口 + 落地」；多跳在 P1 用虚拟包装节点展开。

**推荐分期：**

| 阶段 | 能力 |
|------|------|
| **P0** | 订阅级 + 节点级；链长 = 0 或 1；导出 `dialer-proxy` |
| **P1** | 链长 ≥ 2；中间 hop 自动生成包装 proxy |
| **P2** | 可选导出 `type: relay` 展示组（只读/调试）；UI 链预览 |

---

## 4. 数据模型

### 4.1 Subscription

新增：

```text
proxy_chain: list[str]          # 有序 hop 名，默认 []
node_proxy_chains: dict[str, list[str] | null]
  # key = 最终节点名（前缀后 + rename 后，与策略组匹配同一套名字）
  # value = 链；null/缺省 key = 跟随订阅；[] = 强制不链
```

说明：

- key 用**最终名**，与现有 `node_renames` / 策略组正则一致，避免「源名 / 前缀名」双轨。
- 手动节点、订阅节点共用最终名空间；重名时与现有 `deduplicate_nodes` 行为一致。

### 4.2 不新增表

- 不单独做 `proxy_chains` 表（减法）。
- 不在 NodeGroup 上再做第三套链（策略组选节点 ≠ 建连链）。若未来要「整组出口统一入口」，用**组名作为 hop**即可。

### 4.3 迁移

- SQLite/Postgres：`proxy_chain JSON NOT NULL DEFAULT '[]'`
- `node_proxy_chains JSON NOT NULL DEFAULT '{}'`
- 与现有 `database.py` 补列风格一致。

---

## 5. 生成链路（改哪里）

```text
_collect_all_nodes
  |-- 拉 enabled 订阅 raw_nodes
  |-- dedup
  |-- ★ 对每个 node 解析 effective_chain
  |-- ★ 写入 dialer-proxy 或生成包装节点
  v
proxies  (含包装节点，建议排在源节点后或单独分区)
  v
proxy-groups  (不变：仍按 include_entries 解析名字)
```

关键约束：

1. **包装节点名**不得撞用户节点名；使用保留前缀 `__chain/`。
2. **确定性**：同输入 → 同包装名（路径编码或稳定 hash）。
3. **校验**：
   - hop 名必须存在于最终 proxies 名或 node_groups 名（生成期）。
   - 禁止落地节点 dialer 指向自己。
   - 禁止链环：`Exit → A → … → Exit`。
   - hop 不能是 `DIRECT`/`REJECT`/`PASS`（无意义或危险）——可配置白名单例外。
4. **开关**：生成 switches 可加 `proxy_chains: true`（默认 true），关闭时忽略所有链字段。

Script.js 路径：若 `exclude_node_proxies`，链式只影响本系统生成的 groups 时需单独说明；P0 以 YAML 订阅为主。

---

## 6. API / Schema

### SubscriptionRead / Update / Create

```json
{
  "proxy_chain": ["香港", "日本中转"],
  "node_proxy_chains": {
    "美区落地-1": ["香港入口"],
    "直连观察节点": []
  }
}
```

### 校验接口（可选 P1）

`POST /api/subscriptions/{id}/proxy-chain/validate`  
返回：未知 hop、环、自引用、将生成的包装节点列表。

### 不在 P0

- 全局「链模板」库
- 按正则给节点批量套链（可用节点级映射 + 前端批量操作代替）

---

## 7. UI

### 订阅表单

- 区块：**默认代理链**
- 有序列表：添加 hop（下拉：策略组名 + 当前所有最终节点名）
- 上下移动 / 删除
- 说明：`本订阅节点默认：本机 → hop1 → … → 节点 → 目标`

### 节点预览 / 改名旁

- 每节点可「覆盖链」：跟随订阅 / 自定义 / 无链
- 自定义时与订阅级同一套 hop 编辑器

### 生成预览

- 显示 effective dialer 与包装节点数
- 不自动跑 TCP 探活（链通不通是另一回事）

---

## 8. 测试（harness 先于 UI）

1. 无链：生成结果无 `dialer-proxy` 字段变化。
2. 订阅级单跳：该订阅所有节点 `dialer-proxy == hop`。
3. 节点级覆盖：仅该节点不同；`[]` 覆盖掉订阅链。
4. 未知 hop：保存或生成时报 400 + 中文 detail。
5. 自引用 / 环：拒绝。
6. 多跳（P1）：包装节点名稳定；落地 dialer 指向链尾。
7. 改名：`node_proxy_chains` key 随最终名迁移（与 rename 同一事务或保存时重写）。

---

## 9. 明确不做

| 不做 | 原因 |
|------|------|
| 把 relay 当主存储 | 与 dialer-proxy 双轨，UI/生成分叉 |
| 自动测「整链延迟」 | 已拆 mihomo；可用性另论 |
| 链上再嵌套 proxy_chain 递归 | 复杂度与环爆炸 |
| 按机场协议自动猜入口 | 不可靠，违背「手动设置」 |
| 在规则里写链式 | 类型错误 |

---

## 10. 实现切片（验收边界）

1. **模型 + 迁移 + schema** — 字段可读写，旧数据默认空  
2. **生成 P0 单跳** — harness 全绿  
3. **订阅 UI 默认链**  
4. **节点级覆盖 UI**  
5. **P1 多跳包装节点**  
6. **文档 + roadmap 勾选**

每片：`pytest` 绿 + 一次 compose rebuild（生产习惯）。

---

## 11. 配置示例（用户心智）

```yaml
# 生成结果示意（P0 单跳）
proxies:
  - name: 香港入口
    type: ss
    ...
  - name: 美国-1
    type: vmess
    ...
    dialer-proxy: 香港入口   # 来自订阅默认链或节点覆盖
```

```text
订阅「落地机」.proxy_chain = ["香港"]
订阅「落地机」.node_proxy_chains = {
  "特殊节点": ["日本"],      # 覆盖
  "不要链的节点": []         # 强制直连拨号
}
```

---

## 12. 回滚

```bash
cd /home/service/clash-sub-parser
git checkout pre-proxy-chain-design
docker compose up -d --build
```

Tag 对象：`d0988c4`（含 TCP 探活、统一 exclude_group_nodes 等当前稳定线）。
