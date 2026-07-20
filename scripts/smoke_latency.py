#!/usr/bin/env python3
import json
import os
import sqlite3
from urllib.parse import quote

import httpx


def main() -> None:
    con = sqlite3.connect("/data/clash_sub_parser.db")
    rn = json.loads(
        con.execute("select raw_nodes from subscriptions where id=1").fetchone()[0]
        or "[]"
    )
    name = rn[1]["name"] if len(rn) > 1 else (rn[0]["name"] if rn else None)
    print("sample", name)
    base = os.environ.get("CLASH_MIHOMO_API_URL", "")
    secret = os.environ.get("CLASH_MIHOMO_API_SECRET", "")
    print("api", base, "secret_set", bool(secret))
    if name and base:
        headers = {"Authorization": f"Bearer {secret}"} if secret else {}
        path = quote(name, safe="")
        url = f"{base.rstrip('/')}/proxies/{path}/delay"
        with httpx.Client(headers=headers, timeout=15.0, trust_env=False) as client:
            resp = client.get(
                url,
                params={
                    "url": "https://www.google.com/generate_204",
                    "timeout": "8000",
                },
            )
            print("mihomo delay", resp.status_code, resp.text[:200])
    with httpx.Client(timeout=20.0, trust_env=False) as client:
        resp = client.post(
            "http://127.0.0.1:18080/api/latency/check",
            json={"names": [name] if name else [], "timeout_ms": 8000},
        )
        print("app latency", resp.status_code, resp.text[:300])
        resp2 = client.post(
            "http://127.0.0.1:18080/api/geoip/lookup",
            json={"hosts": ["1.1.1.1"]},
        )
        print("app geoip", resp2.status_code, resp2.text[:200])


if __name__ == "__main__":
    main()
