## Why

旧 CSP 已被物理清空，且旧 Python、旧前端与旧数据模型均不再是可维护基线。保留的生产 SQLite 含有用户业务内容，但将其作为新运行时数据库或兼容旧表会把历史架构重新引入产品，因此需要在明确数据边界下重建单一、可验证的控制平面。

## What Changes

- **BREAKING** 新建 CSP 1.0 产品边界：Go 1.22+ 单二进制、内嵌管理界面、新 SQLite schema 与新的 HTTP API；不保留旧应用代码、路由、序列化形状、页面状态或运行时适配层
- **BREAKING** 弃用旧的 `/yaml`、`/script`、旧 `/api/*` 和旧输出形状。新发布仅支持带版本前缀的受控导出契约；旧 URL 必须返回明确的已弃用错误，不能悄然映射
- 新增订阅采集、规范化节点台账、可取消且受配额限制的 sing-box 内存探测、证据化能力观察、服务端分页查询和递归策略树解析
- 新增以一个规范化解析结果为唯一输入的多客户端配置编译器，支持 Clash、Mihomo、sing-box、Surge、Quantumult X 五种目标
- 新增采用成熟开源标杆 Zephyruso/zashboard 体系的管理界面：基于 Vue 3 + Vite + Tailwind CSS + DaisyUI，原生多主题平滑切换，TanStack Virtual 虚拟滚动，移动端采用 Zashboard 经过实战检验的流畅弹性交互，不搞死板教条限制
- 新增独立、离线、一次性的 legacy SQLite 导入工具：仅导入经 allowlist 和校验通过的非秘密逻辑内容到新 schema 的待审草稿；不自动激活、不双写、不影子读、不原位改写历史库
- 新增部署、备份、恢复、切流与回滚契约：新服务使用独立新卷，旧停止的容器及历史卷只用于人工批准的回滚或离线导入

## Capabilities

### New Capabilities
- `control-plane-boundary`: 新 CSP 1.0 的版本化管理 API、鉴权、错误和废弃端点拒绝行为
- `subscription-inventory`: 订阅采集、节点逻辑身份、差集收敛、服务端分页节点台账与筛选
- `probe-evidence-engine`: 可取消、限流、幂等的 sing-box 内存探测作业及可审计结果证据
- `policy-tree-and-compiler`: 递归策略树单一解析器及五目标配置编译与发布
- `clean-slate-import-and-operations`: 新 schema、离线历史导入隔离、备份恢复、切流回滚及秘密边界
- `admin-workbench-experience`: 双模态、无障碍、服务端分页和移动端可用的管理工作台

### Modified Capabilities
- 无。本仓库的旧 OpenSpec 主规格随旧源码被移除；本变更只定义 CSP 1.0 新能力。

## Impact

- 规划中的实现范围为新建 `cmd/`、`internal/`、`web/`、`migrations/`、`test/` 和部署工件；不得恢复或引用已删除的 Python/FastAPI、旧 Vue 代码或旧表模型
- 运行时依赖为 Go 1.22+、`modernc.org/sqlite`、sing-box Go 模块、Vue 3、Vite、Tailwind、shadcn-vue；Go `embed` 用于同一二进制交付静态界面
- 现有 `clash-sub-parser_backend-data` 卷和其 17 MB 历史数据库不挂载给新服务作数据目录；历史源仅由显式操作的离线导入器只读打开
- 端口 18080 与 17000 的最终绑定、反向代理切流和旧容器销毁属于受控发布操作，不由本计划自动执行
