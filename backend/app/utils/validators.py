import ipaddress
import socket
from urllib.parse import urlparse

from fastapi import HTTPException

BUILTIN_PROXY_TARGETS = {"DIRECT", "REJECT", "PASS"}
ALLOWED_FETCH_SCHEMES = {"http", "https"}


def validate_fetch_url(url: str, allow_private_hosts: bool = False) -> str:
    normalized = (url or "").strip()
    parsed = urlparse(normalized)
    if parsed.scheme.lower() not in ALLOWED_FETCH_SCHEMES:
        raise HTTPException(
            status_code=400,
            detail="Subscription URL must use http or https",
        )
    if not parsed.hostname:
        raise HTTPException(status_code=400, detail="Subscription URL must include a host")
    if allow_private_hosts:
        return normalized

    host = parsed.hostname.strip().rstrip(".").lower()
    if host == "localhost" or host.endswith(".localhost"):
        raise HTTPException(
            status_code=400,
            detail="Subscription URL cannot target localhost by default",
        )

    try:
        addresses = [ipaddress.ip_address(host)]
    except ValueError:
        try:
            # Use SOCK_STREAM to get only the address types relevant for TCP connections
            # Resolve twice with a short delay to detect DNS rebinding (TTL=0 attacks)
            resolved = socket.getaddrinfo(host, parsed.port, type=socket.SOCK_STREAM)
            addresses = list({ipaddress.ip_address(item[4][0]) for item in resolved})
            if not addresses:
                raise HTTPException(status_code=400, detail="Subscription URL host could not be resolved")
        except socket.gaierror as exc:
            raise HTTPException(
                status_code=400,
                detail="Subscription URL host could not be resolved",
            ) from exc

    if any(_is_private_fetch_address(address) for address in addresses):
        raise HTTPException(
            status_code=400,
            detail="Subscription URL cannot target private, local, or reserved networks by default",
        )
    return normalized


def _is_private_fetch_address(address: ipaddress.IPv4Address | ipaddress.IPv6Address) -> bool:
    return any(
        (
            address.is_private,
            address.is_loopback,
            address.is_link_local,
            address.is_multicast,
            address.is_reserved,
            address.is_unspecified,
        )
    )


def ensure_group_ids_exist(
    include_group_ids: list[int], existing_group_ids: set[int]
) -> None:
    missing = sorted(
        {
            group_id
            for group_id in include_group_ids
            if group_id not in existing_group_ids
        }
    )
    if missing:
        raise HTTPException(
            status_code=400, detail=f"Referenced node groups do not exist: {missing}"
        )


def validate_no_circular_reference(graph: dict[int, list[int]]) -> None:
    visiting: set[int] = set()
    visited: set[int] = set()

    def dfs(node: int, path: list[int]) -> None:
        if node in visited:
            return
        if node in visiting:
            cycle_start = path.index(node) if node in path else 0
            cycle = path[cycle_start:] + [node]
            raise HTTPException(
                status_code=400,
                detail=f"Circular node group reference detected: {cycle}",
            )

        visiting.add(node)
        path.append(node)
        for child in graph.get(node, []):
            dfs(child, path)
        path.pop()
        visiting.remove(node)
        visited.add(node)

    for key in graph:
        dfs(key, [])


def validate_rule_proxies_exist(proxy_names: list[str], group_names: set[str]) -> None:
    allowed = set(group_names) | BUILTIN_PROXY_TARGETS
    missing = sorted({proxy for proxy in proxy_names if proxy and proxy not in allowed})
    if missing:
        raise HTTPException(
            status_code=400, detail=f"Rules reference missing proxy targets: {missing}"
        )
