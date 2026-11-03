# Specification quality checklist

- [x] 用户原话与实施/生产边界明确，CSP/DNS/链不是空入口。
- [x] 当前全链丢失事实、保留能力、正常与失败路径有证据与可验收结果。
- [x] 三类目标/两类跳板/稳定身份、排序/overlap/传递环/CAS/失效行为冻结。
- [x] API、Go共享类型、各模块写集及外部接口事实明确，单轨无兼容胶水。
- [x] CSP独立保存边界、DNS单草稿/17键unknown/outer/409明确。
- [x] 每FR/SC有任务与真实验收入口，云端编译与隔离预览/独审/Critic必选。
- [x] 不留NEEDS CLARIFICATION，无需用户为可取证工程事实决策；未授权事项明确不做。
