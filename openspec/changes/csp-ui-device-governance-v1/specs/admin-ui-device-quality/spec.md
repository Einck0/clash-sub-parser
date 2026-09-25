## Purpose

规定 CSP 管理界面在隔离浏览器中对发布预览、国际化、失败恢复和策略交互的可观察行为及安全验收边界，使已报告的错误可复现、修复并跨移动与桌面视口复验，同时不将 headless 浏览器结果冒充实屏体验。

## ADDED Requirements

### Requirement: 发布预览和模板生成可预测
管理界面 SHALL 在合法输入下获得 `POST /api/v1/publications/preview` 的有效预览结果；模板生成 SHALL 明确区分合法输入、边界输入与无效输入，无效输入或后端失败 MUST NOT 变为未说明的 500 或虚假的成功状态。前后端 SHALL 按一致的错误语义展示可理解反馈。

#### Scenario: 合法与边界输入
- **WHEN** 用户在隔离实例上提交合法发布预览或模板生成输入（包括明确支持的边界值）
- **THEN** 服务 SHALL 返回契约定义的有效结果，界面 SHALL 展示对应预览且不报告失败

#### Scenario: 无效模板或预览输入
- **WHEN** 用户提交缺失、格式错误或不受支持的模板生成/预览输入
- **THEN** 服务 SHALL 返回可解释的非成功结果，界面 SHALL 展示对应错误与恢复动作，MUST NOT 因可预期输入错误返回未说明的 500

### Requirement: 全路由完整双语与失败恢复
管理界面 SHALL 在 dashboard、subscriptions、nodes、probes、policy、publications、settings 七个入口提供完整中英文用户文案，MUST NOT 显示裸 i18n key。可恢复的请求异常或空状态 SHALL 呈现与状态相符的引导和可操作的重试或返回路径，重试后 SHALL 使用真实服务端结果，MUST NOT 伪报成功。

#### Scenario: 切换中英文
- **WHEN** 用户分别以中文和英文浏览七个入口、异常提示及抽屉
- **THEN** 页面 SHALL 显示自然语言文本而非裸 key，且不出现未处理的运行时错误

#### Scenario: 失败后重试
- **WHEN** 视图请求返回 401、403、409、500 或网络错误，随后用户触发可用的恢复/重试动作
- **THEN** 页面 SHALL 呈现可理解的失败说明与适用的下一步，并按新的真实响应更新状态；鉴权失败 SHALL 引导鉴权恢复，失败的写操作 MUST NOT 显示成功

### Requirement: PolicyView 拓扑与抽屉一致
PolicyView SHALL 将拓扑节点的选择与相应详情/编辑抽屉联动；抽屉保存或取消后拓扑、详情与服务端确认的状态 SHALL 保持一致，失败 MUST NOT 静默丢失用户可见状态。

#### Scenario: 选中拓扑节点并编辑
- **WHEN** 用户在策略拓扑中选择节点、打开抽屉并保存有效更改或取消编辑
- **THEN** 抽屉 SHALL 对应选中节点；保存成功后拓扑和详情 SHALL 呈现已确认的新状态，取消 SHALL 不修改已保存状态

### Requirement: 视口可触达与证据边界
界面 SHALL 在 392×872、375×667 和 1280×800 视口保证关键操作可辨识且可通过正常滚动、键盘与指针到达；交互区域 MUST 有可访问名称、可见焦点及可可靠激活的范围，相邻操作 MUST 可区分而不发生误触重叠，不以统一像素尺寸作为充分或必要门禁。底部 Dock MUST NOT 遮挡最后的关键内容或操作，抽屉 MUST NOT 产生不可恢复的横向溢出，其关闭及提交操作 MUST 可到达。自动化验收 SHALL 经唯一授权的 SSH→Ubuntu chroot ARM64 Chromium Playwright 路径执行，并 MUST 明确区分 headless 视口证据与实屏触控证据；设备 headless 结果 MUST NOT 声称实屏通过。生产 18080/17000 与数据卷 MUST NOT 接收测试写入。

#### Scenario: 跨视口浏览与抽屉
- **WHEN** 浏览器在上述三个视口依次打开七入口、滚动到底部并打开 PolicyView 或其他编辑抽屉，尝试焦点导航与关键操作
- **THEN** 关键控件 SHALL 能辨识、聚焦并可靠激活，邻近目标不会重叠误触，底 Dock 不遮挡末项，抽屉关闭与主操作可达、无不可恢复的横向溢出，主路由无裸 key 或未处理错误；验收 SHALL 记录实际可见性、遮挡、滚动及操作结果，而非只断言统一尺寸

#### Scenario: headless 取证报告
- **WHEN** 使用设备 Ubuntu chroot 的 Chromium Playwright 完成无实屏的回归
- **THEN** 报告 SHALL 标明 headless 模式、视口、构建与脱敏错误证据，MUST NOT 将其描述为实屏触控、软键盘或系统安全区域验收
