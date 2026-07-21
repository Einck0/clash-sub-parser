"""Proxy-chain helpers: effective dialer hop for subscription/node overrides.

P0 export: single hop only (last hop of the effective chain) → dialer-proxy.
Multi-hop wrapper nodes are P1.
"""

from __future__ import annotations

from typing import Any


FORBIDDEN_HOPS = frozenset({"DIRECT", "REJECT", "PASS"})


def normalize_chain(raw: Any) -> list[str]:
    """Normalize an ordered hop list: strip, drop empties, keep order, dedupe consecutive."""
    if raw is None:
        return []
    if not isinstance(raw, (list, tuple)):
        return []
    out: list[str] = []
    for item in raw:
        name = str(item or "").strip()
        if not name:
            continue
        if out and out[-1] == name:
            continue
        out.append(name)
    return out


def normalize_node_proxy_chains(raw: Any) -> dict[str, list[str]]:
    """Normalize per-node overrides.

    Missing key in product sense is "follow subscription" — we simply omit it.
    Explicit [] is stored as empty list (force no chain).
    Values that are null are omitted (follow subscription).
    """
    if not isinstance(raw, dict):
        return {}
    out: dict[str, list[str]] = {}
    for key, value in raw.items():
        name = str(key or "").strip()
        if not name:
            continue
        if value is None:
            continue
        out[name] = normalize_chain(value)
    return out


def effective_chain(
    node_name: str,
    *,
    subscription_chain: list[str] | None,
    node_chains: dict[str, list[str]] | None,
) -> list[str] | None:
    """Return effective hop list for a final node name.

    Returns:
      - list (possibly empty) when node has an explicit override key
      - subscription_chain (normalized) when no override
    Distinguishes override [] (disable) from missing key (follow).
    """
    name = str(node_name or "").strip()
    mapping = node_chains or {}
    if name in mapping:
        # explicit override, including []
        return list(mapping[name] or [])
    return normalize_chain(subscription_chain or [])


def dialer_hop_from_chain(chain: list[str] | None) -> str | None:
    """P0: use the last hop as dialer-proxy. Empty → None."""
    hops = normalize_chain(chain or [])
    if not hops:
        return None
    return hops[-1]


def apply_dialer_proxy(
    node: dict,
    hop: str | None,
    *,
    known_names: set[str] | None = None,
) -> dict:
    """Return a shallow copy of node with dialer-proxy set or cleared.

    If hop is None: strip dialer-proxy (if any) so overrides to [] win.
    If hop equals node name: leave without dialer (self-ref).
    If known_names provided and hop not in it: still set hop (group names may
    not be in proxies yet); callers may validate separately.
    """
    copied = dict(node)
    name = str(copied.get("name") or "").strip()
    if not hop:
        copied.pop("dialer-proxy", None)
        return copied
    hop = str(hop).strip()
    if not hop or hop == name:
        copied.pop("dialer-proxy", None)
        return copied
    if hop in FORBIDDEN_HOPS:
        copied.pop("dialer-proxy", None)
        return copied
    if known_names is not None and hop not in known_names and hop not in FORBIDDEN_HOPS:
        # Still allow: hop may be a proxy-group resolved later.
        pass
    copied["dialer-proxy"] = hop
    return copied


def apply_subscription_chains(
    nodes: list[dict],
    *,
    subscription_chain: list[str] | None,
    node_chains: dict[str, list[str]] | None,
    known_names: set[str] | None = None,
) -> list[dict]:
    """Apply effective dialer-proxy to each node (P0 last-hop only)."""
    sub = normalize_chain(subscription_chain or [])
    overrides = normalize_node_proxy_chains(node_chains or {})
    out: list[dict] = []
    for node in nodes or []:
        if not isinstance(node, dict):
            continue
        name = str(node.get("name") or "").strip()
        chain = effective_chain(name, subscription_chain=sub, node_chains=overrides)
        hop = dialer_hop_from_chain(chain)
        out.append(apply_dialer_proxy(node, hop, known_names=known_names))
    return out
