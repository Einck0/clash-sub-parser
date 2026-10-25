# DNS 页面重设计验收终态（2026-10-25）

身份：`pi_run_20261025_123522_590576 / change_pi_run_20261025_123522_590576 / whole_task / epoch 1`。

## 完成与授权

已完成 [dns-ui-redesign.md](dns-ui-redesign.md) FR-DU1..DU5 的开发、dev 推送、同 SHA 云测、隔离成品/API导出验证、独立工程审查和实际桌面/移动视口验收。未生产部署、未写生产 DNS 配置、未改宿主 DNS，未执行真实上游连通性探测。此次高级配置是客户端导出参数编辑，不宣称参数组合已在 Mihomo 真 DNS 网络中通过验证。

最终业务 SHA：`cdc60ad859ec66250b24ccd61a3dd21a8386fd9d`（dev）。后续仅本验收记录和任务勾选的文档提交不改变已验证业务源码，不把文档提交冒充另一次镜像验证。

## 交付行为

- 基础首屏主解析服务器、增强模式、IPv6；高级配置按连接/引导、Fake-IP/缓存、回退/域名策略、Hosts 折叠组织，不重复大标题横幅。
- 新增已有后端支持的 prefer-h3、direct-nameserver-follow-policy、proxy-server-nameserver-policy 可视编辑；字段中英文业务说明和依赖/冲突建议，不静默改参数。
- 未指定/true/false 区分、删除字段重置、自定义枚举与未知嵌套键保全、JSON 对象严格类型检查、折叠保留无效输入。
- 服务端规范化 YAML 只读草稿预览、编辑后失效，说明不保留注释且不代表连通性验证；可视/Raw 单草稿，CAS409/422保留，忙态锁定和过期响应防护，默认来源不伪称已保存。
- 最新移动工具栏支持自然分行与不拆行开关标签，保留原应用主题与导航，无全局改造/新依赖/API/迁移。

## 云端链路与失败保留

