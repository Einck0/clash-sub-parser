from __future__ import annotations

from datetime import datetime, timezone
from typing import Any, Literal
from pydantic import BaseModel, Field


class MembershipEntry(BaseModel):
    """策略组有序成员选择表达式"""

    entry_type: Literal["node_name", "regex", "group_ref", "source_ref", "exclude_node", "exclude_group"]
    value: str
    order_idx: int = 0


class LogicalGroupConfig(BaseModel):
    """逻辑策略组配置定义，以 logical_id 为唯一主键"""

    logical_id: str
    name: str
    group_type: Literal["select", "url-test", "fallback", "load-balance"] = "select"
    sort_order: int = 0
    add_fallback: bool = False
    include_entries: list[MembershipEntry] = Field(default_factory=list)
    exclude_nodes: list[str] = Field(default_factory=list)
    filter_min_speed_mbps: float | None = None
    filter_media_unlock: list[str] = Field(default_factory=list)
    url_test_config: dict[str, Any] | None = None
    load_balance_config: dict[str, Any] | None = None
    fallback_config: dict[str, Any] | None = None


class LogicalRuleCategory(BaseModel):
    """逻辑规则分类定义"""

    logical_id: str
    name: str
    sort_order: int = 0
    enabled: bool = True


class LogicalRuleConfig(BaseModel):
    """逻辑分流规则定义"""

    logical_id: str
    category_logical_id: str | None = None
    rule_type: str = "DOMAIN-SUFFIX"
    payload: str
    target_group_logical_id: str | None = None
    target_direct_or_reject: str | None = None
    sort_order: int = 0
    enabled: bool = True


class LogicalDnsConfig(BaseModel):
    """逻辑 DNS 设置"""

    raw_yaml: str = ""
    enabled: bool = True


class LogicalGenerateSettings(BaseModel):
    """逻辑生成设置"""

    switches: dict[str, Any] = Field(default_factory=dict)


class LogicalConfigurationBundle(BaseModel):
    """全量逻辑配置 Bundle，包含策略组、分流规则、DNS 与生成配置"""

    schema_version: int = 1
    logical_id: str
    name: str = "default"
    groups: list[LogicalGroupConfig] = Field(default_factory=list)
    rule_categories: list[LogicalRuleCategory] = Field(default_factory=list)
    rules: list[LogicalRuleConfig] = Field(default_factory=list)
    dns: LogicalDnsConfig = Field(default_factory=LogicalDnsConfig)
    generate: LogicalGenerateSettings = Field(default_factory=LogicalGenerateSettings)
    created_at: str = Field(
        default_factory=lambda: datetime.now(timezone.utc).isoformat(timespec="seconds").replace("+00:00", "Z")
    )


class CompilationDiagnostic(BaseModel):
    """编译过程诊断信息"""

    level: Literal["info", "warning", "error"]
    code: str
    message: str
    location: str | None = None


class CompiledGroup(BaseModel):
    """编译后展开完成的策略组"""

    name: str
    group_type: str
    resolved_node_names: list[str]
    resolved_node_ids: list[str] = Field(default_factory=list)
    raw_config: dict[str, Any] = Field(default_factory=dict)


class CompilationResult(BaseModel):
    """纯函数编译输出结果"""

    success: bool
    semantic_fingerprint: str
    groups: list[CompiledGroup] = Field(default_factory=list)
    rules: list[dict[str, Any]] = Field(default_factory=list)
    diagnostics: list[CompilationDiagnostic] = Field(default_factory=list)
    rendered_output: str | None = None
