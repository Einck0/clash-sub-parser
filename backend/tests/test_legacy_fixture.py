from __future__ import annotations

import ipaddress
import copy
import json
from pathlib import Path

import pytest
import yaml
from sqlalchemy import select
from sqlalchemy.ext.asyncio import async_sessionmaker, create_async_engine

from app.database import Base
from app.models.dns import DnsConfig
from app.models.generate_config import GenerateConfig
from app.models.node_group import NodeGroup
from app.models.node_probe_result import NodeProbeResult
from app.models.probe_config import ProbeConfig
from app.models.proxy_chain import ProxyChainBinding
from app.models.rule import Rule
from app.models.rule_category import RuleCategory
from app.models.subscription import Subscription
from app.services.generate_service import generate_subscription_payload, render_current
from app.services.node_group_service import preview_node_groups
from app.services.output_comparator import compare_outputs


FIXTURE_PATH = Path(__file__).parent / "fixtures" / "legacy-system-v1.json"
OUTPUT_FIXTURE_PATH = Path(__file__).parent / "fixtures" / "legacy-output-v1.json"
MODEL_BY_TABLE = {
    "subscriptions": Subscription,
    "node_groups": NodeGroup,
    "rule_categories": RuleCategory,
    "rules": Rule,
    "dns_config": DnsConfig,
    "generate_config": GenerateConfig,
    "proxy_chain_bindings": ProxyChainBinding,
    "probe_config": ProbeConfig,
    "node_probe_results": NodeProbeResult,
}
FORBIDDEN_KEYS = {
    "authorization",
    "credential",
    "password",
    "private_key",
    "secret",
    "token",
    "token_hash",
    "uuid",
}
DOCUMENTATION_NETWORKS = (
    ipaddress.ip_network("192.0.2.0/24"),
    ipaddress.ip_network("198.51.100.0/24"),
    ipaddress.ip_network("203.0.113.0/24"),
)


def _assert_redacted(value) -> None:
    if isinstance(value, dict):
        assert not FORBIDDEN_KEYS.intersection(value)
        for key, nested in value.items():
            if key.endswith("url"):
                assert nested.startswith("https://fixtures.invalid/")
            _assert_redacted(nested)
    elif isinstance(value, list):
        for nested in value:
            _assert_redacted(nested)


def _assert_documentation_addresses(document: dict) -> None:
    for subscription in document["tables"]["subscriptions"]:
        for node in subscription["raw_nodes"]:
            address = ipaddress.ip_address(node["server"])
            assert any(address in network for network in DOCUMENTATION_NETWORKS)
    for result in document["tables"]["node_probe_results"]:
        if result["ip"]:
            address = ipaddress.ip_address(result["ip"])
            assert any(address in network for network in DOCUMENTATION_NETWORKS)


async def _load_document_into(session, document: dict) -> None:
    for table_name, model in MODEL_BY_TABLE.items():
        for row in document["tables"][table_name]:
            session.add(model(**row))
    await session.commit()


async def _render_legacy_output(document: dict) -> dict:
    engine = create_async_engine("sqlite+aiosqlite:///:memory:")
    async with engine.begin() as connection:
        await connection.run_sync(Base.metadata.create_all)

    session_factory = async_sessionmaker(engine, expire_on_commit=False)
    try:
        async with session_factory() as session:
            await _load_document_into(session, document)
            yaml_text = await render_current(session, "yaml")
            script = await render_current(session, "script")
            preview_result = await preview_node_groups(session)
            subscription_result = await generate_subscription_payload(session, 101)
    finally:
        await engine.dispose()

    marker = "  const modules = "
    modules = json.loads(script.split(marker, 1)[1].split(";\n  if (modules", 1)[0])
    return {
        "yaml": yaml.safe_load(yaml_text),
        "script_modules": modules,
        "node_group_preview": preview_result,
        "subscription_yaml": yaml.safe_load(subscription_result["yaml"]),
    }


