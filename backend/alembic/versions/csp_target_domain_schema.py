"""create target domain schema tables

Revision ID: csp_target_domain_schema
Revises: csp_legacy_bootstrap_bridge
"""
from __future__ import annotations

from alembic import op
import sqlalchemy as sa


revision = "csp_target_domain_schema"
down_revision = "csp_legacy_bootstrap_bridge"
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.create_table(
        "sources",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("logical_id", sa.String(length=36), nullable=False),
        sa.Column("name", sa.String(length=120), nullable=False),
        sa.Column("kind", sa.String(length=32), nullable=False),
        sa.Column("url", sa.Text(), nullable=True),
        sa.Column("update_interval", sa.Integer(), nullable=True),
        sa.Column("enabled", sa.Boolean(), nullable=False, server_default=sa.text("'1'")),
        sa.Column("created_at", sa.DateTime(), nullable=False),
        sa.Column("updated_at", sa.DateTime(), nullable=False),
    )
    op.create_index("ix_sources_logical_id", "sources", ["logical_id"], unique=True)

    op.create_table(
        "source_revisions",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("source_id", sa.Integer(), sa.ForeignKey("sources.id", ondelete="CASCADE"), nullable=False),
        sa.Column("revision_id", sa.String(length=36), nullable=False),
        sa.Column("status", sa.String(length=32), nullable=False),
        sa.Column("payload_hash", sa.String(length=64), nullable=False),
        sa.Column("node_count", sa.Integer(), nullable=False, server_default=sa.text("0")),
        sa.Column("error_summary", sa.Text(), nullable=True),
        sa.Column("fetched_at", sa.DateTime(), nullable=True),
        sa.Column("created_at", sa.DateTime(), nullable=False),
    )
    op.create_index("ix_source_revisions_source_id", "source_revisions", ["source_id"])
    op.create_index("ix_source_revisions_revision_id", "source_revisions", ["revision_id"], unique=True)

    op.create_table(
        "nodes",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("logical_id", sa.String(length=36), nullable=False),
        sa.Column("name", sa.String(length=255), nullable=False),
        sa.Column("protocol", sa.String(length=64), nullable=False),
        sa.Column("server", sa.String(length=255), nullable=False),
        sa.Column("port", sa.Integer(), nullable=False),
        sa.Column("normalized_payload", sa.JSON(), nullable=False),
        sa.Column("payload_fingerprint", sa.String(length=64), nullable=False),
        sa.Column("lifecycle_state", sa.String(length=32), nullable=False, server_default=sa.text("'active'")),
        sa.Column("created_at", sa.DateTime(), nullable=False),
        sa.Column("updated_at", sa.DateTime(), nullable=False),
    )
    op.create_index("ix_nodes_logical_id", "nodes", ["logical_id"], unique=True)
    op.create_index("ix_nodes_payload_fingerprint", "nodes", ["payload_fingerprint"])

    op.create_table(
        "node_source_links",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("source_revision_id", sa.Integer(), sa.ForeignKey("source_revisions.id", ondelete="CASCADE"), nullable=False),
        sa.Column("node_id", sa.Integer(), sa.ForeignKey("nodes.id", ondelete="CASCADE"), nullable=False),
        sa.Column("display_name", sa.String(length=255), nullable=False),
        sa.Column("original_order", sa.Integer(), nullable=False, server_default=sa.text("0")),
        sa.Column("created_at", sa.DateTime(), nullable=False),
    )
    op.create_index("ix_node_source_links_source_revision_id", "node_source_links", ["source_revision_id"])
    op.create_index("ix_node_source_links_node_id", "node_source_links", ["node_id"])

    op.create_table(
        "configuration_revisions",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("logical_id", sa.String(length=36), nullable=False),
        sa.Column("schema_version", sa.Integer(), nullable=False, server_default=sa.text("1")),
        sa.Column("bundle_json", sa.JSON(), nullable=False),
        sa.Column("checksum", sa.String(length=64), nullable=False),
        sa.Column("is_active", sa.Boolean(), nullable=False, server_default=sa.text("'0'")),
        sa.Column("created_at", sa.DateTime(), nullable=False),
        sa.Column("created_by", sa.String(length=100), nullable=False, server_default=sa.text("'system'")),
    )
    op.create_index("ix_configuration_revisions_logical_id", "configuration_revisions", ["logical_id"], unique=True)
    op.create_index("ix_configuration_revisions_is_active", "configuration_revisions", ["is_active"])

    op.create_table(
        "probe_profiles",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("logical_id", sa.String(length=36), nullable=False),
        sa.Column("name", sa.String(length=120), nullable=False),
        sa.Column("platforms_json", sa.JSON(), nullable=False),
        sa.Column("timeout_s", sa.Integer(), nullable=False, server_default=sa.text("10")),
        sa.Column("concurrency", sa.Integer(), nullable=False, server_default=sa.text("5")),
        sa.Column("max_bytes", sa.Integer(), nullable=False, server_default=sa.text("50000000")),
        sa.Column("min_speed_mbps", sa.Float(), nullable=True),
        sa.Column("created_at", sa.DateTime(), nullable=False),
        sa.Column("updated_at", sa.DateTime(), nullable=False),
    )
    op.create_index("ix_probe_profiles_logical_id", "probe_profiles", ["logical_id"], unique=True)
    op.create_index("ix_probe_profiles_name", "probe_profiles", ["name"], unique=True)

    op.create_table(
        "probe_jobs",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("logical_id", sa.String(length=36), nullable=False),
        sa.Column("profile_id", sa.Integer(), sa.ForeignKey("probe_profiles.id", ondelete="CASCADE"), nullable=False),
        sa.Column("trigger_type", sa.String(length=32), nullable=False),
        sa.Column("status", sa.String(length=32), nullable=False),
        sa.Column("queued_at", sa.DateTime(), nullable=False),
        sa.Column("started_at", sa.DateTime(), nullable=True),
        sa.Column("finished_at", sa.DateTime(), nullable=True),
        sa.Column("summary_json", sa.JSON(), nullable=True),
    )
    op.create_index("ix_probe_jobs_logical_id", "probe_jobs", ["logical_id"], unique=True)
    op.create_index("ix_probe_jobs_profile_id", "probe_jobs", ["profile_id"])

    op.create_table(
        "probe_observations",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("job_id", sa.Integer(), sa.ForeignKey("probe_jobs.id", ondelete="CASCADE"), nullable=False),
        sa.Column("node_id", sa.Integer(), sa.ForeignKey("nodes.id", ondelete="CASCADE"), nullable=False),
        sa.Column("status", sa.String(length=32), nullable=False),
        sa.Column("latency_ms", sa.Integer(), nullable=True),
        sa.Column("speed_mbps", sa.Float(), nullable=True),
        sa.Column("egress_ip", sa.String(length=128), nullable=True),
        sa.Column("country", sa.String(length=32), nullable=True),
        sa.Column("asn", sa.Integer(), nullable=True),
        sa.Column("organization", sa.String(length=255), nullable=True),
        sa.Column("media_results_json", sa.JSON(), nullable=False, server_default=sa.text("'{}'")),
        sa.Column("error", sa.Text(), nullable=True),
        sa.Column("observed_at", sa.DateTime(), nullable=False),
    )
    op.create_index("ix_probe_observations_job_id", "probe_observations", ["job_id"])
    op.create_index("ix_probe_observations_node_id", "probe_observations", ["node_id"])
    op.create_index("ix_probe_observations_observed_at", "probe_observations", ["observed_at"])

    op.create_table(
        "quarantine_records",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("category", sa.String(length=64), nullable=False),
        sa.Column("source_table", sa.String(length=64), nullable=False),
        sa.Column("source_record_id", sa.String(length=64), nullable=True),
        sa.Column("payload_json", sa.JSON(), nullable=False),
        sa.Column("reason", sa.Text(), nullable=False),
        sa.Column("created_at", sa.DateTime(), nullable=False),
    )


def downgrade() -> None:
    op.drop_table("quarantine_records")
    op.drop_table("probe_observations")
    op.drop_table("probe_jobs")
    op.drop_table("probe_profiles")
    op.drop_table("configuration_revisions")
    op.drop_table("node_source_links")
    op.drop_table("nodes")
    op.drop_table("source_revisions")
    op.drop_table("sources")
