from datetime import datetime, timezone
import logging
import re
from urllib.parse import urljoin

import httpx
from fastapi import HTTPException
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.config import get_settings
from app.models.subscription import Subscription
from app.schemas.subscription import ManualNodeCreate, SubscriptionCreate, SubscriptionUpdate
from app.services.security_settings_service import get_fetch_proxy_config, get_security_settings
from app.utils.clash_parser import compile_regex, parse_node_links, parse_subscription_content
from app.utils.dedup import deduplicate_nodes
from app.utils.validators import validate_fetch_url

settings = get_settings()
logger = logging.getLogger(__name__)


async def list_subscriptions(db: AsyncSession) -> list[Subscription]:
    result = await db.execute(select(Subscription).order_by(Subscription.id.asc()))
    return list(result.scalars().all())


async def get_subscription(
    db: AsyncSession, subscription_id: int
) -> Subscription | None:
    return await db.get(Subscription, subscription_id)


async def create_subscription(
    db: AsyncSession, payload: SubscriptionCreate
) -> Subscription:
    if payload.is_primary:
        await _clear_primary(db)

    data = payload.model_dump()
    manual_node_links = data.pop("manual_node_links", None)
    data["filter_regex"] = _normalize_and_validate_regex(data.get("filter_regex", []))
    data["include_node_names"] = _normalize_node_names(data.get("include_node_names", []))
    data["exclude_node_names"] = _normalize_node_names(data.get("exclude_node_names", []))
    data["manual_nodes"] = _merge_manual_nodes(data.get("manual_nodes") or [], manual_node_links)
    item = Subscription(**data)
    _refresh_selected_nodes(item)
    db.add(item)
    await db.commit()
    await db.refresh(item)
    return item


