"""遗留输出语义比较器

规范化配置输出后做语义比对，区分格式变动和语义变动
"""
from __future__ import annotations

import json


class SemanticDifference:
    """一条语义差异"""

    def __init__(self, path: str, kind: str, detail: str) -> None:
        self.path = path
        self.kind = kind
        self.detail = detail

    def __repr__(self) -> str:
        return f"SemanticDifference({self.path!r}, {self.kind!r}, {self.detail!r})"


def normalize_yaml_output(data: dict) -> dict:
    """规范化 YAML 配置输出，消除格式差异"""
    normalized = {}
    for key, value in sorted(data.items()):
        if isinstance(value, dict):
            normalized[key] = normalize_yaml_output(value)
        elif isinstance(value, list):
            normalized[key] = _normalize_list(value)
        elif isinstance(value, str):
            normalized[key] = value.strip()
        else:
            normalized[key] = value
    return normalized


def normalize_script_modules(modules: list) -> list:
    """规范化 script.js 模块列表，消除格式差异"""
    return sorted(
        [_normalize_module(m) for m in modules],
        key=lambda m: json.dumps(m, ensure_ascii=False, sort_keys=True),
    )


def normalize_preview(preview: list | dict) -> list | dict:
    """规范化节点组预览输出"""
    if isinstance(preview, list):
        return sorted(
            [_normalize_preview_entry(e) for e in preview],
            key=lambda e: json.dumps(e, ensure_ascii=False, sort_keys=True),
        )
    return preview


def compare_outputs(
    actual: dict,
    expected: dict,
    approved_differences: set[str] | None = None,
) -> list[SemanticDifference]:
    """比较两组输出，返回语义差异列表

    approved_differences 是允许的差异路径集合（白名单）
    """
    approved = approved_differences or set()
    diffs: list[SemanticDifference] = []

    norm_actual_yaml = normalize_yaml_output(actual.get("yaml", {}))
    norm_expected_yaml = normalize_yaml_output(expected.get("yaml", {}))
    _diff_recursive(norm_actual_yaml, norm_expected_yaml, "yaml", diffs)

    norm_actual_modules = normalize_script_modules(actual.get("script_modules", []))
    norm_expected_modules = normalize_script_modules(expected.get("script_modules", []))
    if norm_actual_modules != norm_expected_modules:
        diffs.append(SemanticDifference("script_modules", "changed", "模块列表语义不一致"))

    norm_actual_preview = normalize_preview(actual.get("node_group_preview", []))
    norm_expected_preview = normalize_preview(expected.get("node_group_preview", []))
    if norm_actual_preview != norm_expected_preview:
        diffs.append(SemanticDifference("node_group_preview", "changed", "节点组预览语义不一致"))

    norm_actual_sub = normalize_yaml_output(actual.get("subscription_yaml", {}))
    norm_expected_sub = normalize_yaml_output(expected.get("subscription_yaml", {}))
    _diff_recursive(norm_actual_sub, norm_expected_sub, "subscription_yaml", diffs)

    return [d for d in diffs if d.path not in approved]


def _normalize_list(items: list) -> list:
    normalized = []
    for item in items:
        if isinstance(item, dict):
            normalized.append(normalize_yaml_output(item))
        elif isinstance(item, str):
            normalized.append(item.strip())
        else:
            normalized.append(item)
    return normalized


def _normalize_module(module: dict | list | str) -> dict | list | str:
    if isinstance(module, dict):
        return {k: _normalize_module(v) for k, v in sorted(module.items())}
    if isinstance(module, list):
        return [_normalize_module(i) for i in module]
    if isinstance(module, str):
        return module.strip()
    return module


def _normalize_preview_entry(entry: dict | list | str) -> dict | list | str:
    if isinstance(entry, dict):
        return {k: _normalize_preview_entry(v) for k, v in sorted(entry.items())}
    if isinstance(entry, list):
        return [_normalize_preview_entry(i) for i in entry]
    return entry


def _diff_recursive(
    actual: object,
    expected: object,
    path: str,
    diffs: list[SemanticDifference],
) -> None:
    if type(actual) is not type(expected):
        diffs.append(SemanticDifference(path, "type_changed", f"{type(expected).__name__} -> {type(actual).__name__}"))
        return
    if isinstance(actual, dict) and isinstance(expected, dict):
        all_keys = set(actual) | set(expected)
        for key in sorted(all_keys):
            child_path = f"{path}.{key}"
            if key not in actual:
                diffs.append(SemanticDifference(child_path, "removed", "键被移除"))
            elif key not in expected:
                diffs.append(SemanticDifference(child_path, "added", "新增键"))
            else:
                _diff_recursive(actual[key], expected[key], child_path, diffs)
    elif isinstance(actual, list) and isinstance(expected, list):
        if len(actual) != len(expected):
            diffs.append(SemanticDifference(path, "length_changed", f"长度从 {len(expected)} 变为 {len(actual)}"))
        for i, (a, e) in enumerate(zip(actual, expected)):
            _diff_recursive(a, e, f"{path}[{i}]", diffs)
    elif actual != expected:
        diffs.append(SemanticDifference(path, "value_changed", f"{expected!r} -> {actual!r}"))
