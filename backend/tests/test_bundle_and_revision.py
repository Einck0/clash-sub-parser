from __future__ import annotations

import json
import pytest

from tests.conftest import TestSession
from app.schemas.logical_config import (
    LogicalConfigurationBundle,
    LogicalGroupConfig,
    LogicalRuleCategory,
    LogicalRuleConfig,
    MembershipEntry,
)
from app.services.bundle_preflight_service import preflight_bundle_json
from app.services.revision_service import (
    cleanup_old_revisions,
    compute_bundle_checksum,
    create_and_activate_revision,
    get_active_revision,
    list_revision_history,
    rollback_to_revision,
)


def test_bundle_json_roundtrip_and_checksum_determinism():
    bundle = LogicalConfigurationBundle(
        logical_id="bundle-rt-001",
        name="roundtrip_test",
        groups=[
            LogicalGroupConfig(
                logical_id="g1",
                name="自动选择",
                group_type="url-test",
                include_entries=[MembershipEntry(entry_type="regex", value="香港")],
            )
        ],
        rule_categories=[LogicalRuleCategory(logical_id="c1", name="广告拦截")],
        rules=[
            LogicalRuleConfig(
                logical_id="r1",
                category_logical_id="c1",
                rule_type="DOMAIN-SUFFIX",
                payload="doubleclick.net",
                target_direct_or_reject="REJECT",
            )
        ],
    )

    # 序列化为 JSON 并反序列化
    raw_json = bundle.model_dump_json()
    loaded_dict = json.loads(raw_json)
    restored = LogicalConfigurationBundle.model_validate(loaded_dict)

    assert restored.logical_id == bundle.logical_id
    assert len(restored.groups) == 1
    assert restored.groups[0].name == "自动选择"
    assert restored.rules[0].payload == "doubleclick.net"

    # 验证校验和确定性
    sum1 = compute_bundle_checksum(bundle)
    sum2 = compute_bundle_checksum(restored)
    assert sum1 == sum2


def test_preflight_detects_blockers_and_secrets():
    # 构造包含私有敏感凭据的 Bundle
    leaked_dict = {
        "schema_version": 1,
        "logical_id": "b-leak",
        "name": "leak_test",
        "password": "supersecretpassword",
        "groups": [],
    }
    res_leak = preflight_bundle_json(leaked_dict)
    assert res_leak.valid is False
    codes = [b.code for b in res_leak.blockers]
    assert "SECRET_LEAK" in codes

    # 构造包含循环依赖的 Bundle
    cyclic_dict = {
        "schema_version": 1,
        "logical_id": "b-cycle",
        "groups": [
            {
                "logical_id": "g1",
                "name": "组1",
                "include_entries": [{"entry_type": "group_ref", "value": "g2"}],
            },
            {
                "logical_id": "g2",
                "name": "组2",
                "include_entries": [{"entry_type": "group_ref", "value": "g1"}],
            },
        ],
    }
    res_cycle = preflight_bundle_json(cyclic_dict)
    assert res_cycle.valid is False
    assert any(b.code == "CIRCULAR_DEPENDENCY" for b in res_cycle.blockers)


@pytest.mark.asyncio
async def test_revision_lifecycle_activation_and_rollback():
    async with TestSession() as session:
        b1 = LogicalConfigurationBundle(
            logical_id="b1",
            name="v1",
            groups=[LogicalGroupConfig(logical_id="g1", name="组1")],
        )
        b2 = LogicalConfigurationBundle(
            logical_id="b2",
            name="v2",
            groups=[LogicalGroupConfig(logical_id="g2", name="组2")],
        )

        # 1 创建并激活版本 1
        rev1 = await create_and_activate_revision(session, b1, created_by="admin")
        await session.commit()

        active = await get_active_revision(session)
        assert active is not None
        assert active.logical_id == rev1.logical_id
        assert active.is_active is True

        # 2 创建并激活版本 2
        rev2 = await create_and_activate_revision(session, b2, created_by="admin")
        await session.commit()

        active2 = await get_active_revision(session)
        assert active2 is not None
        assert active2.logical_id == rev2.logical_id

        # 验证 rev1 已被自动取消激活
        history = await list_revision_history(session)
        assert len(history) == 2
        rev1_in_db = next(r for r in history if r.logical_id == rev1.logical_id)
        assert rev1_in_db.is_active is False

        # 3 回滚至版本 1
        await rollback_to_revision(session, rev1.logical_id)
        await session.commit()

        active_after_rollback = await get_active_revision(session)
        assert active_after_rollback is not None
        assert active_after_rollback.logical_id == rev1.logical_id


@pytest.mark.asyncio
async def test_cleanup_old_revisions_protects_active():
    async with TestSession() as session:
        # 连续创建 5 个版本
        revisions = []
        for i in range(5):
            bundle = LogicalConfigurationBundle(
                logical_id=f"bundle-clean-{i}",
                name=f"v{i}",
                groups=[LogicalGroupConfig(logical_id=f"g{i}", name=f"组{i}")],
            )
            rev = await create_and_activate_revision(session, bundle)
            revisions.append(rev)
        await session.commit()

        # 保留最近 2 个，验证清理数量
        deleted = await cleanup_old_revisions(session, retention_count=2)
        await session.commit()

        assert deleted == 3
        remaining = await list_revision_history(session)
        assert len(remaining) == 2
        # 当前激活版本（最新版本）得到妥善保护
        active = await get_active_revision(session)
        assert active is not None
        assert active.logical_id == revisions[-1].logical_id
