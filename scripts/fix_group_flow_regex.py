#!/usr/bin/env python3
"""Remove flow keyword from node-group regex exclude lists."""
from __future__ import annotations

import json
import sqlite3
import sys

DB = sys.argv[1] if len(sys.argv) > 1 else "/data/clash_sub_parser.db"
FLOW = "流量"


def strip_flow(text: str) -> str:
    t = str(text or "")
    t = t.replace("|" + FLOW, "").replace(FLOW + "|", "").replace(FLOW, "")
    while "||" in t:
        t = t.replace("||", "|")
    return t.replace("(|", "(").replace("|)", ")")


def main() -> None:
    con = sqlite3.connect(DB)
    con.row_factory = sqlite3.Row
    cur = con.cursor()
    changed = 0
    for row in cur.execute(
        "select id, name, include_entries, regex_rules from node_groups"
    ):
        entries = json.loads(row["include_entries"] or "[]")
        rr = json.loads(row["regex_rules"] or "[]")
        blob = json.dumps(entries, ensure_ascii=False) + json.dumps(rr, ensure_ascii=False)
        if FLOW not in blob:
            continue
        new_entries = []
        for entry in entries:
            if isinstance(entry, dict) and entry.get("type") == "regex":
                entry = {**entry, "value": strip_flow(entry.get("value"))}
            new_entries.append(entry)
        new_rr = [strip_flow(x) for x in rr]
        cur.execute(
            "update node_groups set include_entries=?, regex_rules=? where id=?",
            (
                json.dumps(new_entries, ensure_ascii=False),
                json.dumps(new_rr, ensure_ascii=False),
                row["id"],
            ),
        )
        changed += 1
        print("fixed", row["id"], row["name"], new_rr)
    con.commit()
    print("changed", changed)
    for row in cur.execute(
        "select id,name,regex_rules from node_groups where id in (10,24)"
    ):
        print(dict(row))
    con.close()


if __name__ == "__main__":
    main()
