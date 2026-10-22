# Frontend state model (no server schema migration)

权威实体继续现API Subscription/NormalizedNode/PolicyGroup+ordered MemberSource/PolicyRule+Category/DNSSettings(revision)/AuthSettings。字段不改；CAS只用于真实支持的组/全局筛选/DNS等端点，不能凭抽象给所有API编revision。

内部状态：remote loading/ready/refreshing/error区分成功空；group/DNS draft与base服务数据分离；preview绑定draft-generation/实体/目标；任务态与健康三态分离。stale结果不能覆盖当前会话或草稿。配置值只内存，prefs仅theme/density/collapse，URL仅非敏感filter/ID。详细transition和失败输出见modern-frontend-contract.md。无新数据库表，无历史业务数据迁移，无自动导入/恢复生产DNS。
