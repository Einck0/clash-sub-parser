#!/usr/bin/env python3
"""Probe how User-Agent affects 7li node count."""
from __future__ import annotations

import sqlite3
import sys

import httpx

sys.path.insert(0, "/app")
from app.utils.clash_parser import parse_subscription_content


def main() -> None:
    con = sqlite3.connect("/data/clash_sub_parser.db")
    url = con.execute("select url from subscriptions where id=1").fetchone()[0]
    proxy = con.execute(
        "select fetch_proxy_url from security_settings where fetch_proxy_enabled=1"
    ).fetchone()
    proxy = proxy[0] if proxy else None
    uas = [
        "ClashforWindows/0.20",
        "clash.meta",
        "ClashMeta/1.18",
        "clash-verge/v2",
        "Mozilla/5.0",
    ]
    for ua in uas:
        with httpx.Client(proxy=proxy, timeout=60.0, follow_redirects=True) as client:
            resp = client.get(url, headers={"User-Agent": ua})
        nodes, _ = parse_subscription_content(resp.text)
        types: dict[str, int] = {}
        for node in nodes:
            t = str(node.get("type") or "?")
            types[t] = types.get(t, 0) + 1
        print(ua, "status", resp.status_code, "nodes", len(nodes), "types", types)


if __name__ == "__main__":
    main()
