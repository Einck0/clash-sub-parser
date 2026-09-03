from __future__ import annotations

import hashlib
import json
import re
from typing import Any, Literal, Sequence

from app.schemas.inventory import InventoryNodeItem
from app.schemas.logical_config import (
    CompilationDiagnostic,
    CompilationResult,
    CompiledGroup,
    LogicalConfigurationBundle,
    LogicalGroupConfig,
)


def _matches_regex(pattern: str, text: str) -> bool:
    try:
        return bool(re.search(pattern, text, re.IGNORECASE))
    except re.error:
        return False


def _is_qualified_for_policy(
    node: dict[str, Any],
    observation: dict[str, Any] | None,
    min_speed_mbps: float | None,
    required_media: list[str],
) -> bool:
    has_speed = min_speed_mbps is not None and min_speed_mbps > 0
    has_media = bool(required_media)
    if not has_speed and not has_media:
        return True

    if not observation or observation.get("status") != "ok":
        return False

    if has_speed and min_speed_mbps is not None:
        spd = observation.get("speed_mbps")
        if spd is None or float(spd) < float(min_speed_mbps):
            return False

    if has_media:
        media_map = observation.get("media") or {}
        for plat in required_media:
            p_res = media_map.get(plat.lower().strip())
            if not p_res:
                return False
            status = p_res.get("status")
            unlocked = p_res.get("unlocked")
            if status not in ("ok", "full", "originals") and not unlocked:
                return False

    return True


