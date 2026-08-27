from datetime import datetime, timezone
import logging
import re

import httpx
from fastapi import HTTPException
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm.exc import StaleDataError

from app.config import get_settings
from app.models.subscription import Subscription
from app.schemas.subscription import ManualNodeCreate, SubscriptionCreate, SubscriptionUpdate
from app.services.security_settings_service import get_fetch_proxy_config, get_security_settings
from app.utils.clash_parser import compile_regex, parse_node_links, parse_subscription_content
from app.utils.dedup import deduplicate_nodes
from app.utils.http_fetch import stream_fetch

settings = get_settings()
logger = logging.getLogger(__name__)


class SubscriptionFetchError(Exception):
    """订阅抓取领域异常，解耦调度器与 HTTP 传输层。"""

    def __init__(self, message: str, status_code: int = 502):
        super().__init__(message)
        self.message = message
        self.status_code = status_code



async def list_subscriptions(db: AsyncSession) -> list[Subscription]:
    result = await db.execute(select(Subscription).order_by(Subscription.id.asc()))
    return list(result.scalars().all())


async def get_subscription(
    db: AsyncSession, subscription_id: int
) -> Subscription | None:
    return await db.get(Subscription, subscription_id)


async def _require_subscription(
    db: AsyncSession, subscription_id: int
) -> Subscription:
    item = await db.get(Subscription, subscription_id)
    if item is None:
        raise HTTPException(status_code=404, detail="Subscription not found")
    return item


async def create_subscription(
    db: AsyncSession, payload: SubscriptionCreate
) -> Subscription:
    data = payload.model_dump()
    manual_node_links = data.pop("manual_node_links", None)
    data["filter_regex"] = _normalize_and_validate_regex(data.get("filter_regex", []))
    data["include_node_names"] = _normalize_node_names(data.get("include_node_names", []))
    data["exclude_node_names"] = _normalize_node_names(data.get("exclude_node_names", []))
    data["node_renames"] = _normalize_node_renames(data.get("node_renames", {}))
    data["manual_nodes"] = _merge_manual_nodes(data.get("manual_nodes") or [], manual_node_links)
    if data.get("is_primary"):
        await _clear_primary(db)
    item = Subscription(**data)
    _refresh_selected_nodes(item)
    db.add(item)
    await db.commit()
    await db.refresh(item)
    return item


async def create_manual_node_subscription(
    db: AsyncSession, payload: ManualNodeCreate
) -> Subscription:
    # 支持 raw 模式、base64 解码、yaml 或直接链接解析
    try:
        proxies, _ = parse_subscription_content(payload.node_links)
        nodes = proxies
    except Exception:
        nodes = parse_node_links(payload.node_links)
    if not nodes:
        raise HTTPException(status_code=400, detail="未找到支持的节点链接或原始内容")

    name = payload.name.strip()
    prefix = payload.node_prefix.strip() if payload.node_prefix else None
    selected_nodes = deduplicate_nodes(nodes)
    prefixed_nodes = _apply_prefix(
        selected_nodes,
        _resolve_prefix(name, prefix, payload.is_primary),
    )
    if payload.is_primary:
        await _clear_primary(db)
    item = Subscription(
        name=name,
        url="manual://nodes",
        update_interval=None,
        is_primary=payload.is_primary,
        enabled=getattr(payload, "enabled", True),
        node_prefix=prefix,
        filter_regex=[],
        include_node_names=[],
        exclude_node_names=[],
        node_renames={},
        source_nodes=[],
        manual_nodes=selected_nodes,
        raw_nodes=deduplicate_nodes(prefixed_nodes),
        last_fetched_at=datetime.now(timezone.utc),
        last_fetch_error=None,
        fetch_failed_count=0,
        fetch_comments=[],
    )
    db.add(item)
    await db.commit()
    await db.refresh(item)
    return item


async def update_subscription(
    db: AsyncSession, item: Subscription, payload: SubscriptionUpdate
) -> Subscription:
    data = payload.model_dump(exclude_unset=True)
    manual_node_links = data.pop("manual_node_links", None)
    if data.get("is_primary"):
        await _clear_primary(db)
    if "filter_regex" in data and data["filter_regex"] is not None:
        data["filter_regex"] = _normalize_and_validate_regex(data["filter_regex"])
    if "include_node_names" in data and data["include_node_names"] is not None:
        data["include_node_names"] = _normalize_node_names(data["include_node_names"])
    if "exclude_node_names" in data and data["exclude_node_names"] is not None:
        data["exclude_node_names"] = _normalize_node_names(data["exclude_node_names"])
    if "node_renames" in data and data["node_renames"] is not None:
        data["node_renames"] = _normalize_node_renames(data["node_renames"])
    if "manual_nodes" in data and data["manual_nodes"] is not None:
        data["manual_nodes"] = deduplicate_nodes(data["manual_nodes"] or [])
    if manual_node_links is not None:
        data["manual_nodes"] = _merge_manual_nodes(data.get("manual_nodes", item.manual_nodes or []), manual_node_links)

    selection_changed = bool(
        {
            "filter_regex",
            "include_node_names",
            "exclude_node_names",
            "node_renames",
            "manual_nodes",
            "node_prefix",
            "is_primary",
            "name",
        }
        & set(data.keys())
    ) or manual_node_links is not None

    for key, value in data.items():
        setattr(item, key, value)

    if selection_changed:
        _refresh_selected_nodes(item)

    db.add(item)
    await db.commit()
    await db.refresh(item)
    return item


