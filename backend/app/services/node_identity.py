"""Canonical node identity service.

Provides a pure, dependency-free constructor for the canonical node identity key
in the format 'name|type|server:port'.
"""

from __future__ import annotations

from typing import Any, Mapping


def canonical_node_key(node: Mapping[str, Any]) -> str:
    """Generate canonical node identity key in format 'name|type|server:port'.

    String components (name, type, server) are trimmed.
    A missing or None port is serialized as an empty segment.
    """
    if not isinstance(node, Mapping):
        return "||:"
    name = str(node.get("name") or "").strip()
    ntype = str(node.get("type") or "").strip()
    server = str(node.get("server") or "").strip()
    port_val = node.get("port")
    port = "" if port_val is None else str(port_val).strip()
    return f"{name}|{ntype}|{server}:{port}"
