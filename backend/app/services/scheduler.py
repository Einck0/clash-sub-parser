import asyncio
import logging
import time
from typing import Any

from apscheduler.schedulers.asyncio import AsyncIOScheduler
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.config import get_settings
from app.database import AsyncSessionLocal
from app.models.subscription import Subscription
from app.services.node_identity import canonical_node_key
from app.services.subscription_service import fetch_due_subscriptions

logger = logging.getLogger(__name__)
scheduler = AsyncIOScheduler()
settings = get_settings()
_lock = asyncio.Lock()
_probe_lock = asyncio.Lock()
_last_probe_run_at: float = 0.0


class ProbeScheduleRuntime:
    """Single-owner in-memory runtime state manager for periodic background probing.

    Tracks scheduler lifecycle states (disabled, initializing, waiting, running, failed),
    execution timestamps, latest execution summaries, and sanitized error codes.
    """

    def __init__(self) -> None:
        self.state: str = "initializing"
        self.last_started_at: int | None = None
        self.last_finished_at: int | None = None
        self.last_summary: dict[str, int] | None = None
        self.last_error_code: str | None = None
        self._baseline_at: float = 0.0

    def reset_for_restart(self) -> None:
        """Reset state upon process restart (initializing)."""
        self.state = "initializing"
        self.last_started_at = None
        self.last_finished_at = None
        self.last_summary = None
        self.last_error_code = None
        self._baseline_at = 0.0

    def record_baseline(self, timestamp: float) -> None:
        """Establish startup baseline on first valid tick."""
        self._baseline_at = timestamp
        if self.state == "initializing":
            self.state = "waiting"

    def record_start(self, timestamp: float) -> None:
        """Record transition into running state before executing a probe run."""
        self.state = "running"
        self.last_started_at = int(timestamp)
        self.last_error_code = None

    def record_success(self, timestamp: float, summary: dict[str, int]) -> None:
        """Record successful completion of a periodic probe run."""
        self.state = "waiting"
        self.last_finished_at = int(timestamp)
        self.last_summary = {
            "total": int(summary.get("total", 0)),
            "ok": int(summary.get("ok", 0)),
            "fail": int(summary.get("fail", 0)),
            "timeout": int(summary.get("timeout", 0)),
            "skipped": int(summary.get("skipped", 0)),
        }
        self.last_error_code = None
        self._baseline_at = timestamp

    def record_failure(self, timestamp: float, error_code: str = "probe_run_failed") -> None:
        """Record failed probe run with stable sanitized error code."""
        self.state = "failed"
        self.last_finished_at = int(timestamp)
        self.last_error_code = error_code
        self._baseline_at = timestamp

    def get_status(
        self,
        config: Any = None,
        *,
        server_now: int | None = None,
    ) -> dict[str, Any]:
        """Produce the read-only projection dictionary conforming to design.md."""
        now_ts = int(time.time()) if server_now is None else server_now

        probe_enabled = getattr(config, "probe_enabled", True) if config is not None else True
        probe_cron_enabled = getattr(config, "probe_cron_enabled", True) if config is not None else True
        interval_minutes_val = getattr(config, "probe_cron_interval_minutes", 60) if config is not None else 60
        interval_minutes = int(interval_minutes_val) if interval_minutes_val is not None else 60

        is_disabled = (not probe_enabled) or (not probe_cron_enabled) or (interval_minutes <= 0)

        if is_disabled:
            effective_state = "disabled"
            next_expected_at = None
        elif self.state == "running" or _probe_lock.locked():
            effective_state = "running"
            next_expected_at = None
        elif self._baseline_at == 0.0:
            effective_state = "initializing"
            next_expected_at = None
        elif self.state == "failed":
            effective_state = "failed"
            next_expected_at = int(self._baseline_at + interval_minutes * 60)
        else:
            effective_state = "waiting"
            next_expected_at = int(self._baseline_at + interval_minutes * 60)

        return {
            "state": effective_state,
            "server_now": now_ts,
            "interval_minutes": interval_minutes if not is_disabled else (interval_minutes if interval_minutes > 0 else None),
            "next_expected_at": next_expected_at,
            "last_started_at": self.last_started_at,
            "last_finished_at": self.last_finished_at,
            "last_summary": self.last_summary,
            "last_error_code": self.last_error_code if effective_state == "failed" else None,
        }


probe_schedule_runtime = ProbeScheduleRuntime()


def get_probe_schedule_runtime() -> ProbeScheduleRuntime:
    return probe_schedule_runtime


async def collect_scheduler_inventory_nodes(db: AsyncSession) -> list[dict]:
    """Collect all raw_nodes from all enabled subscriptions without capability filtering,
    deduplicated by canonical node key.
    """
    result = await db.execute(
        select(Subscription).where(Subscription.enabled.is_(True))
    )
    merged: list[dict] = []
    seen: set[str] = set()
    for sub in result.scalars().all():
        for node in sub.raw_nodes or []:
            if not isinstance(node, dict):
                continue
            name = str(node.get("name") or "").strip()
            if not name:
                continue
            key = canonical_node_key(node)
            if key in seen:
                continue
            seen.add(key)
            merged.append(node)
    return merged


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
                    probe_schedule_runtime._baseline_at = now

                # 启动保护：启动首次 tick 初始化基准线，禁止无间隔补跑冲击服务器
                if _last_probe_run_at == 0.0 or probe_schedule_runtime._baseline_at == 0.0:
                    _last_probe_run_at = now
                    probe_schedule_runtime.record_baseline(now)
                    return

                interval_seconds = interval_minutes * 60
                if now - _last_probe_run_at < interval_seconds:
                    return

                all_nodes = await collect_scheduler_inventory_nodes(session)
                if not all_nodes:
                    return

                logger.info(
                    "Starting periodic background probe for %d nodes (cron interval: %dm)",
                    len(all_nodes),
                    interval_minutes,
                )
                _last_probe_run_at = now
                probe_schedule_runtime.record_start(now)
                resolved = resolve_probe_config(config)
                try:
                    res = await probe_batch_nodes(all_nodes, resolved_config=resolved, db=session)
                    summary = res.get("summary") if isinstance(res, dict) else None
                    if not summary:
                        results_list = res.get("results", []) if isinstance(res, dict) else []
                        summary = {
                            "total": len(all_nodes),
                            "ok": sum(1 for r in results_list if r.get("status") == "ok"),
                            "fail": sum(1 for r in results_list if r.get("status") == "fail"),
                            "timeout": sum(1 for r in results_list if r.get("status") == "timeout"),
                            "skipped": sum(1 for r in results_list if r.get("status") == "skipped"),
                        }
                    probe_schedule_runtime.record_success(time.time(), summary)
                    logger.info("Completed periodic background probe")
                except Exception:
                    probe_schedule_runtime.record_failure(time.time(), error_code="probe_run_failed")
                    raise
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