async def delete_subscription(db: AsyncSession, item: Subscription) -> None:
    """Delete a subscription by primary key.

    Re-load by id so concurrent scheduler/fetch sessions don't leave us
    holding a detached or already-removed instance.
    """
    target = await db.get(Subscription, item.id)
    if target is None:
        return
    await db.delete(target)
    await db.commit()


async def fetch_subscription_nodes(
    db: AsyncSession, item: Subscription
) -> Subscription:
    # 手动节点没有远端 URL，点击刷新只重新计算当前保存的节点
    if item.url == "manual://nodes":
        _refresh_selected_nodes(item)
        item.last_fetched_at = datetime.now(timezone.utc)
        item.last_fetch_error = None
        item.fetch_failed_count = 0
        db.add(item)
        await db.commit()
        await db.refresh(item)
        return item

    # Capture identity early: concurrent delete may expire/detach the ORM row.
    sub_id = item.id
    sub_name = item.name

    try:
        runtime_security = await get_security_settings(db)
        fetch_proxy_enabled, fetch_proxy_url = get_fetch_proxy_config(runtime_security)
        client_kwargs = {
            "timeout": settings.request_timeout_seconds,
            "trust_env": settings.request_trust_env if not fetch_proxy_enabled else False,
            "headers": {
                "User-Agent": settings.request_user_agent,
                "Accept": "*/*",
            },
        }
        if fetch_proxy_enabled and fetch_proxy_url:
            client_kwargs["proxy"] = fetch_proxy_url

        async with httpx.AsyncClient(**client_kwargs) as client:
            response, raw_text = await _fetch_subscription_text(client, item.url)

        # Row may have been deleted while the HTTP request was in flight.
        live = await _require_subscription(db, sub_id)

        fetched_nodes, comments = parse_subscription_content(raw_text)
        live.source_nodes = deduplicate_nodes(fetched_nodes)
        live.raw_nodes = _materialize_raw_nodes(
            live.source_nodes,
            live.manual_nodes or [],
            filter_regex=live.filter_regex,
            include_node_names=live.include_node_names or [],
            exclude_node_names=live.exclude_node_names or [],
            name=live.name,
            node_prefix=live.node_prefix,
            is_primary=live.is_primary,
            node_renames=live.node_renames or {},
        )
        live.last_fetched_at = datetime.now(timezone.utc)
        live.last_fetch_error = None
        live.fetch_failed_count = 0
        live.fetch_comments = comments if live.is_primary else []
        response_headers = getattr(response, "headers", {}) or {}
        live.subscription_userinfo = response_headers.get("subscription-userinfo")
        live.profile_update_interval = response_headers.get("profile-update-interval")
        live.profile_web_page_url = response_headers.get("profile-web-page-url")
        db.add(live)
        await db.commit()
        await db.refresh(live)
        return live
    except Exception as exc:
        error_message = _format_fetch_error(exc)
        try:
            live = await db.get(Subscription, sub_id)
            if live is None:
                logger.info(
                    "Subscription %s was deleted during failed fetch; ignoring update",
                    sub_id,
                )
                raise HTTPException(status_code=404, detail="Subscription not found") from exc

            # Always stamp last_fetched_at on failure so the scheduler
            # respects update_interval instead of retrying every minute.
            live.last_fetch_error = error_message
            live.fetch_failed_count = int(live.fetch_failed_count or 0) + 1
            live.last_fetched_at = datetime.now(timezone.utc)
            db.add(live)
            await db.commit()
            await db.refresh(live)
        except HTTPException:
            raise
        except StaleDataError:
            await db.rollback()
            logger.info(
                "Subscription %s was deleted during failed fetch; ignoring update",
                sub_id,
            )
            raise HTTPException(status_code=404, detail="Subscription not found") from exc

        logger.error(
            "Failed to fetch subscription %s (%s): %s",
            sub_id,
            sub_name,
            error_message,
        )
        if isinstance(exc, HTTPException):
            raise exc
        raise


