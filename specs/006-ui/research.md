# Research: modern frontend increment

当前研究补差量而非重做原方案。参考固定6225aac84439425ae4f4493437b110a314753436；原完整报告94fac686…为历史输入，不作为当前执行授权。

1. 决定升级Tailwind4+DaisyUI5：最新用户确认优先于旧保守依赖方案；官方迁移支持CSS-first/Vite plugin与主题token。参考https://tailwindcss.com/docs/upgrade-guide、https://daisyui.com/docs/upgrade/；存档本轮style-research/tailwindcss-upgrade-guide.txt及daisyui-upgrade-guide.txt。
2. 决定保Vite6/Router4：npm实际@tailwindcss/vite4.3.3 peer含Vite6，当前CI Node22满足>=20；不因参考Vite8强迁。版本选择Tailwind4.3.3、DaisyUI5.7.50，锁文件精确解析与云测作最终凭据，未验证不声称兼容PASS。详情style-migration-report.md/npm-metadata.json。
3. 决定手工迁移而非全仓upgrade工具：保护dirty并按模块改类；旧input-bordered/form-control/label/tabs-boxed及hsl(--p)不以旧语义兼容层长期续命。7主题名/偏好保持。
4. 信息架构/卡片/数据流沿原报告：单Router、4项移动导航、紧凑对象优先，公共组件后域拆分；不把CSP变mihomo实时控制器。依据旧报告与本轮source差量一致，源码前端无初始未提交改动。
5. 原006-ui归属真实前端能力，使用环境显式指定且NO_PERSIST，不覆盖002-policy指针。setup-plan保现plan不覆盖；修改前保存旧spec/plan/tasks到planning-baseline。无extensions.yml钩子。
