from __future__ import annotations

import os
from typing import Any
from sqlalchemy.ext.asyncio import AsyncSession

from app.repositories.node_repository import NodeRepository
from app.services.canonical_compiler import CanonicalGraphResolver
from app.services.compiler_target_adapters import render_compiled_yaml
from app.services.legacy_config_importer import build_logical_bundle_from_db
from app.services import generate_service


class ShadowCompilerService:
    """影子模式比对服务，比对旧生成管线与规范化编译器输出差异"""

    def __init__(self, session: AsyncSession):
        self.session = session

    def is_shadow_enabled(self) -> bool:
        """检查环境变量或开关是否启用影子模式"""
        return os.environ.get("SHADOW_COMPILER_ENABLED", "0").lower() in ("1", "true", "yes")

    async def compare_and_record_diff(self, target: str = "clash") -> dict[str, Any]:
        """执行影子比对，仅记录结构与摘要差异，不产生写副作用"""
        # 1 旧管线生成
        legacy_res = await generate_service.generate_yaml(self.session)
        legacy_yaml = str(legacy_res.get("yaml") or "")

        # 2 新规范化管线编译
        bundle = await build_logical_bundle_from_db(self.session, bundle_name="shadow-run")

        nodes = await NodeRepository.list_nodes(self.session, lifecycle_state="active")
        node_payloads = [
            {
                "logical_id": n.logical_id,
                "name": n.name,
                "protocol": n.protocol,
                "server": n.server,
                "port": n.port,
                "normalized_payload": n.normalized_payload,
            }
            for n in nodes
        ]

        resolver = CanonicalGraphResolver(bundle=bundle, inventory_nodes=node_payloads)
        comp_res = resolver.compile()
        raw_nodes = [n.normalized_payload for n in nodes if n.normalized_payload]
        new_yaml = render_compiled_yaml(comp_res, bundle=bundle, raw_nodes=raw_nodes)

        # 3 生成安全对比摘要
        legacy_lines = [
            line.strip()
            for line in legacy_yaml.splitlines()
            if line.strip() and not line.strip().startswith("#")
        ]
        new_lines = [
            line.strip()
            for line in new_yaml.splitlines()
            if line.strip() and not line.strip().startswith("#")
        ]

        diff_summary = {
            "target": target,
            "legacy_line_count": len(legacy_lines),
            "new_line_count": len(new_lines),
            "semantic_fingerprint": comp_res.semantic_fingerprint,
            "diagnostics_count": len(comp_res.diagnostics),
            "is_exact_match": legacy_lines == new_lines,
        }
        return diff_summary