def _format_fetch_error(exc: Exception) -> str:
    if isinstance(exc, httpx.HTTPStatusError):
        response = exc.response
        return f"HTTP {response.status_code} while fetching subscription"
    message = str(exc).strip()
    if message:
        return message
    cause = getattr(exc, "__cause__", None)
    cause_message = str(cause).strip() if cause else ""
    if cause_message:
        return f"{type(exc).__name__}: {type(cause).__name__}: {cause_message}"
    return type(exc).__name__


async def _fetch_subscription_text(
    client: httpx.AsyncClient,
    url: str,
    max_redirects: int = 5,
) -> tuple[httpx.Response, str]:
    response = await stream_fetch(
        client,
        url,
        allow_private=settings.allow_private_fetch_urls,
        max_redirects=max_redirects,
    )
    try:
        response.raise_for_status()
        chunks: list[bytes] = []
        total = 0
        async for chunk in response.aiter_bytes():
            total += len(chunk)
            if total > settings.request_max_bytes:
                raise HTTPException(
                    status_code=413,
                    detail="Subscription response is too large",
                )
            chunks.append(chunk)
        content = b"".join(chunks)
        encoding = response.encoding or "utf-8"
        return response, content.decode(encoding, errors="replace")
    finally:
        await response.aclose()



def _materialize_raw_nodes(
    source_nodes: list[dict],
    manual_nodes: list[dict],
    *,
    filter_regex,
    include_node_names: list[str] | None,
    exclude_node_names: list[str] | None,
    name: str,
    node_prefix: str | None,
    is_primary: bool,
    node_renames: dict | None,
) -> list[dict]:
    """源节点到 raw_nodes 的固定管线: 合并筛选前缀改名去重

    fetch 与本地刷新共用, 顺序不要改: 正则与 include 按上游名, renames 按加前缀后的名
    """
    all_source_nodes = _combined_source_nodes(source_nodes or [], manual_nodes or [])
    selected_nodes = _apply_selection(
        all_source_nodes,
        compile_regex(filter_regex),
        include_node_names or [],
        exclude_node_names or [],
    )
    prefixed_nodes = _apply_prefix(
        selected_nodes,
        _resolve_prefix(name, node_prefix, is_primary),
    )
    renamed_nodes = _apply_renames(prefixed_nodes, node_renames or {})
    return deduplicate_nodes(renamed_nodes)


def _apply_selection(
    nodes: list[dict],
    regex_patterns: list,
    include_node_names: list[str],
    exclude_node_names: list[str],
) -> list[dict]:
    """Apply the final subscription node selection layer.

    Pipeline:
    1. Regex is the coarse filter. Empty regex means all nodes.
    2. include_node_names manually adds nodes by original upstream name.
    3. exclude_node_names removes nodes by original upstream name.
    """
    include_set = set(include_node_names or [])
    exclude_set = set(exclude_node_names or [])
    selected: dict[str, dict] = {}

    for node in nodes:
        name = str(node.get("name", "")).strip()
        if not name:
            continue
        if not regex_patterns or any(pattern.search(name) for pattern in regex_patterns):
            selected[name] = node

    if include_set:
        for node in nodes:
            name = str(node.get("name", "")).strip()
            if name in include_set:
                selected[name] = node

    for name in exclude_set:
        selected.pop(name, None)

    return list(selected.values())


def _refresh_selected_nodes(item: Subscription) -> None:
    source_nodes = item.source_nodes or []
    manual_nodes = item.manual_nodes or []

    # 有上游或手动源时走完整管线；两者皆空时把当前 raw_nodes 当已加前缀名, 只套 renames
    if item.url == "manual://nodes" or source_nodes or manual_nodes:
        item.raw_nodes = _materialize_raw_nodes(
            source_nodes,
            manual_nodes,
            filter_regex=item.filter_regex,
            include_node_names=item.include_node_names or [],
            exclude_node_names=item.exclude_node_names or [],
            name=item.name,
            node_prefix=item.node_prefix,
            is_primary=item.is_primary,
            node_renames=item.node_renames or {},
        )
        return

    if item.raw_nodes:
        item.raw_nodes = deduplicate_nodes(
            _apply_renames(item.raw_nodes or [], item.node_renames or {})
        )


def _merge_manual_nodes(existing_nodes: list[dict], node_links: str | None) -> list[dict]:
    if not node_links or not node_links.strip():
        return deduplicate_nodes(existing_nodes or [])
    try:
        parsed_nodes, _ = parse_subscription_content(node_links)
    except Exception as exc:
        raise HTTPException(status_code=400, detail="未找到支持的节点链接或原始内容") from exc
    if not parsed_nodes:
        raise HTTPException(status_code=400, detail="未找到支持的节点链接或原始内容")
    return deduplicate_nodes([*(existing_nodes or []), *parsed_nodes])


