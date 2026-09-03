from __future__ import annotations

import pytest

from tests.conftest import TestSession
from app.models.dns import DnsConfig
from app.models.generate_config import GenerateConfig
from app.models.node_group import NodeGroup
from app.models.rule import Rule
from app.models.rule_category import RuleCategory
from app.schemas.logical_config import (
    LogicalConfigurationBundle,
    LogicalDnsConfig,
    LogicalGenerateSettings,
    LogicalGroupConfig,
    LogicalRuleConfig,
    MembershipEntry,
)
from app.services.canonical_compiler import CanonicalGraphResolver
from app.services.compiler_cache import compiler_cache
from app.services.compiler_target_adapters import (
    render_compiled_script,
    render_compiled_yaml,
)
from app.services.legacy_config_importer import build_logical_bundle_from_db


def test_validation_detects_self_reference_and_cycles():
    # 构造含自引用与循环引用的策略组
    g1 = LogicalGroupConfig(
        logical_id="g1",
        name="组1",
        include_entries=[MembershipEntry(entry_type="group_ref", value="g2")],
    )
    g2 = LogicalGroupConfig(
        logical_id="g2",
        name="组2",
        include_entries=[
            MembershipEntry(entry_type="group_ref", value="g2"),  # 自引用
            MembershipEntry(entry_type="group_ref", value="g1"),  # 循环引用
            MembershipEntry(entry_type="group_ref", value="unknown_group"),  # 未知组
            MembershipEntry(entry_type="regex", value="[invalid_regex"),  # 语法错误正则
        ],
    )

    bundle = LogicalConfigurationBundle(
        logical_id="b1",
        groups=[g1, g2],
    )

    resolver = CanonicalGraphResolver(bundle=bundle, inventory_nodes=[])
    result = resolver.compile()

    assert result.success is False
    codes = [d.code for d in result.diagnostics]
    assert "SELF_REFERENCE" in codes
    assert "CIRCULAR_DEPENDENCY" in codes
    assert "UNKNOWN_GROUP_REF" in codes
    assert "INVALID_REGEX" in codes


def test_canonical_graph_resolver_expansion_and_dedup():
    nodes = [
        {"logical_id": "n1", "name": "香港 01", "source_id": "src1"},
        {"logical_id": "n2", "name": "香港 02", "source_id": "src1"},
        {"logical_id": "n3", "name": "日本 01", "source_id": "src2"},
        {"logical_id": "n4", "name": "美国 01", "source_id": "src2"},
    ]

    # g_hk 通过正则匹配
    g_hk = LogicalGroupConfig(
        logical_id="g_hk",
        name="香港节点",
        include_entries=[
            MembershipEntry(entry_type="regex", value="香港"),
        ],
    )
    # g_all 包含 g_hk，加显式节点，排除 香港 02
    g_all = LogicalGroupConfig(
        logical_id="g_all",
        name="所有节点",
        include_entries=[
            MembershipEntry(entry_type="group_ref", value="g_hk"),
            MembershipEntry(entry_type="node_name", value="日本 01"),
            MembershipEntry(entry_type="source_ref", value="src2"),
            MembershipEntry(entry_type="exclude_node", value="香港 02"),
        ],
    )

    bundle = LogicalConfigurationBundle(
        logical_id="b2",
        groups=[g_hk, g_all],
    )

    resolver = CanonicalGraphResolver(bundle=bundle, inventory_nodes=nodes)
    result = resolver.compile()

    assert result.success is True
    groups_dict = {g.name: g.resolved_node_names for g in result.groups}
    assert groups_dict["香港节点"] == ["香港 01", "香港 02"]
    # 验证去重与排除：香港 01, 日本 01, 美国 01（香港 02 被排除，日本 01 不重复）
    assert groups_dict["所有节点"] == ["香港 01", "日本 01", "美国 01"]


def test_policy_eligibility_filter():
    nodes = [
        {"logical_id": "n1", "name": "高速 01", "type": "ss", "server": "1.1.1.1", "port": 443},
        {"logical_id": "n2", "name": "低速 02", "type": "ss", "server": "2.2.2.2", "port": 443},
        {"logical_id": "n3", "name": "解锁 03", "type": "ss", "server": "3.3.3.3", "port": 443},
    ]
    observations = {
        "高速 01": {"status": "ok", "speed_mbps": 50.0, "media": {}},
        "低速 02": {"status": "ok", "speed_mbps": 2.0, "media": {}},
        "解锁 03": {
            "status": "ok",
            "speed_mbps": 20.0,
            "media": {"netflix": {"status": "full", "label": "原生全解锁"}},
        },
    }

    g_fast = LogicalGroupConfig(
        logical_id="g_fast",
        name="测速优选",
        include_entries=[MembershipEntry(entry_type="regex", value=".*")],
        filter_min_speed_mbps=10.0,
    )
    g_media = LogicalGroupConfig(
        logical_id="g_media",
        name="奈飞专线",
        include_entries=[MembershipEntry(entry_type="regex", value=".*")],
        filter_media_unlock=["netflix"],
    )

    bundle = LogicalConfigurationBundle(logical_id="b3", groups=[g_fast, g_media])
    resolver = CanonicalGraphResolver(bundle=bundle, inventory_nodes=nodes, observations=observations)
    result = resolver.compile()

    groups_dict = {g.name: g.resolved_node_names for g in result.groups}
    assert groups_dict["测速优选"] == ["高速 01", "解锁 03"]
    assert groups_dict["奈飞专线"] == ["解锁 03"]


