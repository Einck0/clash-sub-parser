"""add probe execution orchestration and dynamic cron columns

Revision ID: probe_orchestration_config
Revises: csp_target_domain_schema
"""
from __future__ import annotations

from alembic import op
import sqlalchemy as sa
from sqlalchemy import inspect, text


revision = "probe_orchestration_config"
down_revision = "csp_target_domain_schema"
branch_labels = None
depends_on = None


def _columns(table_name: str) -> set[str]:
    bind = op.get_bind()
    return {column["name"] for column in inspect(bind).get_columns(table_name)}


def upgrade() -> None:
    bind = op.get_bind()
    existing_cols = _columns("probe_config")

    if "probe_concurrency" not in existing_cols:
        op.add_column(
            "probe_config",
            sa.Column("probe_concurrency", sa.Integer(), nullable=False, server_default=sa.text("10")),
        )
    else:
        # Backfill NULL concurrency with 10 without overwriting explicit saved values
        bind.execute(
            text("UPDATE probe_config SET probe_concurrency = 10 WHERE probe_concurrency IS NULL")
        )

    if "probe_service_timeout_ms" not in existing_cols:
        op.add_column(
            "probe_config",
            sa.Column("probe_service_timeout_ms", sa.Integer(), nullable=False, server_default=sa.text("2000")),
        )
    else:
        bind.execute(
            text("UPDATE probe_config SET probe_service_timeout_ms = 2000 WHERE probe_service_timeout_ms IS NULL")
        )

    if "probe_cron_enabled" not in existing_cols:
        op.add_column(
            "probe_config",
            sa.Column("probe_cron_enabled", sa.Boolean(), nullable=False, server_default=sa.text("'1'")),
        )
    else:
        bind.execute(
            text("UPDATE probe_config SET probe_cron_enabled = '1' WHERE probe_cron_enabled IS NULL")
        )

    if "probe_cron_interval_minutes" not in existing_cols:
        op.add_column(
            "probe_config",
            sa.Column("probe_cron_interval_minutes", sa.Integer(), nullable=False, server_default=sa.text("60")),
        )
    else:
        bind.execute(
            text("UPDATE probe_config SET probe_cron_interval_minutes = 60 WHERE probe_cron_interval_minutes IS NULL")
        )


def downgrade() -> None:
    existing_cols = _columns("probe_config")
    cols_to_drop = [
        col for col in (
            "probe_cron_interval_minutes",
            "probe_cron_enabled",
            "probe_service_timeout_ms",
        )
        if col in existing_cols
    ]
    if cols_to_drop:
        with op.batch_alter_table("probe_config") as batch_op:
            for col in cols_to_drop:
                batch_op.drop_column(col)
