from __future__ import annotations

from typing import Any, cast
import uuid
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

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
    LogicalRuleCategory,
    LogicalRuleConfig,
    MembershipEntry,
)


async def build_logical_bundle_from_db(
    session: AsyncSession,
    bundle_name: str = "imported_legacy_bundle",
) -> LogicalConfigurationBundle:
    """从传统数据库 ORM 表全量提取并构建纯净逻辑配置 Bundle"""
    # 1 策略组
    ng_res = await session.execute(select(NodeGroup).order_by(NodeGroup.sort_order.asc(), NodeGroup.id.asc()))
    node_groups = ng_res.scalars().all()

    logical_groups: list[LogicalGroupConfig] = []
    for g in node_groups:
        entries: list[MembershipEntry] = []
        if g.include_entries:
            for idx, item in enumerate(g.include_entries):
                etype = item.get("type") or "node_name"
                val = str(item.get("value") or "")
                if etype in ("node_name", "regex", "group_ref", "source_ref", "exclude_node", "exclude_group") and val:
                    entries.append(MembershipEntry(entry_type=cast(Any, etype), value=val, order_idx=idx))
        else:
            # 兼容旧字段
            order_idx = 0
            for name in g.include_nodes or []:
                entries.append(MembershipEntry(entry_type="node_name", value=name, order_idx=order_idx))
                order_idx += 1
            for gid in g.include_group_ids or []:
                entries.append(MembershipEntry(entry_type="group_ref", value=str(gid), order_idx=order_idx))
                order_idx += 1
            for rule in g.regex_rules or []:
                entries.append(MembershipEntry(entry_type="regex", value=rule, order_idx=order_idx))
                order_idx += 1

        g_type = cast(Any, g.group_type if g.group_type in ("select", "url-test", "fallback", "load-balance") else "select")

        logical_groups.append(
            LogicalGroupConfig(
                logical_id=str(g.id),
                name=g.name,
                group_type=g_type,
                sort_order=g.sort_order or 0,
                add_fallback=bool(g.add_fallback),
                include_entries=entries,
                exclude_nodes=g.exclude_nodes or [],
                filter_min_speed_mbps=g.filter_min_speed_mbps,
                filter_media_unlock=g.filter_media_unlock or [],
                url_test_config=g.url_test_config,
                load_balance_config=g.load_balance_config,
                fallback_config=g.fallback_config,
            )
        )

    # 2 规则分类
    cat_res = await session.execute(select(RuleCategory).order_by(RuleCategory.sort_order.asc(), RuleCategory.id.asc()))
    categories = cat_res.scalars().all()
    logical_categories: list[LogicalRuleCategory] = [
        LogicalRuleCategory(
            logical_id=str(c.id),
            name=c.name,
            sort_order=c.sort_order or 0,
            enabled=True,
        )
        for c in categories
    ]

    # 3 规则
    rule_res = await session.execute(select(Rule).order_by(Rule.sort_order.asc(), Rule.id.asc()))
    rules = rule_res.scalars().all()
    logical_rules: list[LogicalRuleConfig] = [
        LogicalRuleConfig(
            logical_id=str(r.id),
            category_logical_id=r.category,
            rule_type=r.type or "DOMAIN-SUFFIX",
            payload=r.value or "",
            target_group_logical_id=r.proxy if r.proxy not in ("DIRECT", "REJECT") else None,
            target_direct_or_reject=r.proxy if r.proxy in ("DIRECT", "REJECT") else None,
            sort_order=r.sort_order or 0,
            enabled=bool(r.enabled),
        )
        for r in rules
    ]

    # 4 DNS
    dns_res = await session.execute(select(DnsConfig).where(DnsConfig.id == 1))
    dns_item = dns_res.scalar_one_or_none()
    logical_dns = LogicalDnsConfig(
        raw_yaml=dns_item.raw_yaml if dns_item else "",
        enabled=dns_item.enabled if dns_item else True,
    )

    # 5 生成设置
    gen_res = await session.execute(select(GenerateConfig).where(GenerateConfig.id == 1))
    gen_item = gen_res.scalar_one_or_none()
    switches: dict[str, Any] = {}
    if gen_item:
        switches = {
            "enabled": gen_item.enabled,
            "subscriptions": gen_item.subscriptions,
            "node_groups": gen_item.node_groups,
            "rules": gen_item.rules,
            "dns": gen_item.dns,
            "exclude_node_proxies": gen_item.exclude_node_proxies,
        }
    logical_generate = LogicalGenerateSettings(switches=switches)

    return LogicalConfigurationBundle(
        schema_version=1,
        logical_id=str(uuid.uuid4()),
        name=bundle_name,
        groups=logical_groups,
        rule_categories=logical_categories,
        rules=logical_rules,
        dns=logical_dns,
        generate=logical_generate,
    )
