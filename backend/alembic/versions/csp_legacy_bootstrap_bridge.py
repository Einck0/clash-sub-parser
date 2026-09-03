"""bridge runtime bootstrap schema

Revision ID: csp_legacy_bootstrap_bridge
Revises: csp_legacy_schema_baseline
"""

from alembic import op
import sqlalchemy as sa
from sqlalchemy import inspect, text


revision = "csp_legacy_bootstrap_bridge"
down_revision = "csp_legacy_schema_baseline"
branch_labels = None
depends_on = None


def upgrade() -> None:
    bind = op.get_bind()
    _add_columns(
        "node_groups",
        [
            sa.Column("filter_min_speed_mbps", sa.Float(), nullable=True),
            sa.Column("filter_media_unlock", sa.JSON(), nullable=False, server_default=sa.text("'[]'")),
            sa.Column("exclude_group_ids", sa.JSON(), nullable=False, server_default=sa.text("'[]'")),
        ],
    )
    _add_columns(
        "subscriptions",
        [
            sa.Column("filter_min_speed_mbps", sa.Float(), nullable=True),
            sa.Column("filter_media_unlock", sa.JSON(), nullable=False, server_default=sa.text("'[]'")),
        ],
    )
    if "ix_subscriptions_enabled" not in _indexes("subscriptions"):
        op.create_index("ix_subscriptions_enabled", "subscriptions", ["enabled"])
    categories = bind.execute(
        text(
            "SELECT category, MIN(sort_order) AS first_order "
            "FROM rules WHERE category IS NOT NULL AND category != '' "
            "GROUP BY category ORDER BY first_order ASC, category ASC"
        )
    ).fetchall()
    existing = {
        row[0]
        for row in bind.execute(text("SELECT name FROM rule_categories")).fetchall()
    }
    for index, (name, _) in enumerate(categories):
        if name not in existing:
            bind.execute(
                text("INSERT INTO rule_categories (name, sort_order) VALUES (:name, :sort_order)"),
                {"name": name, "sort_order": index * 10},
            )


def downgrade() -> None:
    columns = _columns("subscriptions")
    for column_name in ("filter_media_unlock", "filter_min_speed_mbps"):
        if column_name in columns:
            op.drop_column("subscriptions", column_name)
    columns = _columns("node_groups")
    for column_name in ("exclude_group_ids", "filter_media_unlock", "filter_min_speed_mbps"):
        if column_name in columns:
            op.drop_column("node_groups", column_name)


def _add_columns(table_name: str, columns: list[sa.Column]) -> None:
    existing = _columns(table_name)
    for column in columns:
        if column.name not in existing:
            op.add_column(table_name, column)


def _indexes(table_name: str) -> set[str]:
    return {
        index["name"]
        for index in inspect(op.get_bind()).get_indexes(table_name)
        if index["name"] is not None
    }


def _columns(table_name: str) -> set[str]:
    return {column["name"] for column in inspect(op.get_bind()).get_columns(table_name)}
