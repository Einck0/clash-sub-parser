from __future__ import annotations

import json
from typing import Any

from app.schemas.bundle import BundlePreflightBlocker, BundlePreflightResult
from app.schemas.logical_config import LogicalConfigurationBundle
from app.services.canonical_compiler import CanonicalGraphResolver


def preflight_bundle_json(bundle_dict: dict[str, Any]) -> BundlePreflightResult:
    """非破坏性静态预检配置 Bundle，返回阻断与警告信息"""
    blockers: list[BundlePreflightBlocker] = []
    warnings: list[str] = []

    # 1 校验 Schema 版本
    schema_ver = int(bundle_dict.get("schema_version", 1))
    migration_required = False
    if schema_ver > 1:
        migration_required = True
        warnings.append(f"Bundle Schema 版本为 {schema_ver}，需要迁移器适配")

    # 2 检查敏感认证字段泄露
    serialized = json.dumps(bundle_dict, ensure_ascii=False)
    for forbidden in ("password", "private_key", "secret_key"):
        if f'"{forbidden}"' in serialized:
            blockers.append(
                BundlePreflightBlocker(
                    code="SECRET_LEAK",
                    message=f"配置 Bundle 中严禁包含私有敏感凭据字段 '{forbidden}'",
                    location="bundle_root",
                    level="blocker",
                )
            )

    # 3 解析逻辑 Bundle 模型
    try:
        bundle = LogicalConfigurationBundle.model_validate(bundle_dict)
    except Exception as exc:
        blockers.append(
            BundlePreflightBlocker(
                code="SCHEMA_VALIDATION_ERROR",
                message=f"Bundle 数据结构校验失败: {exc}",
                location="bundle_root",
                level="blocker",
            )
        )
        return BundlePreflightResult(
            valid=False,
            schema_version=schema_ver,
            migration_required=migration_required,
            blockers=blockers,
            warnings=warnings,
        )

    # 4 静态图引用与循环依赖校验
    resolver = CanonicalGraphResolver(bundle=bundle, inventory_nodes=[])
    diagnostics = resolver.validate_graph()

    for diag in diagnostics:
        if diag.level == "error":
            blockers.append(
                BundlePreflightBlocker(
                    code=diag.code,
                    message=diag.message,
                    location=diag.location,
                    level="blocker",
                )
            )
        elif diag.level == "warning":
            warnings.append(f"[{diag.code}] {diag.location}: {diag.message}")

    is_valid = len(blockers) == 0
    return BundlePreflightResult(
        valid=is_valid,
        schema_version=schema_ver,
        migration_required=migration_required,
        blockers=blockers,
        warnings=warnings,
    )
