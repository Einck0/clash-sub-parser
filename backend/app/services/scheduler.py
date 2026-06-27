import asyncio

from apscheduler.schedulers.asyncio import AsyncIOScheduler

from app.config import get_settings
from app.database import AsyncSessionLocal
from app.services.subscription_service import fetch_due_subscriptions

scheduler = AsyncIOScheduler()
settings = get_settings()
_lock = asyncio.Lock()


async def _poll_subscriptions() -> None:
    if _lock.locked():
        return
    async with _lock:
        try:
            async with AsyncSessionLocal() as session:
                await fetch_due_subscriptions(session)
        except Exception:
            import logging
            logging.getLogger(__name__).exception("Scheduled subscription poll failed")


def start_scheduler() -> None:
    if not settings.scheduler_enabled:
        return
    if scheduler.running:
        return
    scheduler.add_job(
        _poll_subscriptions,
        "interval",
        minutes=1,
        id="subscription-poller",
        replace_existing=True,
        max_instances=1,
    )
    scheduler.start()


def shutdown_scheduler() -> None:
    if scheduler.running:
        scheduler.shutdown(wait=False)
