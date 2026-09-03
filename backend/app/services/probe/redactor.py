from __future__ import annotations

import re
from typing import Any

SECRET_KEYS = {
    "password",
    "uuid",
    "token",
    "secret",
    "auth",
    "private_key",
    "public_key",
    "psk",
    "key",
}


def redact_node_data(data: dict[str, Any]) -> dict[str, Any]:
    """脱敏节点敏感认证凭据，保留拓扑与协议参数"""
    redacted = dict(data)
    for k in list(redacted.keys()):
        if k.lower() in SECRET_KEYS:
            redacted[k] = "[REDACTED]"
    return redacted


def sanitize_diagnostic_message(msg: str) -> str:
    """脱敏诊断与异常日志中的密码、Token 与私有凭据"""
    if not msg:
        return ""
    # 遮蔽常见 URL 形式中的密码 user:pass@host
    sanitized = re.sub(r"://([^:@\s]+):([^@\s]+)@", r"://\1:[REDACTED]@", msg)
    # 遮蔽 UUID
    sanitized = re.sub(
        r"[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}",
        "[REDACTED_UUID]",
        sanitized,
    )
    # 遮蔽长 Token 串
    sanitized = re.sub(r"(token|secret|password|key)=([^\s&]+)", r"\1=[REDACTED]", sanitized, flags=re.IGNORECASE)
    return sanitized
