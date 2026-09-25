# CSP Clean-Slate 重构设计核心要求（Einck 最新权威指示）

## 核心指导思想
1. **彻底推平重构**：代码彻底推平，数据库模型彻底重新设计，存量 17MB 数据后续走独立离线迁移管道导入。
2. **前端架构终极定版：全面采用 Zashboard（Zephyruso/zashboard）**：
   - **官方标杆**：`https://github.com/Zephyruso/zashboard`
   - **技术栈**：Vue 3 + Vite + TypeScript + Tailwind CSS + **DaisyUI** + @heroicons/vue + @tanstack/vue-virtual。
   - **彻底废除生硬死板限制**：严禁搞机械的 44px 强制死板几何断言或繁杂束缚。直接参考并采用 Zashboard 的成熟交互体系与布局规范！
   - **手机版核心体验（对标 Zashboard）**：
     - 自然流动的移动端响应式布局，利用 DaisyUI 的自适应设计；
     - 卡片点击原地生长放大展开（平滑 200ms ease-out 动效 + 40% 柔和毛玻璃遮罩）；
     - 虚拟滚动（@tanstack/vue-virtual）保障海量节点丝滑不卡顿；
     - 原生丰富主题体系（DaisyUI 官方主题，支持 light, dark, dim, cyberpunk, synthwave 等一键无缝切换）；
     - 布局自然、交互符合手机真实操作直觉，拒绝过度工程化死板限制。
3. **后端架构与数据模型**：
   - Go 1.22+ Clean Architecture，单二进制 embed.FS 打包内嵌前端；
   - 全新设计的 SQLite Schema（强实体、规范外键索引）；
   - 纯内存 sing-box 探针管道、滑动窗口并发工作池、五目标导出编译。
