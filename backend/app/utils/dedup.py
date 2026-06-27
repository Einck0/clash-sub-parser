def deduplicate_nodes(nodes: list[dict]) -> list[dict]:
    seen_names: set[str] = set()
    seen_signatures: set[str] = set()
    result: list[dict] = []
    for node in nodes:
        name = str(node.get("name", "")).strip()
        if not name:
            continue
        signature = _build_signature(node)
        if name in seen_names or signature in seen_signatures:
            continue
        seen_names.add(name)
        seen_signatures.add(signature)
        result.append(node)
    return result


def _build_signature(node: dict) -> str:
    node_type = str(node.get("type", "")).strip().lower()
    server = str(node.get("server", "")).strip().lower()
    port = str(node.get("port", "")).strip()
    identity = str(node.get("uuid") or node.get("password") or "").strip().lower()
    tls = str(node.get("tls", "")).strip().lower()
    network = str(node.get("network", "")).strip().lower()
    sni = str(node.get("servername") or node.get("sni") or "").strip().lower()
    return "|".join([node_type, server, port, identity, tls, network, sni])
