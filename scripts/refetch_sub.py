#!/usr/bin/env python3
"""Force refetch one subscription by id inside the app container."""
from __future__ import annotations

import asyncio
import sys

sys.path.insert(0, "/app")

from app.database import AsyncSessionLocal
from app.services.subscription_service import fetch_subscription_nodes, get_subscription


async def main() -> None:
    sub_id = int(sys.argv[1]) if len(sys.argv) > 1 else 1
    async with AsyncSessionLocal() as db:
        item = await get_subscription(db, sub_id)
        if item is None:
            print("missing", sub_id)
            return
        print(
            "before",
            item.name,
            "source",
            len(item.source_nodes or []),
            "raw",
            len(item.raw_nodes or []),
        )
        live = await fetch_subscription_nodes(db, item)
        print(
            "after",
            live.name,
            "source",
            len(live.source_nodes or []),
            "raw",
            len(live.raw_nodes or []),
        )
        print("types", sorted({n.get("type") for n in (live.source_nodes or [])}))
        print("head", [n.get("name") for n in (live.raw_nodes or [])[:8]])


if __name__ == "__main__":
    asyncio.run(main())
