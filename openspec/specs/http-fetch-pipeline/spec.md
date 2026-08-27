# http-fetch-pipeline Specification

## Purpose
统一的流式 HTTP 抓取管线，作为订阅拉取与资产下载共用的 SSRF 防线。单点实现 redirect 逐跳校验、响应字节限长与重定向深度控制。

## Requirements

### Requirement: 重定向逐跳校验

跟随重定向时，每一跳的目标 URL SHALL 通过与首跳相同的 SSRF 校验（禁止内网/环回/保留地址），校验失败立即终止并报错。

#### Scenario: 重定向到内网地址被拒绝
- **WHEN** 目标 URL 响应 302 指向 `http://127.0.0.1/` 或其他私网地址
- **THEN** 管线终止请求并返回明确错误，不发起对内网的任何请求

#### Scenario: 重定向缺失 Location
- **WHEN** 上游返回 3xx 但无 Location 头
- **THEN** 管线行为确定（统一为报错终止），两条调用路径表现一致

### Requirement: 响应字节限长

管线 SHALL 在读取流式响应时强制字节上限；超限时中止连接并报错，不会将超限内容部分写入下游。

#### Scenario: 超大文件截断
- **WHEN** 上游返回超过配置上限（如 64MB）的响应体
- **THEN** 下载在达到上限时中止并报「超出大小限制」类错误

### Requirement: 重定向深度上限

管线 SHALL 限制最大重定向次数，超限报错；次数由调用方指定。

#### Scenario: 重定向循环
- **WHEN** 上游构造 A→B→A 重定向循环且超过 max_redirects
- **THEN** 报「重定向次数超限」错误，不无限循环

### Requirement: 双路径行为一致

订阅拉取与资产下载 SHALL 共享同一管线实现，两者在相同输入下的校验行为与错误语义完全一致。

#### Scenario: 相同恶意 URL 两路径同拒
- **WHEN** 同一指向内网重定向链分别经订阅拉取与资产下载触发
- **THEN** 两者均以相同错误类别失败
