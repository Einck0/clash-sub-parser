## Why

当前用户明确要求实际修复 CSP 管理界面故障，而旧合同将 ADB/USB 真机连接设为前置门槛，已被废除。已报告 `POST /api/v1/publications/preview` 返回 500、模板生成边界不清、`web/src/locales/messages.ts` 缺失 130+ 中英文键、异常状态缺乏可操作重试以及 PolicyView 拓扑与抽屉不联动。需要保留已经完成的隔离取证工作，立即把剩余施工和复验转向这些可验证缺陷。

## What Changes

- 修复发布预览 500 并明确模板生成的合法输入、无效输入及错误响应边界；失败不可伪装为成功，服务端与前端按同一接口契约联调。
- 补齐管理界面实际使用的 i18n 键的中英文翻译，七个入口不显示裸键；异常/空状态给出明确恢复或重试动作。
- 修复 PolicyView 拓扑选中、抽屉编辑和数据刷新之间的状态联动，触控下保持可操作。
- 使用唯一授权的设备执行路径 `ssh xiaomi-phone "/data/local/einck/script/ubuntu_chroot.sh run '<cmd>'"`，在 Ubuntu chroot ARM64 的 `/root/.cache/ms-playwright` Chromium Playwright 做浏览器验收；headless 证据只称设备上 headless Chromium，不称实屏或物理触控验收。覆盖移动 392×872、375×667 与桌面 1280×800。
- 保护已有 726 项工作区变更；仅隔离实例/临时数据可用于写测试，生产 18080/17000 和数据卷禁止写入；不授权部署、重启或清场。

## Capabilities

### New Capabilities

- `admin-ui-device-quality`: CSP 管理界面的发布预览、国际化、失败恢复、策略联动及跨视口浏览器验收契约。

### Modified Capabilities

- 无。与其他进行中 Change 的 API/鉴权契约冲突时先核对归属，不擅自更改其工件。

## Impact

- 预期后端发布预览/模板生成路径及相关契约测试、`web/src/locales/messages.ts`、异常状态组件与 PolicyView，以及定向 Playwright/单元或集成回归。具体文件归属由施工者先查现有实现确定，重叠接口串行处理。
- 已完成的基线、隔离服务与历史故障复核四项保留；它们不是当前修复已经完成的证明。
- 不对共享服务、数据卷或现有构建产物进行测试写入；非本次授权的实屏、ADB、USB 或设备唤屏操作不纳入任务。
