from __future__ import annotations

from typing import Literal
from pydantic import BaseModel, Field

from app.schemas.logical_config import LogicalConfigurationBundle


class BundlePreflightBlocker(BaseModel):
    """预检阻断项表达"""

    code: str
    message: str
    location: str | None = None
    level: Literal["blocker", "warning"] = "blocker"


class BundlePreflightResult(BaseModel):
    """配置 Bundle 导入非破坏性预检结果"""

    valid: bool
    schema_version: int
    migration_required: bool = False
    blockers: list[BundlePreflightBlocker] = Field(default_factory=list)
    warnings: list[str] = Field(default_factory=list)


class BundleExportEnvelope(BaseModel):
    """配置 Bundle 导出打包封套"""

    schema_version: int = 1
    checksum: str
    exported_at: str
    bundle: LogicalConfigurationBundle


class RevisionDTO(BaseModel):
    """配置版本元数据表达"""

    logical_id: str
    schema_version: int
    checksum: str
    is_active: bool
    created_at: str
    created_by: str
    group_count: int = 0
    rule_count: int = 0