def _combined_source_nodes(source_nodes: list[dict], manual_nodes: list[dict]) -> list[dict]:
    return deduplicate_nodes([*(source_nodes or []), *(manual_nodes or [])])


def _resolve_prefix(name: str, custom_prefix: str | None, is_primary: bool) -> str:
    if custom_prefix is not None and custom_prefix.strip() != "":
        return custom_prefix.strip()
    if is_primary:
        return ""
    return name.strip()


def _apply_prefix(nodes: list[dict], prefix: str) -> list[dict]:
    if not prefix:
        return nodes

    prefixed: list[dict] = []
    for node in nodes:
        copied = dict(node)
        raw_name = str(copied.get("name", "")).strip()
        copied["name"] = f"{prefix}-{raw_name}" if raw_name else prefix
        prefixed.append(copied)
    return prefixed


def _apply_renames(nodes: list[dict], renames: dict | None) -> list[dict]:
    """Rename nodes after prefixing.

    Keys are post-prefix names. When the caller passes a new full rename map
    from an already-renamed raw_nodes list, keys refer to the current names.
    """
    mapping = _normalize_node_renames(renames or {})
    if not mapping:
        return nodes
    renamed: list[dict] = []
    for node in nodes:
        copied = dict(node)
        current = str(copied.get("name", "")).strip()
        if current in mapping:
            copied["name"] = mapping[current]
        renamed.append(copied)
    return renamed


async def _clear_primary(db: AsyncSession) -> None:
    result = await db.execute(
        select(Subscription).where(Subscription.is_primary.is_(True))
    )
    items = result.scalars().all()
    for item in items:
        item.is_primary = False
        db.add(item)
    if items:
        await db.flush()


async def fetch_due_subscriptions(db: AsyncSession) -> int:
    now = datetime.now(timezone.utc)
    # Strip timezone for comparison with SQLite naive datetimes
    now_naive = now.replace(tzinfo=None)
    # Only load enabled subscriptions with a positive interval; due-ness is
    # decided per-row with the real update_interval (no fake 1-minute prefilter).
    result = await db.execute(
        select(Subscription).where(
            Subscription.enabled.is_(True),
            Subscription.update_interval > 0,
        )
    )
    all_subs = list(result.scalars().all())
    fetched = 0

    for item in all_subs:
        # Re-load each row just before fetch so a UI delete between select and
        # fetch does not leave us mutating a deleted subscription.
        current = await db.get(Subscription, item.id)
        if current is None:
            continue
        if not current.update_interval or current.update_interval <= 0:
            continue
        if current.last_fetched_at:
            last_at = (
                current.last_fetched_at.replace(tzinfo=None)
                if current.last_fetched_at.tzinfo
                else current.last_fetched_at
            )
            delta = (now_naive - last_at).total_seconds() / 60
            if delta < current.update_interval:
                continue
        try:
            await fetch_subscription_nodes(db, current)
            fetched += 1
        except (HTTPException, SubscriptionFetchError) as exc:
            status = getattr(exc, "status_code", None)
            if status == 404:
                continue
            logger.warning(
                "Scheduled subscription fetch failed: id=%s name=%s, error=%s",
                current.id,
                current.name,
                getattr(exc, "message", getattr(exc, "detail", str(exc))),
            )
            continue
        except Exception as exc:
            logger.warning(
                "Scheduled subscription fetch failed: id=%s name=%s, unexpected=%s",
                current.id,
                current.name,
                exc,
            )
            continue

    return fetched


async def collect_all_subscription_nodes(db: AsyncSession) -> list[dict]:
    result = await db.execute(
        select(Subscription.raw_nodes).where(Subscription.enabled.is_(True))
    )
    merged: list[dict] = []
    for nodes in result.scalars().all():
        merged.extend(nodes or [])
    return deduplicate_nodes(merged)


def _normalize_and_validate_regex(regex_list: list[str]) -> list[str]:
    normalized = [entry.strip() for entry in regex_list if entry and entry.strip()]
    for index, pattern in enumerate(normalized, start=1):
        try:
            re.compile(pattern)
        except re.error as exc:
            raise HTTPException(
                status_code=400,
                detail=f"Invalid filter_regex #{index}: {exc}",
            ) from exc
    return normalized


def _normalize_node_names(names: list[str]) -> list[str]:
    normalized: list[str] = []
    seen = set()
    for item in names or []:
        name = str(item or "").strip()
        if not name or name in seen:
            continue
        seen.add(name)
        normalized.append(name)
    return normalized


def _normalize_node_renames(renames: dict | None) -> dict[str, str]:
    normalized: dict[str, str] = {}
    if not isinstance(renames, dict):
        return normalized
    for key, value in renames.items():
        source = str(key or "").strip()
        target = str(value or "").strip()
        if not source or not target or source == target:
            continue
        normalized[source] = target
    return normalized