def test_deterministic_semantic_fingerprint_and_adapters():
    bundle = LogicalConfigurationBundle(
        logical_id="b4",
        groups=[
            LogicalGroupConfig(
                logical_id="g1",
                name="PROXY",
                group_type="select",
                include_entries=[MembershipEntry(entry_type="node_name", value="节点A")],
            )
        ],
        rules=[
            LogicalRuleConfig(
                logical_id="r1",
                rule_type="DOMAIN-SUFFIX",
                payload="google.com",
                target_group_logical_id="g1",
            )
        ],
        dns=LogicalDnsConfig(raw_yaml="nameserver: [8.8.8.8]", enabled=True),
        generate=LogicalGenerateSettings(switches={"remove_nodes": False}),
    )
    nodes = [{"name": "节点A", "type": "ss", "server": "1.1.1.1", "port": 8388}]

    resolver = CanonicalGraphResolver(bundle=bundle, inventory_nodes=nodes)
    res1 = resolver.compile()
    res2 = resolver.compile()

    # 验证相同输入指纹绝对一致
    assert res1.semantic_fingerprint == res2.semantic_fingerprint
    assert len(res1.semantic_fingerprint) == 64

    # 验证目标适配器输出
    yaml_out = render_compiled_yaml(res1, bundle, nodes)
    assert "PROXY" in yaml_out
    assert "google.com,PROXY" in yaml_out
    assert "8.8.8.8" in yaml_out

    script_out = render_compiled_script(res1, bundle)
    assert "function main(config)" in script_out
    assert "PROXY" in script_out


def test_compiler_cache_behavior():
    compiler_cache.clear()
    bundle = LogicalConfigurationBundle(logical_id="b5")
    resolver = CanonicalGraphResolver(bundle=bundle, inventory_nodes=[])
    res = resolver.compile()

    assert compiler_cache.get("rev1", "gen1") is None
    compiler_cache.set("rev1", "gen1", res)

    cached = compiler_cache.get("rev1", "gen1")
    assert cached is not None
    assert cached.semantic_fingerprint == res.semantic_fingerprint

    # 修改 revision 未命中
    assert compiler_cache.get("rev2", "gen1") is None

    # 按配置版本主动失效
    deleted_count = compiler_cache.invalidate_by_config("rev1")
    assert deleted_count == 1
    assert compiler_cache.get("rev1", "gen1") is None

    # 重新存入并按库存世代失效
    compiler_cache.set("rev1", "gen1", res)
    assert compiler_cache.get("rev1", "gen1") is not None
    del_inv = compiler_cache.invalidate_by_inventory("gen1")
    assert del_inv == 1
    assert compiler_cache.get("rev1", "gen1") is None


@pytest.mark.asyncio
async def test_legacy_config_importer_integration():
    async with TestSession() as session:
        # 准备旧 ORM 数据
        cat = RuleCategory(name="广告拦截", sort_order=1)
        session.add(cat)
        grp = NodeGroup(name="自动选择", group_type="url-test", sort_order=1, include_nodes=["HK-01", "JP-01"])
        session.add(grp)
        rule = Rule(name="测试规则", category="广告拦截", type="DOMAIN-SUFFIX", value="ads.com", proxy="REJECT", sort_order=1, enabled=True)
        session.add(rule)
        dns = DnsConfig(id=1, raw_yaml="nameserver: [1.1.1.1]", enabled=True)
        session.add(dns)
        gen = GenerateConfig(id=1, enabled=True, exclude_node_proxies=False)
        session.add(gen)
        await session.commit()

        # 执行提取与转换
        bundle = await build_logical_bundle_from_db(session, "test_legacy_import")

        assert len(bundle.groups) == 1
        assert bundle.groups[0].name == "自动选择"
        assert bundle.groups[0].group_type == "url-test"
        assert len(bundle.groups[0].include_entries) == 2
        assert len(bundle.rule_categories) == 1
        assert len(bundle.rules) == 1
        assert bundle.rules[0].target_direct_or_reject == "REJECT"
        assert bundle.dns.enabled is True
        assert bundle.generate.switches.get("exclude_node_proxies") is False

        # 编译导入后的 bundle
        nodes = [{"name": "HK-01"}, {"name": "JP-01"}]
        resolver = CanonicalGraphResolver(bundle=bundle, inventory_nodes=nodes)
        result = resolver.compile()
        assert result.success is True
        assert result.groups[0].resolved_node_names == ["HK-01", "JP-01"]
