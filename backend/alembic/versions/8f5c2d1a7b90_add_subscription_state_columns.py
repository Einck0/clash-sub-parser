"""add subscription state columns

Revision ID: 8f5c2d1a7b90
Revises: d49593de8ad6
"""

from alembic import op
import sqlalchemy as sa
from sqlalchemy import inspect


revision = "8f5c2d1a7b90"
down_revision = "d49593de8ad6"
branch_labels = None
depends_on = None


def upgrade() -> None:
    bind = op.get_bind()
    columns = {column["name"] for column in inspect(bind).get_columns("subscriptions")}

    if "enabled" not in columns:
        op.add_column(
            "subscriptions",
            sa.Column("enabled", sa.Boolean(), nullable=False, server_default=sa.true()),
        )

    if "node_renames" not in columns:
        op.add_column(
            "subscriptions",
            sa.Column(
                "node_renames",
                sa.JSON(),
                nullable=False,
                server_default=sa.text("'{}'"),
            ),
        )


def downgrade() -> None:
    bind = op.get_bind()
    columns = {column["name"] for column in inspect(bind).get_columns("subscriptions")}

    if "node_renames" in columns:
        op.drop_column("subscriptions", "node_renames")
    if "enabled" in columns:
        op.drop_column("subscriptions", "enabled")
