# Implementation tasks: modern CSP frontend and delivery increment

本文件记录当前范围与剩余事项。最新结果为 **应用完成并已发布预发布；整体受 G04 的 GitHub 删除权限阻断**，不是全部完成。历史 migration、Q6/千万级未决不在本轮验收声明内。

## Planning
- [x] M00 主脑修订 spec/plan/modern contract；官方 Tailwind/Daisy 兼容性调查、模块接口与业务场景。
- [x] G00 基于官方与仓库证据冻结 github-delivery-contract.md，区分 Release/tag/branch/GHCR 与远程 pull；保生产及历史授权边界。

## Frontend scope
- [x] M01 [FR-M01/02/10] Foundation/style/theme/App/router/ui/guard/locale；7 主题、旧 hash、lazy route、移动入口与 More。
- [x] M02 [FR-M03] Nodes 紧凑视图、真实 health/task/geo/platform、URL filters、稳定 ID、隐藏选择范围与安全取消。
- [x] M03 [FR-M04] Policy 单 draft、preview 拆分，保有序来源/generation/CAS/dirty；实际保存、GET/重开/导出与真实 409 通过。
- [x] M04 [FR-M05] Rules 分类与响应式维护；实际 UI 新增/删除，470 规则/14 分类恢复、MATCH 最后。
- [x] M05 [FR-M06] Subscriptions 摘要/basic/advanced 单 draft；实际保存与重开，保 primary、命名及 probe_policy。
- [x] M06 [FR-M07] DNS 分组/单 draft/mode/离开保护；17 已知键及 unknown nested、outer authority、真实 CAS/回读/导出通过。
- [x] M07 [FR-M08/10] Publications/settings 四直链、真实下载结果与错误、分类设置与凭据清理逃生入口。
- [x] M08 [FR-M09] Dashboard 紧凑动线，loading/error/zero 区分，500 夹具不得变成假零。

## GitHub/deployment/privacy scope
- [x] G01 [FR-G01/02/06] dev checks+SHA 镜像不 Release；真实 prelease 分支完整云测后滚动单渠道发布；main 不自动 stable；guard/串行/三资产校验、取消 Release-based reuse。
- [x] G02 [FR-G04/06] image-only Compose、必填 CSP_IMAGE、pull_policy always、失败停止及受控 pull 后身份核验；公开手册更新。未执行生产部署。
- [x] G03 [FR-G05/06] 精确 manifest/ignore/index 止损，本地资料保全；dev/Docker context 排除内部资料，保业务 spec 和 unrelated dirty。
- [ ] G04 [FR-G02/03] **部分完成且阻断**：已 non-force 创建 prelease 至 3d42f94，Actions 39785295934 三 jobs 成功，固定 prelease Release 三资产/镜像 alias 已核验。17 旧 Release 的 51 资产已仓外持久保存并全部校验；精确删除前重新核远端与资产。首个 DELETE 被 GitHub 403 `Resource not accessible by personal access token` 拒绝，**0 个删除，当前共 18 个 Release**。需仓库 Release 删除权限后继续；不得声称只剩一个。保历史 tag/commit、GHCR 与生产/回滚依赖。
- [x] G05 [FR-G05] main 独立 privacy-only 非强推提交 fbcbbd3，未合入前端；dev/main 当前树停止追踪内部资料。本地文件保留，历史仍暴露，不是全历史 secret audit，无清史/轮换授权。

## Integration and actual gates
- [x] M09 逐文件开发提交并推 dev；3d42f94 对应 Actions 39763598054 checks/image 成功，dev promote skipped 正确。51 前端文件/495 tests、Go 与 37+27 交付契约通过。prelease 同 SHA Actions 39785295934 checks/image/promote 全成功。
- [x] M09-R01 既有 npm/type/Vitest/合成/文案契约失败及复验链保留；最终云测已通过，未用本机测试替代。
- [x] M10 同 SHA digest/OCI revision/binary 核验；合成 DB/internal network/禁 scheduler/loopback 预览、实际保存/回读/导出/409/乱序与错误路径有工件。
- [x] M11 当前源码/云 jobs/工件与关键风险差量独审已通过；发布/清理终态独审仍须明确外部 G04 阻断，不伪造整体 PASS。
- [x] M12 Critic 八页 desktop/mobile/light-dark 与实际保存/导出 PNG 检视通过；移动浏览器视口，不声称实体手机实测。
- [ ] M13 owned preview/中继/合成卷和临时凭据已回收，审计保留；待最终独审及规格交付。**G04 权限阻断不因收尾而变完成**。

## Evidence and limits

应用源码与预发布对应 3d42f94；公开 Actions 运行 39763598054（dev）与 39785295934（prelease）。详细内部验收凭据、运行记录与受限恢复材料不纳入公共仓库。当前树止损不能消除历史披露；G04 权限阻断不等于已完成清理。
