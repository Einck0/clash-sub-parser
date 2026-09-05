import asyncio
import logging
import time

from apscheduler.schedulers.asyncio import AsyncIOScheduler

from app.config import get_settings
from app.database import AsyncSessionLocal
from app.services.subscription_service import (
    collect_all_subscription_nodes,
    fetch_due_subscriptions,
)

logger = logging.getLogger(__name__)
scheduler = AsyncIOScheduler()
settings = get_settings()
_lock = asyncio.Lock()
_probe_lock = asyncio.Lock()
_last_probe_run_at: float = 0.0


async def _poll_subscriptions() -> None:
    if _lock.locked():
        return
    async with _lock:
        try:
            async with AsyncSessionLocal() as session:
                await fetch_due_subscriptions(session)
        except Exception:
            logger.exception("Scheduled subscription poll failed")


async def _poll_node_probes() -> None:
    global _last_probe_run_at
    if _probe_lock.locked():
        return
    async with _probe_lock:
        try:
            async with AsyncSessionLocal() as session:
                from app.services.probe.service import probe_batch_nodes
                from app.services.probe_settings_service import get_probe_config, resolve_probe_config

                config = await get_probe_config(session)
                # 总开关检查
                if not config.probe_enabled:
                    return
                # 定时质检开关检查（解耦旧 probe_interval_minutes）
                if not config.probe_cron_enabled:
                    return

                interval_minutes = config.probe_cron_interval_minutes or 60
                if interval_minutes <= 0:
                    return

                now = time.time()
                # 时钟回退防御：若系统时钟后退，重置基准线避免无限期抑制
                if now < _last_probe_run_at:
                    _last_probe_run_at = now

                # 启动保护：启动首次 tick 初始化基准线，禁止无间隔补跑冲击服务器
                if _last_probe_run_at == 0.0:
                    _last_probe_run_at = now
                    return

                interval_seconds = interval_minutes * 60
                if now - _last_probe_run_at < interval_seconds:
                    return

                all_nodes = await collect_all_subscription_nodes(session)
                if not all_nodes:
                    return

                logger.info(
                    "Starting periodic background probe for %d nodes (cron interval: %dm)",
                    len(all_nodes),
                    interval_minutes,
                )
                _last_probe_run_at = now
                resolved = resolve_probe_config(config)
                await probe_batch_nodes(all_nodes, resolved_config=resolved, db=session)
                logger.info("Completed periodic background probe")
        except Exception:
            logger.exception("Scheduled node probe task failed")


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
    scheduler.add_job(
        _poll_node_probes,
        "interval",
        minutes=1,
        id="node-probe-scheduler",
        replace_existing=True,
        max_instances=1,
    )
    scheduler.start()


def shutdown_scheduler() -> None:
    if scheduler.running:
        scheduler.shutdown(wait=False)
