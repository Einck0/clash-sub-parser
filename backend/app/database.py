from collections.abc import AsyncGenerator

from sqlalchemy import text
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker, create_async_engine
from sqlalchemy.orm import DeclarativeBase

from app.config import get_settings


class Base(DeclarativeBase):
    pass


settings = get_settings()
engine = create_async_engine(settings.database_url, echo=False)
AsyncSessionLocal = async_sessionmaker(
    engine, class_=AsyncSession, expire_on_commit=False
)


async def get_db() -> AsyncGenerator[AsyncSession, None]:
    async with AsyncSessionLocal() as session:
        yield session


async def init_db() -> None:
    from app.models import dns, generate_config, node_group, rule, rule_category, security_settings, subscription  # noqa: F401

    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)
        await _bootstrap_schema(conn)


async def _bootstrap_schema(conn) -> None:
    dialect = conn.dialect.name
    if dialect == "sqlite":
        result = await conn.execute(text("PRAGMA table_info(node_groups)"))
        columns = {row[1] for row in result.fetchall()}
        if "include_group_nodes_ids" not in columns:
            await conn.execute(
                text(
                    "ALTER TABLE node_groups ADD COLUMN include_group_nodes_ids JSON NOT NULL DEFAULT '[]'"
                )
            )
        if "include_entries" not in columns:
            await conn.execute(
                text(
                    "ALTER TABLE node_groups ADD COLUMN include_entries JSON NOT NULL DEFAULT '[]'"
                )
            )
        if "add_fallback" not in columns:
            await conn.execute(
                text(
                    "ALTER TABLE node_groups ADD COLUMN add_fallback BOOLEAN NOT NULL DEFAULT 1"
                )
            )

        rule_result = await conn.execute(text("PRAGMA table_info(rules)"))
        rule_columns = {row[1] for row in rule_result.fetchall()}
        if "category" not in rule_columns:
            await conn.execute(
                text(
                    "ALTER TABLE rules ADD COLUMN category VARCHAR(80) NOT NULL DEFAULT 'default'"
                )
            )
        await _bootstrap_rule_categories(conn)

        sub_result = await conn.execute(text("PRAGMA table_info(subscriptions)"))
        sub_columns = {row[1] for row in sub_result.fetchall()}
        if "last_fetch_error" not in sub_columns:
            await conn.execute(
                text("ALTER TABLE subscriptions ADD COLUMN last_fetch_error TEXT")
            )
        if "fetch_failed_count" not in sub_columns:
            await conn.execute(
                text(
                    "ALTER TABLE subscriptions ADD COLUMN fetch_failed_count INTEGER NOT NULL DEFAULT 0"
                )
            )
        if "subscription_userinfo" not in sub_columns:
            await conn.execute(
                text("ALTER TABLE subscriptions ADD COLUMN subscription_userinfo TEXT")
            )
        if "profile_update_interval" not in sub_columns:
            await conn.execute(
                text("ALTER TABLE subscriptions ADD COLUMN profile_update_interval VARCHAR(40)")
            )
        if "profile_web_page_url" not in sub_columns:
            await conn.execute(
                text("ALTER TABLE subscriptions ADD COLUMN profile_web_page_url TEXT")
            )
        if "include_node_names" not in sub_columns:
            await conn.execute(
                text("ALTER TABLE subscriptions ADD COLUMN include_node_names JSON NOT NULL DEFAULT '[]'")
            )
        if "exclude_node_names" not in sub_columns:
            await conn.execute(
                text("ALTER TABLE subscriptions ADD COLUMN exclude_node_names JSON NOT NULL DEFAULT '[]'")
            )
        if "source_nodes" not in sub_columns:
            await conn.execute(
                text("ALTER TABLE subscriptions ADD COLUMN source_nodes JSON NOT NULL DEFAULT '[]'")
            )
        if "manual_nodes" not in sub_columns:
            await conn.execute(
                text("ALTER TABLE subscriptions ADD COLUMN manual_nodes JSON NOT NULL DEFAULT '[]'")
            )

        sec_result = await conn.execute(text("PRAGMA table_info(security_settings)"))
        sec_columns = {row[1] for row in sec_result.fetchall()}
        if "fetch_proxy_enabled" not in sec_columns:
            await conn.execute(
                text("ALTER TABLE security_settings ADD COLUMN fetch_proxy_enabled BOOLEAN NOT NULL DEFAULT 0")
            )
        if "fetch_proxy_url" not in sec_columns:
            await conn.execute(
                text("ALTER TABLE security_settings ADD COLUMN fetch_proxy_url VARCHAR(512) NOT NULL DEFAULT ''")
            )
    elif dialect == "postgresql":
        await conn.execute(
            text(
                "ALTER TABLE node_groups ADD COLUMN IF NOT EXISTS include_group_nodes_ids JSON NOT NULL DEFAULT '[]'"
            )
        )
        await conn.execute(
            text(
                "ALTER TABLE node_groups ADD COLUMN IF NOT EXISTS include_entries JSON NOT NULL DEFAULT '[]'"
            )
        )
        await conn.execute(
            text(
                "ALTER TABLE node_groups ADD COLUMN IF NOT EXISTS add_fallback BOOLEAN NOT NULL DEFAULT FALSE"
            )
        )
        await conn.execute(
            text(
                "ALTER TABLE rules ADD COLUMN IF NOT EXISTS category VARCHAR(80) NOT NULL DEFAULT 'default'"
            )
        )
        await _bootstrap_rule_categories(conn)

        await conn.execute(
            text("ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS last_fetch_error TEXT")
        )
        await conn.execute(
            text(
                "ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS fetch_failed_count INTEGER NOT NULL DEFAULT 0"
            )
        )
        await conn.execute(
            text("ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS subscription_userinfo TEXT")
        )
        await conn.execute(
            text("ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS profile_update_interval VARCHAR(40)")
        )
        await conn.execute(
            text("ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS profile_web_page_url TEXT")
        )
        await conn.execute(
            text("ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS include_node_names JSON NOT NULL DEFAULT '[]'")
        )
        await conn.execute(
            text("ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS exclude_node_names JSON NOT NULL DEFAULT '[]'")
        )
        await conn.execute(
            text("ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS source_nodes JSON NOT NULL DEFAULT '[]'")
        )
        await conn.execute(
            text("ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS manual_nodes JSON NOT NULL DEFAULT '[]'")
        )
        await conn.execute(
            text("ALTER TABLE security_settings ADD COLUMN IF NOT EXISTS fetch_proxy_enabled BOOLEAN NOT NULL DEFAULT FALSE")
        )
        await conn.execute(
            text("ALTER TABLE security_settings ADD COLUMN IF NOT EXISTS fetch_proxy_url VARCHAR(512) NOT NULL DEFAULT ''")
        )


async def _bootstrap_rule_categories(conn) -> None:
    existing = await conn.execute(text("SELECT COUNT(*) FROM rule_categories"))
    if int(existing.scalar() or 0) > 0:
        return

    result = await conn.execute(
        text(
            "SELECT category, MIN(sort_order) AS first_order "
            "FROM rules WHERE category IS NOT NULL AND category != '' "
            "GROUP BY category ORDER BY first_order ASC, category ASC"
        )
    )
    for index, row in enumerate(result.fetchall()):
        await conn.execute(
            text(
                "INSERT INTO rule_categories (name, sort_order) "
                "VALUES (:name, :sort_order)"
            ),
            {"name": row[0], "sort_order": index * 10},
        )
