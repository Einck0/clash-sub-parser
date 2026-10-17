# Architecture Reflection & Research: Eliminating Over-Engineering

## 1. 用户反馈与架构反思 (2026-10-17)

Einck 原话：
> “现在订阅发布完全没有意义啊，改成最简单的形式不就行，每个类型返回对应类型的，他美啊，你现在加什么添加K你完全过于复杂过于工程化了呀，这你你想过这个工程是干嘛用的吗？你这个有什么意义吗，公网访问基准地址 (Public Base URL)又是干嘛的？如果不是这个，这个还能访问吗？我设置其他地址，你也访问不了啊”
> “为什么CSP，要设置成现在这样，比如链接为 https://sub.einck.top/subscribe/v1/01a14780-38e6-7a79-ad38-e1bbc9753c4e?token=sub_c919e3c930fadea5e5e5ffe9d6bbfadfaffb590d97569c18b66aa4b001f62f8b，搞这么复杂，有必要吗？配置是什么时候生成的，还是订阅的时候生成的，为什么不能用最简单的clash，singbox，这种后缀，然后加token，如果设置token的话，这是不是过度设计，”
> “现在做几件事情，第1把现在的架构改了，使用我们设置中设置的Token，我记得那个设置本来就有这个要求，可以保护管理端，也可以保护导出地址，第二，让主老好好TMD想想，我们这个架构是是什么样的，应该怎么设计整个架构，消除过度设计”

### 核心反思与决策

1. **CSP 的真实定位**：
   CSP 是 Einck 的个人私有高性能订阅解析与规则编排控制平面，而不是面向海量租户的商业 SaaS 机场分销系统。
   商业 SaaS 需要为每个外部用户分配链接、限速、统计流量、独立吊销；而个人私有工具的核心需求是**自己能把配好的规则以最简单、最直观的方式导入自己的客户端**。

2. **为什么废除 `subscription_links` 独立 Token 机制**：
   - 现存方案在数据库为每个客户端链接分配 UUID 和随机 64-hex Token，导致直链长达上百字符，肉眼完全无法辨认；
   - 并且带来了前所未有的复杂度：状态机、轮换、吊销、掩码、公网基准地址强制校验等；
   - 系统内部本就有一套统一的 Settings Token（`admin_token` / `TokenHolder`），且原生支持 `adminAuthEnabled` 与 `exportAuthEnabled` 开关；
   - **决策**：直接复用系统 Settings 中的全局统一 Token，保护管理端与导出端点。Open Mode 时免密直接下载；Protected Mode 时带上统一的 Token 即可！

3. **直观路由与现场动态编译**：
   - 配置生成时机：**客户端发起订阅拉取请求时现场动态编译**（Live on-request compilation）；
   - 路由设计：废除 UUIDv7，直接按客户端类型提供端点（`/clash`、`/singbox`、`/surge`、`/qx`，及 `/sub/{target}`）；
   - 用户在客户端直接填 `https://sub.einck.top/clash?token=xxx`，一目了然，极度直观。

4. **前端界面的彻底解放**：
   - 将“客户端订阅”页面彻底去过度设计，直接展示四种客户端卡片或四行，一键复制；
   - 前端根据当前系统的认证状态自动决定是否拼接 `?token=xxx`，无需用户管理任何链接对象。
