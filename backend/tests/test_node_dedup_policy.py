from __future__ import annotations

from app.models.node import Node
from app.models.source import NodeSourceLink, Source
from app.services.node_dedup_policy import compile_deduplicated_nodes


def test_compile_dedup_disambiguates_duplicate_names():
    src1 = Source(logical_id="src_1", name="订阅源1", kind="subscription")
    src2 = Source(logical_id="src_2", name="订阅源2", kind="subscription")

    # 两个不同 payload 但同名的节点
    node1 = Node(
        logical_id="node_1",
        name="香港 01",
        protocol="ss",
        server="1.1.1.1",
        port=443,
        normalized_payload={"type": "ss", "server": "1.1.1.1", "port": 443},
        payload_fingerprint="fp_1",
    )
    node2 = Node(
        logical_id="node_2",
        name="香港 01",
        protocol="ss",
        server="2.2.2.2",
        port=443,
        normalized_payload={"type": "ss", "server": "2.2.2.2", "port": 443},
        payload_fingerprint="fp_2",
    )

    link1 = NodeSourceLink(display_name="香港 01", original_order=0)
    link2 = NodeSourceLink(display_name="香港 01", original_order=0)

    entries = [(node1, link1, src1), (node2, link2, src2)]

    # 1 disambiguate_names 模式
    compiled = compile_deduplicated_nodes(entries, mode="disambiguate_names")
    assert len(compiled) == 2
    assert compiled[0].display_name == "香港 01"
    assert compiled[0].source_name == "订阅源1"
    assert compiled[1].display_name == "香港 01 (2)"
    assert compiled[1].source_name == "订阅源2"

    # 2 keep_first 模式
    compiled_first = compile_deduplicated_nodes(entries, mode="keep_first")
    assert len(compiled_first) == 1
    assert compiled_first[0].display_name == "香港 01"
    assert compiled_first[0].source_name == "订阅源1"


def test_compile_dedup_fingerprints_cross_source():
    src1 = Source(logical_id="src_1", name="源1", kind="subscription")
    src2 = Source(logical_id="src_2", name="源2", kind="subscription")

    # 两个不同源但相同 payload 指纹的节点
    node_common = Node(
        logical_id="node_common",
        name="公共节点",
        protocol="trojan",
        server="3.3.3.3",
        port=443,
        normalized_payload={"type": "trojan", "server": "3.3.3.3", "port": 443},
        payload_fingerprint="fp_common",
    )

    link1 = NodeSourceLink(display_name="源1节点", original_order=0)
    link2 = NodeSourceLink(display_name="源2节点", original_order=0)

    entries = [(node_common, link1, src1), (node_common, link2, src2)]

    # 启用指纹去重（默认）
    compiled = compile_deduplicated_nodes(entries, dedup_fingerprints=True)
    assert len(compiled) == 1
    assert compiled[0].display_name == "源1节点"
    assert compiled[0].source_name == "源1"

    # 关闭指纹去重
    compiled_all = compile_deduplicated_nodes(entries, dedup_fingerprints=False)
    assert len(compiled_all) == 2