Actions [40167504421](https://github.com/Einck0/clash-sub-parser/actions/runs/40167504421) 对应最终业务 SHA：

- Checks `121091462631` SUCCESS：web type-check、52文件/515测试、web build、Go vet/全套测试、静态二进制及轻量交付契约。
- Runtime `121092160763` SUCCESS；Prelease `121092247242` SKIPPED，符合 dev 不发 Release 契约。
- 镜像 `ghcr.io/einck0/clash-sub-parser@sha256:99044cce2c0bb8bdbc74650d429642765e927889fd2a08414926834ca811ef3a`，OCI revision 与最终业务 SHA 相同。
- 二进制 SHA256 `490987eb8dddfadf00065bf7f2347f799b7b3944068d9872c0d7e4de60d98bed`。
- 云日志 `/tmp/csp-dns-590576/cloud-final/`；宿主及本地 Docker 未执行测试编译。

保留失败：40163880057 的 vue-tsc exit2 为测试夹具推断过窄；40164174211 npm test exit1 两断言将恢复原配置误当脏修改。修复真实类型/测试场景，不弱化 !dirty 或生产参数校验；e2a02fd 的40164379120先全绿。主脑亲读移动截图发现工具栏挤压，未照收最初视觉PASS，定向修复 cdc60ad 并重新完成全部云测及独审、视觉复验。

## 自测、独审、实际成品

当前集成施工自测 PASS：`81bbdf06-a85a-4cb7-975e-2f5a6b0c2e99`，对应当前 SHA 并链接移动整改的 BLOCKED `c6dc9754-54a1-4abb-b444-9eb42fb70a85`；前序 A/B 自测及失败/挂起链保留真实会话。

独立 reviewer 最终 PASS：`rep_rev_whole_task_37f246c1-3349-4bc9-80cb-70e6cd8774a2`，链接原 `rep_rev_whole_task_83656a19-19cc-4fc5-82b4-0c2e08e5fa61`。修正早期报告对 load 双重版本防护的过度泛化：load 仅 generation，页面 busy 锁定输入；preview/switch/save 对修改版本额外防护。审查工件 `/tmp/csp-dns-590576/review-final/`。

Critic 最终 PASSED：`rep_crit_whole_task_20261025_141900_d3a7c682-6420-4c64-b9ba-237cf858ec17`，链接首次 `rep_crit_whole_task_20261025_135500_7c9f81a2`。实际浏览器桌面1440、移动392及窄360视口、中英文、亮暗主题；当前版本编辑→预览→编辑失效→保存→API回读/实际clash导出一致。主脑另亲读最新桌面/392中文/360英文/实际滚动底部截图接受。最终宽度1440/392/360均无文档横向溢出；392底部保存按钮top666/bottom698，在872视口内且不被导航遮挡。开关文字保持单行。不是物理手机/触摸屏验收。

复杂 JSON 非法值/折叠保留、未知字段/高级项删除/Raw往返、真实双客户端409/取消重载、脏token门禁，先前e2a02fd成品已真实验证；最终修订未变化的这些功能采用差量复验，不冒称全部重新执行。当前版本全部高级面板未逐项重新量宽。纯合成DB/API默认revision0→纯validate不写→PUT→回读→clash导出、enable=false保全、422拒绝和CAS409均有证据；配置最终由critic CAS恢复合成基线revision16，与生产修订无关。

## 工件当前散列

| 工件 | SHA256 |
|---|---|
| `/tmp/csp-dns-590576/critic-final/evidence.json` | `9deb103c66000b225df19e5748e8d7d08194143c6c99b3952b42351f7c7e15ae` |
| `/tmp/csp-dns-590576/critic-final/03-mobile-392-dark-top.png` | `c167a74a8b27d4bfc7d43df7fefc836f64f99514f5569babf673aa45602abd45` |
| `/tmp/csp-dns-590576/critic-final/07-narrow-360-english-top.png` | `5e22456cb42b8084b1b7f8c7495182a87ef42388e3341d93b71b3c423b2d2246` |
| `/tmp/csp-dns-590576/cleanup.json` | `b9a9eddd1a340a6d27d45d25d0a3f8f3693331e03c55af54322a25fda341a562` |
| `/tmp/csp-dns-590576/runtime/preview-instance.json`（closed） | `0d44f33bec3a84d0bdd33c51ac1b9df746a7c47114366b48d0b1cdbcdb48dd97` |

首次critic截图被复用脚本覆盖导致报告hash与早期evidence.json不同，不使用那些过期hash作为最终凭证；以上是主脑最终实际计算值。

## 回收、保护现场与局限

准备者核对最新容器4e0a41f6…、socat612151、38180端口、专属network/volume归属后以清理脚本exit0回收，资源均不存在、端口关闭；生产容器ID/镜像/挂载前后一致。仅保留审计证据，URL不再在线。cleanup最终施工报告 `3f6752a2-c47d-4a79-945a-f82d7d83b2ca` 的资源inspect/kill/grep exit1是预期“不存在”证明，不是测试失败，也不将其作编译测试PASS依据。

224个非任务初始dirty路径完整保留；最终重算patch与入口逐字节相同，SHA256 `ed73cc6ca9402aa75e2a3e6f8b1abc4a914412088c029a609c278400d5446416`。graphify update . 实际exit127（未安装，无graph输出），没有安装或虚称更新。不更改constitution/用户既有文档改动、不创建Release、不推进prelease/main。

## Spec Kit 最终一致性

当前Feature由官方check-prerequisites确定002-policy，无extensions.yml；本轮沿原规格增量，不重跑覆盖性setup。DU1→D25-02；DU2→D25-01/02；DU3→D25-01/02/04；DU4→D25-01/03/04；DU5→D25-02/04。当前所有需求有实现/验证任务，五项业务需求全覆盖，无无主任务，无CRITICAL/HIGH未解决项，宪章原则不变。D25-05代码/回收/交付记录齐备，QQ送达凭据以实际通知工具返回为准，非预先虚构。
