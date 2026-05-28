from apscheduler.schedulers.asyncio import AsyncIOScheduler

from app.config import get_settings
from app.database import AsyncSessionLocal
from app.services.subscription_service import fetch_due_subscriptions

scheduler = AsyncIOScheduler()
settings = get_settings()


async def _poll_subscriptions() -> None:
    async with AsyncSessionLocal() as session:
        await fetch_due_subscriptions(session)


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
    )
    scheduler.start()


def shutdown_scheduler() -> None:
    if scheduler.running:
        scheduler.shutdown(wait=False)