class CanonicalGraphResolver:
    """纯内存规范化策略组图解析与编译器，不发起任何数据库或网络 IO"""

    def __init__(
        self,
        bundle: LogicalConfigurationBundle,
        inventory_nodes: Sequence[InventoryNodeItem | dict[str, Any]],
        observations: dict[str, dict[str, Any]] | None = None,
    ) -> None:
        self.bundle = bundle
        self.inventory_nodes = inventory_nodes
        self.observations = observations or {}
        self.diagnostics: list[CompilationDiagnostic] = []

        # 建立逻辑 ID 与名称索引
        self.group_map: dict[str, LogicalGroupConfig] = {g.logical_id: g for g in bundle.groups}
        self.group_by_name: dict[str, LogicalGroupConfig] = {g.name: g for g in bundle.groups}

        # 节点字典
        self.node_dict_list: list[dict[str, Any]] = []
        for n in inventory_nodes:
            if isinstance(n, InventoryNodeItem):
                self.node_dict_list.append(n.model_dump())
            elif isinstance(n, dict):
                self.node_dict_list.append(n)
            else:
                self.node_dict_list.append(dict(n))

    def validate_graph(self) -> list[CompilationDiagnostic]:
        """静态校验图结构中的自引用、循环依赖与缺失引用"""
        diagnostics: list[CompilationDiagnostic] = []

        # 检查缺失的组引用与自引用
        for group in self.bundle.groups:
            for idx, entry in enumerate(group.include_entries):
                loc = f"groups[{group.name}].include_entries[{idx}]"
                if entry.entry_type == "group_ref":
                    ref_id = entry.value
                    if ref_id == group.logical_id or ref_id == group.name:
                        diagnostics.append(
                            CompilationDiagnostic(
                                level="error",
                                code="SELF_REFERENCE",
                                message=f"策略组 '{group.name}' 包含对自身的直接引用",
                                location=loc,
                            )
                        )
                    elif ref_id not in self.group_map and ref_id not in self.group_by_name:
                        diagnostics.append(
                            CompilationDiagnostic(
                                level="warning",
                                code="UNKNOWN_GROUP_REF",
                                message=f"策略组 '{group.name}' 引用的下游组 '{ref_id}' 不存在",
                                location=loc,
                            )
                        )
                elif entry.entry_type == "regex":
                    try:
                        re.compile(entry.value)
                    except re.error as e:
                        diagnostics.append(
                            CompilationDiagnostic(
                                level="error",
                                code="INVALID_REGEX",
                                message=f"正则表达式 '{entry.value}' 语法错误: {e}",
                                location=loc,
                            )
                        )

        # 检查循环引用 (DFS)
        visited: dict[str, int] = {}  # 0: unvisited, 1: visiting, 2: visited

        def dfs(gid: str, trail: list[str]) -> None:
            visited[gid] = 1
            g = self.group_map.get(gid) or self.group_by_name.get(gid)
            if not g:
                visited[gid] = 2
                return

            for entry in g.include_entries:
                if entry.entry_type in ("group_ref", "exclude_group"):
                    child_id = entry.value
                    child_g = self.group_map.get(child_id) or self.group_by_name.get(child_id)
                    if child_g:
                        cid = child_g.logical_id
                        if visited.get(cid) == 1:
                            cycle_path = " -> ".join(trail + [child_g.name])
                            diagnostics.append(
                                CompilationDiagnostic(
                                    level="error",
                                    code="CIRCULAR_DEPENDENCY",
                                    message=f"策略组存在循环引用: {cycle_path}",
                                    location=f"groups[{g.name}]",
                                )
                            )
                        elif visited.get(cid, 0) == 0:
                            dfs(cid, trail + [child_g.name])
            visited[gid] = 2

        for gid in list(self.group_map.keys()):
            if visited.get(gid, 0) == 0:
                dfs(gid, [self.group_map[gid].name])

        return diagnostics

    def resolve_group_nodes(
        self,
        group: LogicalGroupConfig,
        resolving_trail: set[str] | None = None,
    ) -> list[str]:
        """递归解析单个策略组的包含节点列表"""
        if resolving_trail is None:
            resolving_trail = set()

        if group.logical_id in resolving_trail:
            return []

        current_trail = resolving_trail | {group.logical_id}
        included_names: list[str] = []
        excluded_names: set[str] = set(group.exclude_nodes)

        for entry in group.include_entries:
            etype = entry.entry_type
            val = entry.value

            if etype == "node_name":
                included_names.append(val)
            elif etype == "regex":
                for node in self.node_dict_list:
                    name = str(node.get("name") or "")
                    if name and _matches_regex(val, name):
                        included_names.append(name)
            elif etype == "source_ref":
                for node in self.node_dict_list:
                    src_id = str(node.get("source_id") or node.get("source_logical_id") or "")
                    if src_id == val:
                        name = str(node.get("name") or "")
                        if name:
                            included_names.append(name)
            elif etype == "group_ref":
                child_group = self.group_map.get(val) or self.group_by_name.get(val)
                if child_group:
                    # 如果子组不是叶子节点，加入组名本身或子节点
                    child_nodes = self.resolve_group_nodes(child_group, current_trail)
                    included_names.extend(child_nodes)
            elif etype == "exclude_node":
                excluded_names.add(val)
            elif etype == "exclude_group":
                child_group = self.group_map.get(val) or self.group_by_name.get(val)
                if child_group:
                    child_nodes = self.resolve_group_nodes(child_group, current_trail)
                    excluded_names.update(child_nodes)

        # 保持顺序去重
        deduped: list[str] = []
        seen: set[str] = set()
        for name in included_names:
            if name and name not in excluded_names and name not in seen:
                seen.add(name)
                deduped.append(name)

        # 探针能力与测速过滤
        if group.filter_min_speed_mbps or group.filter_media_unlock:
            filtered: list[str] = []
            for name in deduped:
                # 查找节点对应的探测结果
                obs = self.observations.get(name)
                # 寻找匹配节点
                matching_node = next((n for n in self.node_dict_list if n.get("name") == name), None)
                if matching_node:
                    n_key = f"{matching_node.get('name')}|{matching_node.get('type')}|{matching_node.get('server')}:{matching_node.get('port')}"
                    obs = obs or self.observations.get(n_key) or self.observations.get(str(matching_node.get("logical_id", "")))

                if _is_qualified_for_policy(
                    matching_node or {"name": name},
                    obs,
                    group.filter_min_speed_mbps,
                    group.filter_media_unlock,
                ):
                    filtered.append(name)
            deduped = filtered

        # 兜底 DIRECT
        if group.add_fallback and not deduped:
            deduped.append("DIRECT")

        return deduped

    def compile(self, target: Literal["yaml", "script", "preview", "ledger", "subscription"] = "preview") -> CompilationResult:
        """执行完整编译流程并生成确定性指纹"""
        self.diagnostics = self.validate_graph()

        has_fatal_errors = any(d.level == "error" for d in self.diagnostics)

        compiled_groups: list[CompiledGroup] = []
        for g in sorted(self.bundle.groups, key=lambda x: x.sort_order):
            resolved_names = self.resolve_group_nodes(g)
            compiled_groups.append(
                CompiledGroup(
                    name=g.name,
                    group_type=g.group_type,
                    resolved_node_names=resolved_names,
                    resolved_node_ids=[],
                    raw_config=g.model_dump(),
                )
            )

        # 编译分流规则
        compiled_rules: list[dict[str, Any]] = []
        for r in sorted(self.bundle.rules, key=lambda x: x.sort_order):
            if not r.enabled:
                continue
            target_out = r.target_direct_or_reject
            if not target_out and r.target_group_logical_id:
                target_g = self.group_map.get(r.target_group_logical_id)
                target_out = target_g.name if target_g else r.target_group_logical_id
            compiled_rules.append(
                {
                    "type": r.rule_type,
                    "payload": r.payload,
                    "target": target_out or "DIRECT",
                }
            )

        # 计算确定性语义指纹
        fingerprint_source = {
            "groups": [{"name": cg.name, "type": cg.group_type, "nodes": cg.resolved_node_names} for cg in compiled_groups],
            "rules": compiled_rules,
            "dns": self.bundle.dns.model_dump(),
            "generate": self.bundle.generate.model_dump(),
        }
        semantic_fingerprint = hashlib.sha256(
            json.dumps(fingerprint_source, sort_keys=True, ensure_ascii=False).encode("utf-8")
        ).hexdigest()

        return CompilationResult(
            success=not has_fatal_errors,
            semantic_fingerprint=semantic_fingerprint,
            groups=compiled_groups,
            rules=compiled_rules,
            diagnostics=self.diagnostics,
            rendered_output=None,
        )
