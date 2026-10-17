# Implementation Tasks: Minimal Client Subscriptions (Zero-Glue Clean Slate)

- **Feature Directory**: `specs/004-publication/`
- **Run ID**: `pi_run_20261017_134530_2673323`
- **Change**: `004-publication`
- **Work Package**: `default`
- **Epoch**: 1
- **Status**: IMPLEMENTING — 彻底消除过度设计，统一采用 Settings Token

---

## 任务序列与执行计划

- [x] **S401 [ARCH]** 主脑完成架构反思与契约更新：更新 `spec.md`、`plan.md`、`research.md` 与 `tasks.md`，废除独立链接表与 UUID 路由契约，确立统一 Settings Token 鉴权与直观客户端直链（`/clash`、`/singbox`、`/surge`、`/qx`），执行 speckit-analyze。
- [x] **S402 [BACKEND]** 后端实现直观导出路由与 Settings Token 鉴权：
  - 在 `internal/application/publication/service.go` 中提供按 target 现场动态编译接口（`GenerateLiveTargetArtifact`）；
  - 在 `internal/transport/http/router.go` 中注册直观路由（`/clash`、`/mihomo`、`/singbox`、`/surge`、`/qx`，及 `/sub/{target}`）；
  - 编写统一鉴权 handler：集成 `TokenHolder`，支持 Open Mode 免密与 Protected Mode（`?token=...` / `Bearer`）统一验证，支持 ETag 条件请求与错误处理；
  - 编写后端 HTTP 单元测试与边界测试（覆盖免密、带 Token、错误 Token、ETag 304、不同 target 编译等）。
- [x] **S403 [FRONTEND]** 前端推平过度工程化交互：
  - 重构 `web/src/features/publications/PublicationsView.vue` 与 `usePublications.ts`：
    - 彻底移除对 `subscription_links` CRUD 接口的调用与多余状态机；
    - 呈现 4 种客户端（Mihomo/Clash、Sing-box、Surge、Quantumult X）直观直链与一键复制；
    - 自动根据当前登录认证会话中的 Token 拼接 `?token=...`（若系统处于 Open Mode 则不拼接）；
    - 剪贴板受限时支持只读手动复制弹窗；
  - 更新受影响的前端单元测试（Vitest）与类型检查（`vue-tsc`），确保 100% 绿标。
- [x] **S404 [SELF-TEST & PREVIEW]** 施工子机自测与隔离容器成品预览准备：
  - 在隔离容器内执行 Go 测试与前端全量回归；
  - 准备带有真实合成数据的隔离成品预览实例（端口 28081），交付包含标准 `instanceId`、`url`、`testDataBoundary`、`cleanupCommand` 的预览清单。
- [x] **S405 [REVIEWER]** 独立 Reviewer 只读审查：
  - 对照架构反思核对是否彻底消除了过度设计；
  - 验证直观直链路由、Settings Token 统一鉴权、ETag 缓存与前端简化代码。
- [x] **S406 [CRITIC]** Critic 最终黑盒与多模态视觉验收：
  - 真实浏览器访问隔离预览（桌面与窄屏视口）；
  - 验证四端直链展示、一键复制、Token 自动拼接、免密模式表现，查看截图并出具视觉裁决。
- [x] **S407 [DEPLOY]** Git 提交、官方 Dev 云构建与直接上线：
  - 提交确证源码并推送至 `dev` 分支；
  - 监控官方 GitHub Actions 完成构建与镜像发布；
  - 在线热备生产数据库，平滑更新生产容器，核验真实域名 `https://sub.einck.top`。
