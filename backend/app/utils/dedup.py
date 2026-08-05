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
    if node_type == "wireguard":
        identity = str(node.get("private-key") or node.get("ip") or "").strip()
        peer = str(node.get("public-key") or "").strip().lower()
        wireguard_options = "|".join(
            [
                str(node.get("ipv6") or "").strip().lower(),
                str(node.get("reserved") or "").strip().lower(),
                str(node.get("mtu") or "").strip(),
            ]
        )
    else:
        identity = str(node.get("uuid") or node.get("password") or "").strip().lower()
        peer = ""
        wireguard_options = ""
    tls = str(node.get("tls", "")).strip().lower()
    network = str(node.get("network", "")).strip().lower()
    sni = str(node.get("servername") or node.get("sni") or "").strip().lower()
    return "|".join([node_type, server, port, identity, peer, wireguard_options, tls, network, sni])