async def create_manual_node_subscription(
    db: AsyncSession, payload: ManualNodeCreate
) -> Subscription:
    if payload.is_primary:
        await _clear_primary(db)

    nodes = parse_node_links(payload.node_links)
    if not nodes:
        raise HTTPException(status_code=400, detail="No supported node links found")

    name = payload.name.strip()
    prefix = payload.node_prefix.strip() if payload.node_prefix else None
    selected_nodes = deduplicate_nodes(nodes)
    prefixed_nodes = _apply_prefix(
        selected_nodes,
        _resolve_prefix(name, prefix, payload.is_primary),
    )
    item = Subscription(
        name=name,
        url="manual://nodes",
        update_interval=None,
        is_primary=payload.is_primary,
        node_prefix=prefix,
        filter_regex=[],
        include_node_names=[],
        exclude_node_names=[],
        source_nodes=[],
        manual_nodes=selected_nodes,
        raw_nodes=deduplicate_nodes(prefixed_nodes),
        last_fetched_at=datetime.now(timezone.utc),
        last_fetch_error=None,
        fetch_failed_count=0,
        fetch_comments=[] if payload.is_primary else [],
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
    if "manual_nodes" in data and data["manual_nodes"] is not None:
        data["manual_nodes"] = deduplicate_nodes(data["manual_nodes"] or [])
    if manual_node_links is not None:
        data["manual_nodes"] = _merge_manual_nodes(data.get("manual_nodes", item.manual_nodes or []), manual_node_links)

    selection_changed = bool(
        {"filter_regex", "include_node_names", "exclude_node_names", "manual_nodes", "node_prefix", "is_primary", "name"}
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
    await db.delete(item)
    await db.commit()


async def fetch_subscription_nodes(
    db: AsyncSession, item: Subscription
) -> Subscription:
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

        fetched_nodes, comments = parse_subscription_content(raw_text)
        item.source_nodes = deduplicate_nodes(fetched_nodes)
        all_source_nodes = _combined_source_nodes(item.source_nodes, item.manual_nodes or [])
        regex_patterns = compile_regex(item.filter_regex)
        selected_nodes = _apply_selection(
            all_source_nodes,
            regex_patterns,
            item.include_node_names or [],
            item.exclude_node_names or [],
        )

        prefixed_nodes = _apply_prefix(
            selected_nodes,
            _resolve_prefix(item.name, item.node_prefix, item.is_primary),
        )

        item.raw_nodes = deduplicate_nodes(prefixed_nodes)
        item.last_fetched_at = datetime.now(timezone.utc)
        item.last_fetch_error = None
        item.fetch_failed_count = 0
        item.fetch_comments = comments if item.is_primary else []
        response_headers = getattr(response, "headers", {}) or {}
        item.subscription_userinfo = response_headers.get("subscription-userinfo")
        item.profile_update_interval = response_headers.get("profile-update-interval")
        item.profile_web_page_url = response_headers.get("profile-web-page-url")
    except Exception as exc:
        error_message = _format_fetch_error(exc)
        item.last_fetch_error = error_message
        item.fetch_failed_count = int(item.fetch_failed_count or 0) + 1
        db.add(item)
        await db.commit()
        await db.refresh(item)
        logger.exception("Failed to fetch subscription %s (%s): %s", item.id, item.name, error_message)
        raise

    db.add(item)
    await db.commit()
    await db.refresh(item)
    return item


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
    current_url = validate_fetch_url(
        url,
        allow_private_hosts=settings.allow_private_fetch_urls,
    )
    for _ in range(max_redirects + 1):
        async with client.stream("GET", current_url) as response:
            if response.is_redirect:
                location = response.headers.get("location")
                if not location:
                    raise HTTPException(
                        status_code=502,
                        detail="Subscription redirect is missing Location header",
                    )
                current_url = validate_fetch_url(
                    urljoin(str(response.url), location),
                    allow_private_hosts=settings.allow_private_fetch_urls,
                )
                continue

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

    raise HTTPException(status_code=502, detail="Too many subscription redirects")


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
    source_nodes = _combined_source_nodes(item.source_nodes or [], item.manual_nodes or [])
    fallback_to_current = False
    if not source_nodes and item.raw_nodes:
        source_nodes = item.raw_nodes
        fallback_to_current = True
    if not source_nodes:
        return

    selected_nodes = _apply_selection(
        source_nodes,
        compile_regex(item.filter_regex),
        item.include_node_names or [],
        item.exclude_node_names or [],
    )
    if fallback_to_current:
        item.raw_nodes = deduplicate_nodes(selected_nodes)
        return

    prefixed_nodes = _apply_prefix(
        selected_nodes,
        _resolve_prefix(item.name, item.node_prefix, item.is_primary),
    )
    item.raw_nodes = deduplicate_nodes(prefixed_nodes)


def _merge_manual_nodes(existing_nodes: list[dict], node_links: str | None) -> list[dict]:
    parsed_nodes = parse_node_links(node_links or "") if node_links else []
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
    cutoff = now_naive - __import__('datetime').timedelta(minutes=1)
    # Only fetch subscriptions that need updating (filter at SQL level)
    result = await db.execute(
        select(Subscription).where(
            Subscription.update_interval > 0,
            (Subscription.last_fetched_at.is_(None)) | (Subscription.last_fetched_at < cutoff),
        )
    )
    all_subs = list(result.scalars().all())
    fetched = 0

    for item in all_subs:
        if not item.update_interval or item.update_interval <= 0:
            continue
        if item.last_fetched_at:
            last_at = item.last_fetched_at.replace(tzinfo=None) if item.last_fetched_at.tzinfo else item.last_fetched_at
            delta = (now_naive - last_at).total_seconds() / 60
            if delta < item.update_interval:
                continue
        try:
            await fetch_subscription_nodes(db, item)
            fetched += 1
        except Exception:
            logger.warning(
                "Scheduled subscription fetch failed: id=%s name=%s", item.id, item.name
            )
            continue

    return fetched


async def collect_all_subscription_nodes(db: AsyncSession) -> list[dict]:
    result = await db.execute(select(Subscription.raw_nodes))
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
