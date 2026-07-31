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
        assert "循环" in str(exc) or "Circular" in str(exc)
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


def test_with_fallback_only_when_empty() -> None:
    from app.utils.group_utils import with_fallback

    assert with_fallback(["a", "b"], True) == ["a", "b"]
    assert with_fallback(["a", "PASS", "b"], True) == ["a", "b"]
    assert with_fallback([], True) == ["PASS"]
    assert with_fallback(["PASS"], True) == ["PASS"]
    assert with_fallback([], False) == []
    assert with_fallback(["a"], False) == ["a"]


def _fake_group(
    gid: int,
    name: str,
    *,
    include_entries: list[dict] | None = None,
    exclude_nodes: list[str] | None = None,
):
    """构造最小 NodeGroup 替身, 只填 resolve_group_members 会读的字段"""
    from types import SimpleNamespace

    return SimpleNamespace(
        id=gid,
        name=name,
        include_entries=list(include_entries or []),
        include_nodes=[],
        include_group_ids=[],
        include_group_nodes_ids=[],
        exclude_nodes=list(exclude_nodes or []),
        exclude_group_ids=[],
    )


def test_resolve_group_members_export_keeps_nested_group_name() -> None:
    from app.utils.group_utils import resolve_group_members

    leaf = _fake_group(1, "叶子组", include_entries=[{"type": "node", "value": "A"}])
    parent = _fake_group(
        2,
        "父组",
        include_entries=[
            {"type": "group", "value": 1},
            {"type": "node", "value": "B"},
        ],
    )
    all_names = ["A", "B", "C"]
    out = resolve_group_members([leaf, parent], all_names, leaves_only=False)
    assert out[1] == ["A"]
    # 导出模式保留嵌套策略组名, 不展开成叶子
    assert out[2] == ["叶子组", "B"]


def test_resolve_group_members_leaves_only_expands_nested() -> None:
    from app.utils.group_utils import resolve_group_members

    leaf = _fake_group(1, "叶子组", include_entries=[{"type": "node", "value": "A"}])
    parent = _fake_group(
        2,
        "父组",
        include_entries=[
            {"type": "group", "value": 1},
            {"type": "node", "value": "B"},
        ],
    )
    out = resolve_group_members([leaf, parent], ["A", "B"], leaves_only=True)
    assert out[1] == ["A"]
    assert out[2] == ["A", "B"]


def test_resolve_group_members_regex_and_exclude() -> None:
    from app.utils.group_utils import resolve_group_members

    g = _fake_group(
        1,
        "筛选",
        include_entries=[
            {"type": "regex", "value": "^美"},
            {"type": "node", "value": "香港"},
        ],
        exclude_nodes=["香港"],
    )
    names = ["美国1", "美国2", "日本", "香港"]
    out = resolve_group_members([g], names, leaves_only=True)
    assert out[1] == ["美国1", "美国2"]


def test_resolve_group_members_group_nodes_and_cycle_safe() -> None:
    from app.utils.group_utils import resolve_group_members

    a = _fake_group(
        1,
        "A组",
        include_entries=[{"type": "group_nodes", "value": 2}, {"type": "node", "value": "X"}],
    )
    b = _fake_group(
        2,
        "B组",
        include_entries=[{"type": "group_nodes", "value": 1}, {"type": "node", "value": "Y"}],
    )
    # 互指 group_nodes 时不应死循环, 仍能收到各自直接节点
    out = resolve_group_members([a, b], ["X", "Y"], leaves_only=True)
    assert "X" in out[1]
    assert "Y" in out[2]

