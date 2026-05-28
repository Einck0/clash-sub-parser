from app.utils.base64_decode import try_base64_decode
from app.utils.dedup import deduplicate_nodes
from app.utils.validators import ensure_group_ids_exist, validate_fetch_url, validate_no_circular_reference


def test_try_base64_decode_plain_text_passthrough() -> None:
    text = "not base64"
    assert try_base64_decode(text) == text


def test_deduplicate_nodes_by_name_and_signature() -> None:
    nodes = [
        {"name": "A", "type": "ss", "server": "1.1.1.1", "port": 443, "password": "x"},
        {"name": "A", "type": "ss", "server": "2.2.2.2", "port": 443, "password": "x"},
        {"name": "B", "type": "ss", "server": "1.1.1.1", "port": 443, "password": "x"},
        {"name": "C", "type": "ss", "server": "3.3.3.3", "port": 443, "password": "y"},
    ]
    result = deduplicate_nodes(nodes)
    assert [item["name"] for item in result] == ["A", "C"]


def test_validate_cycle_detected() -> None:
    graph = {1: [2], 2: [3], 3: [1]}
    try:
        validate_no_circular_reference(graph)
    except Exception as exc:
        assert "Circular" in str(exc)
        return
    raise AssertionError("Expected circular reference exception")


def test_ensure_group_ids_exist() -> None:
    try:
        ensure_group_ids_exist([1, 9], {1, 2, 3})
    except Exception as exc:
        assert "do not exist" in str(exc)
        return
    raise AssertionError("Expected missing group id exception")


def test_validate_fetch_url_rejects_unsafe_targets() -> None:
    for url in [
        "file:///etc/passwd",
        "http://localhost:18080/sub",
        "http://127.0.0.1/sub",
        "http://10.0.0.1/sub",
        "http://169.254.169.254/latest/meta-data",
    ]:
        try:
            validate_fetch_url(url)
        except Exception:
            continue
        raise AssertionError(f"Expected unsafe URL to be rejected: {url}")


def test_validate_fetch_url_allows_private_targets_when_configured() -> None:
    assert validate_fetch_url("http://127.0.0.1/sub", allow_private_hosts=True) == "http://127.0.0.1/sub"


def test_parse_vless_reality_link_keeps_reality_options() -> None:
    from app.utils.clash_parser import parse_node_links

    link = (
        "vless://00000000-0000-4000-8000-000000000001@example.com:443"
        "?type=tcp&security=reality&pbk=publicKeyExample&fp=chrome"
        "&sni=www.example.com&sid=abcd&spx=/&flow=xtls-rprx-vision#Reality%20Node"
    )

    nodes = parse_node_links(link)
    assert len(nodes) == 1
    node = nodes[0]
    assert node["name"] == "Reality Node"
    assert node["type"] == "vless"
    assert node["server"] == "example.com"
    assert node["port"] == 443
    assert node["network"] == "tcp"
    assert node["tls"] is True
    assert node["flow"] == "xtls-rprx-vision"
    assert node["servername"] == "www.example.com"
    assert node["client-fingerprint"] == "chrome"
    assert node["reality-opts"] == {
        "public-key": "publicKeyExample",
        "short-id": "abcd",
        "spider-x": "/",
    }
