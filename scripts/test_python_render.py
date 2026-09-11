import asyncio
import os
import sys

# Ensure backend is in path
sys.path.insert(0, os.path.abspath("backend"))

from sqlalchemy.ext.asyncio import create_async_engine, async_sessionmaker, AsyncSession
from app.services.generate_service import render_current

async def main():
    db_path = os.path.abspath("backups/clash_sub_parser_pre_go_rewrite.db")
    db_url = f"sqlite+aiosqlite:///{db_path}?mode=ro"
    engine = create_async_engine(db_url)
    session_factory = async_sessionmaker(engine, class_=AsyncSession, expire_on_commit=False)

    async with session_factory() as session:
        for target in ["clash", "mihomo", "sing-box", "shadowrocket"]:
            try:
                out = await render_current(session, target)
                print(f"Target {target}: output length {len(out)} chars")
            except Exception as e:
                print(f"Target {target} ERROR: {e}")

    await engine.dispose()

if __name__ == "__main__":
    asyncio.run(main())
