import json
from datetime import datetime, timezone
from sqlalchemy import DateTime, select, desc
from sqlalchemy.ext.asyncio import AsyncSession
from app.models.config_snapshot import ConfigSnapshot
from app.models.dns import DnsConfig
from app.models.generate_config import GenerateConfig
from app.models.node_group import NodeGroup
from app.models.rule import Rule
from app.models.rule_category import RuleCategory
from app.models.subscription import Subscription
from app.models.security_settings import SecuritySettings

KEEP_COUNT = 50  # Maximum snapshots to keep

async def create_snapshot(db: AsyncSession, label: str = "", description: str = "") -> ConfigSnapshot:
    """Capture current config state as a snapshot."""
    data = {"version": 1, "tables": {}}
    models = {
        "subscriptions": Subscription,
        "node_groups": NodeGroup,
        "rules": Rule,
        "rule_categories": RuleCategory,
        "dns_config": DnsConfig,
        "generate_config": GenerateConfig,
    }
    for table_name, model in models.items():
        result = await db.execute(select(model))
        data["tables"][table_name] = []
        for item in result.scalars().all():
            row = {}
            for col in item.__table__.columns:
                val = getattr(item, col.name)
                if isinstance(val, datetime):
                    val = val.isoformat()
                row[col.name] = val
            data["tables"][table_name].append(row)

    snapshot = ConfigSnapshot(
        label=label,
        description=description,
        snapshot_data=json.dumps(data, ensure_ascii=False),
    )
    db.add(snapshot)
    await db.commit()
    await db.refresh(snapshot)

    # Prune old snapshots
    count_result = await db.execute(select(ConfigSnapshot.id).order_by(desc(ConfigSnapshot.id)))
    all_ids = [row[0] for row in count_result.all()]
    if len(all_ids) > KEEP_COUNT:
        old_ids = all_ids[KEEP_COUNT:]
        await db.execute(
            ConfigSnapshot.__table__.delete().where(ConfigSnapshot.id.in_(old_ids))
        )
        await db.commit()

    return snapshot


async def list_snapshots(db: AsyncSession, limit: int = 30) -> list[ConfigSnapshot]:
    result = await db.execute(
        select(ConfigSnapshot).order_by(desc(ConfigSnapshot.id)).limit(limit)
    )
    return list(result.scalars().all())


async def get_snapshot(db: AsyncSession, snapshot_id: int) -> ConfigSnapshot | None:
    return await db.get(ConfigSnapshot, snapshot_id)


async def restore_snapshot(db: AsyncSession, snapshot_id: int) -> dict:
    """Restore config from a snapshot. Returns imported counts."""
    snapshot = await db.get(ConfigSnapshot, snapshot_id)
    if not snapshot:
        raise ValueError("Snapshot not found")

    data = json.loads(snapshot.snapshot_data)
    tables = data.get("tables", {})

    from sqlalchemy import delete
    imported = {}

    # Delete in reverse dependency order
    for model in (Rule, RuleCategory, NodeGroup, Subscription, DnsConfig, GenerateConfig):
        await db.execute(delete(model))

    # Import tables
    table_model_map = {
        "subscriptions": Subscription,
        "node_groups": NodeGroup,
        "rules": Rule,
        "rule_categories": RuleCategory,
        "dns_config": DnsConfig,
        "generate_config": GenerateConfig,
    }
    import_order = ["subscriptions", "rule_categories", "node_groups", "rules", "dns_config", "generate_config"]

    for table_name in import_order:
        rows = tables.get(table_name, [])
        model = table_model_map.get(table_name)
        if not model or not rows:
            imported[table_name] = len(rows)
            continue
        datetime_columns = {col.name for col in model.__table__.columns if isinstance(col.type, DateTime)}
        for row_data in rows:
            filtered = {}
            for k, v in row_data.items():
                if k in {"id"}:
                    continue
                if k in datetime_columns and isinstance(v, str):
                    try:
                        v = datetime.fromisoformat(v)
                    except ValueError:
                        pass
                filtered[k] = v
            db.add(model(**filtered))
        imported[table_name] = len(rows)

    await db.commit()
    return {"ok": True, "imported": imported, "snapshot_id": snapshot_id}
