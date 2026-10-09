# Auth Capability Specification

## Purpose

本规范定义 Clash 订阅解析器（clash-sub-parser）的控制面与数据面鉴权架构、凭据生命周期与安全存储模型。系统必须支持管理端与导出链接双开关的物理独立解耦，维护全局统一的系统主令牌（Master Token），遵照用户明确要求执行高透明度的明文持久化，并明确安全隔离与凭据生命周期控制。

## User Scenarios & Priority

### User Story 1 - 管理端与导出链接解耦独立控制 (Priority: P1)
- **GIVEN** 管理员希望内网直接免密访问控制台，但对外的导出链接需要凭密码受控拉取
- **WHEN** 管理员在系统设置中关闭管理端鉴权、开启导出鉴权
- **THEN** 系统立即可靠生效：管理后台免密直接进入，而导出链接强制要求提供令牌

### User Story 2 - 统一系统主令牌共享配置 (Priority: P1)
- **GIVEN** 管理员设置了一个全局系统密码 "MySecret123"
- **WHEN** 管理员使用该密码登录控制台，同时客户端使用该密码拉取任意受控发布
- **THEN** 两处校验均基于同一主令牌通过，无需为各发布项分别维护繁琐的专属密钥

### User Story 3 - 明文配置透明度与安全审计 (Priority: P2)
- **GIVEN** 用户要求敏感字段明文可读、拒绝过度封装加密库
- **WHEN** 查看数据库或导出配置时
- **THEN** 字段以明文呈现便于快速排障，同时系统在文档与日志中记录安全隔离建议

---

## Requirements

### Requirement: 管理端与导出链接独立鉴权开关 [FR-AUTH-01]
系统 SHALL 在 `settings` 表中持久化维护两个物理独立的布尔开关：`admin_auth_enabled`（控制管理端 API 与界面是否要求鉴权）与 `export_auth_enabled`（控制 `/publish/v1/*` 导出端点是否要求鉴权）。

#### Scenario: 管理端免密且导出受控
- **WHEN** admin_auth_enabled=false 且 export_auth_enabled=true
- **THEN** `/api/v1/*` 管理接口无条件放行，`/publish/v1/*` 端点对未授权请求返回 401

### Requirement: 统一系统主令牌配置与共享 [FR-AUTH-02]
系统 SHALL 支持配置统一的系统主令牌（System Master Token）。该令牌 MUST 同时作为管理控制台登录的认证密钥，以及各导出端点通用的超级访问凭据。

#### Scenario: 统一令牌同时校验管理与导出
- **WHEN** 客户端以统一主令牌发送管理端登录请求，以及拉取已发布的订阅
- **THEN** 两类请求均通过身份鉴权并获得对应的授权上下文

### Requirement: 明文敏感信息存储模型与安全警示 [FR-AUTH-03]
遵照用户明确要求，系统 SHALL 以明文形式存储订阅源 URL、用户凭据与节点连接参数，严禁引入不可透明审查的加密 Vault。系统 MUST 在安全说明中明确指出：该部署仅适用于受信任网络环境或经由反向代理实施传输层保护。

#### Scenario: 数据库明文可读且无需密钥解密
- **WHEN** 管理员在排障时直接检视 SQLite 中的订阅与节点表
- **THEN** 关键连接参数以明文字符串呈现，无需复杂的第三方密钥拆解工具

### Requirement: 控制面凭据生命周期与会话 Cookie [FR-AUTH-04]
管理端鉴权 SHALL 支持 Authorization Bearer 头部与安全 HTTP-Only Session Cookie 两种认证方式。当用户在前端退出登录时，系统 MUST 清除会话 Cookie 并销毁上下文凭据。

#### Scenario: 登出操作彻底销毁会话
- **WHEN** 管理员点击控制台顶部的退出登录按钮
- **THEN** 浏览器内的认证 Cookie 被清除，后续未经授权的 API 请求返回 401

---

## Success Criteria

### Measurable Outcomes
- **[SC-AUTH-01]** 双鉴权开关在 4 种组合状态下的模式判定准确率达到 100%。
- **[SC-AUTH-02]** 统一系统令牌在管理登录与导出端点的鉴权互通率达到 100%。
- **[SC-AUTH-03]** 会话登出后未授权 API 拦截率达到 100%。
- **[SC-AUTH-04]** 敏感参数持久化读写在零外部加解密依赖下保持 100% 稳定性。