@pytest.mark.asyncio
async def test_redacted_legacy_fixture_loads_in_isolated_database():
    document = json.loads(FIXTURE_PATH.read_text(encoding="utf-8"))

    assert document["fixture_version"] == 1
    assert set(document["tables"]) == set(MODEL_BY_TABLE)
    _assert_redacted(document)
    _assert_documentation_addresses(document)

    engine = create_async_engine("sqlite+aiosqlite:///:memory:")
    async with engine.begin() as connection:
        await connection.run_sync(Base.metadata.create_all)

    session_factory = async_sessionmaker(engine, expire_on_commit=False)
    async with session_factory() as session:
        await _load_document_into(session, document)

        for table_name, model in MODEL_BY_TABLE.items():
            rows = (await session.execute(select(model))).scalars().all()
            assert len(rows) == len(document["tables"][table_name])

        groups = (await session.execute(select(NodeGroup).order_by(NodeGroup.id))).scalars().all()
        assert groups[1].include_entries == [{"type": "group", "value": groups[0].id}]
        assert groups[1].exclude_group_ids == [groups[0].id]

        subscriptions = (await session.execute(select(Subscription).order_by(Subscription.id))).scalars().all()
        assert subscriptions[0].raw_nodes[0]["name"] == subscriptions[1].raw_nodes[0]["name"]
        assert subscriptions[0].node_renames == {"旧标签": "当前标签"}

    await engine.dispose()


@pytest.mark.asyncio
async def test_legacy_output_semantics_match_golden_baseline():
    document = json.loads(FIXTURE_PATH.read_text(encoding="utf-8"))
    expected = json.loads(OUTPUT_FIXTURE_PATH.read_text(encoding="utf-8"))

    _assert_redacted(expected)
    actual = await _render_legacy_output(document)
    diffs = compare_outputs(actual, expected)
    assert diffs == [], f"语义差异: {diffs}"


@pytest.mark.asyncio
async def test_format_reordering_does_not_cause_semantic_difference():
    """格式重排（键序变化、空白变化）不应产生语义差异"""
    document = json.loads(FIXTURE_PATH.read_text(encoding="utf-8"))
    expected = json.loads(OUTPUT_FIXTURE_PATH.read_text(encoding="utf-8"))
    reordered = json.loads(json.dumps(expected, sort_keys=True))
    actual = await _render_legacy_output(document)
    diffs = compare_outputs(actual, reordered)
    assert diffs == [], f"格式重排不应产生差异: {diffs}"


@pytest.mark.asyncio
async def test_node_semantic_change_is_detected():
    """节点语义变化（名称被篡改）必须被检测到"""
    document = json.loads(FIXTURE_PATH.read_text(encoding="utf-8"))
    expected = json.loads(OUTPUT_FIXTURE_PATH.read_text(encoding="utf-8"))
    tampered = copy.deepcopy(expected)

    yaml_data = tampered["yaml"]
    if "proxies" in yaml_data:
        for proxy in yaml_data["proxies"]:
            if "name" in proxy:
                proxy["name"] = "TAMPERED_NODE"
                break

    actual = await _render_legacy_output(document)
    diffs = compare_outputs(actual, tampered)
    assert len(diffs) > 0, "篡改节点名称后应检测到语义差异"


@pytest.mark.asyncio
async def test_approved_difference_whitelist_suppresses_known_paths():
    """白名单路径的差异应被过滤"""
    document = json.loads(FIXTURE_PATH.read_text(encoding="utf-8"))
    expected = json.loads(OUTPUT_FIXTURE_PATH.read_text(encoding="utf-8"))
    tampered = copy.deepcopy(expected)

    yaml_data = tampered["yaml"]
    if "proxies" in yaml_data:
        for i, proxy in enumerate(yaml_data["proxies"]):
            if "name" in proxy:
                proxy["name"] = "TAMPERED_NODE"
                break

    actual = await _render_legacy_output(document)
    diffs_raw = compare_outputs(actual, tampered)
    assert len(diffs_raw) > 0
    approved_paths = {d.path for d in diffs_raw}
    diffs_filtered = compare_outputs(actual, tampered, approved_differences=approved_paths)
    assert diffs_filtered == [], "白名单应完全过滤已知差异"

