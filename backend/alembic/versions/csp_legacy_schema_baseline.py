"""add missing legacy metadata tables

Revision ID: csp_legacy_schema_baseline
Revises: 8f5c2d1a7b90
"""

from alembic import op
import sqlalchemy as sa
from sqlalchemy import inspect


revision = "csp_legacy_schema_baseline"
down_revision = "8f5c2d1a7b90"
branch_labels = None
depends_on = None


def upgrade() -> None:
    tables = set(inspect(op.get_bind()).get_table_names())
    if "node_probe_results" not in tables:
        op.create_table(
            "node_probe_results",
            sa.Column("id", sa.Integer(), nullable=False),
            sa.Column("node_key", sa.String(length=255), nullable=False),
            sa.Column("name", sa.String(length=255), nullable=False),
            sa.Column("server", sa.String(length=255), nullable=False),
            sa.Column("port", sa.Integer(), nullable=True),
            sa.Column("type", sa.String(length=64), nullable=False),
            sa.Column("status", sa.String(length=32), nullable=False),
            sa.Column("latency_ms", sa.Integer(), nullable=True),
            sa.Column("speed_mbps", sa.Float(), nullable=True),
            sa.Column("ip", sa.String(length=128), nullable=True),
            sa.Column("country", sa.String(length=32), nullable=True),
            sa.Column("asn", sa.Integer(), nullable=True),
            sa.Column("organization", sa.String(length=255), nullable=True),
            sa.Column("media", sa.JSON(), nullable=False),
            sa.Column("error", sa.Text(), nullable=True),
            sa.Column("checked_at", sa.Integer(), nullable=False),
            sa.PrimaryKeyConstraint("id"),
        )
        op.create_index("ix_node_probe_results_checked_at", "node_probe_results", ["checked_at"])
        op.create_index("ix_node_probe_results_name", "node_probe_results", ["name"])
        op.create_index("ix_node_probe_results_node_key", "node_probe_results", ["node_key"], unique=True)
        op.create_index("ix_node_probe_results_server", "node_probe_results", ["server"])
    if "probe_config" not in tables:
        op.create_table(
            "probe_config",
            sa.Column("id", sa.Integer(), nullable=False),
            sa.Column("probe_enabled", sa.Boolean(), nullable=False),
            sa.Column("probe_interval_minutes", sa.Integer(), nullable=False),
            sa.Column("speedtest_enabled", sa.Boolean(), nullable=False),
            sa.Column("speedtest_url", sa.String(length=512), nullable=False),
            sa.Column("speedtest_timeout_s", sa.Integer(), nullable=False),
            sa.Column("speedtest_max_bytes", sa.Integer(), nullable=False),
            sa.Column("speedtest_min_speed_mbps", sa.Float(), nullable=False),
            sa.Column("media_check_enabled", sa.Boolean(), nullable=False),
            sa.Column("media_platforms", sa.JSON(), nullable=False),
            sa.Column("media_timeout_s", sa.Integer(), nullable=False),
            sa.Column("probe_concurrency", sa.Integer(), nullable=False),
            sa.Column("probe_timeout_ms", sa.Integer(), nullable=False),
            sa.PrimaryKeyConstraint("id"),
        )
    if "proxy_chain_bindings" not in tables:
        op.create_table(
            "proxy_chain_bindings",
            sa.Column("id", sa.Integer(), nullable=False),
            sa.Column("target_type", sa.String(length=20), nullable=False),
            sa.Column("target_id", sa.Integer(), nullable=True),
            sa.Column("target_name", sa.String(length=255), nullable=True),
            sa.Column("dialer_type", sa.String(length=20), nullable=False),
            sa.Column("dialer_ref", sa.String(length=255), nullable=False),
            sa.Column("enabled", sa.Boolean(), nullable=False),
            sa.Column("sort_order", sa.Integer(), nullable=False),
            sa.Column("note", sa.Text(), nullable=True),
            sa.PrimaryKeyConstraint("id"),
        )
        op.create_index("ix_proxy_chain_bindings_id", "proxy_chain_bindings", ["id"])
        op.create_index("ix_proxy_chain_bindings_target_id", "proxy_chain_bindings", ["target_id"])
        op.create_index("ix_proxy_chain_bindings_target_type", "proxy_chain_bindings", ["target_type"])


def downgrade() -> None:
    tables = set(inspect(op.get_bind()).get_table_names())
    for table_name in ("proxy_chain_bindings", "probe_config", "node_probe_results"):
        if table_name in tables:
            op.drop_table(table_name)
