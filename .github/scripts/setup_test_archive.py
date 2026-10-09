#!/usr/bin/env python3
"""
Sets up a minimal, hermetic synthetic legacy cold archive SQLite database
for CI unit tests (internal/application/inventory and cmd/csp).
All records are entirely synthetic test fixtures containing zero production data.
Zero external dependencies (standard library sqlite3).
"""

import os
import sqlite3
import subprocess
import sys

DEFAULT_ARCHIVE_PATH = "/home/service/backups/csp-legacy-cold-archive-20260919.db"


def ensure_parent_dir(path: str) -> None:
    parent_dir = os.path.dirname(os.path.abspath(path))
    if os.path.isdir(parent_dir):
        return

    try:
        os.makedirs(parent_dir, exist_ok=True)
    except PermissionError:
        # On GitHub Actions runners, sudo is available passwordless
        subprocess.run(["sudo", "mkdir", "-p", parent_dir], check=True)
        uid = os.getuid()
        gid = os.getgid()
        subprocess.run(["sudo", "chown", "-R", f"{uid}:{gid}", parent_dir], check=True)


def create_synthetic_archive(target_path: str) -> None:
    ensure_parent_dir(target_path)

    # Remove existing synthetic file if present to guarantee clean state
    if os.path.exists(target_path):
        os.remove(target_path)

    conn = sqlite3.connect(target_path)
    cur = conn.cursor()

    cur.execute("""
    CREATE TABLE IF NOT EXISTS subscriptions (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        url TEXT NOT NULL,
        enabled INTEGER NOT NULL
    );
    """)

    cur.execute("""
    CREATE TABLE IF NOT EXISTS sources (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        url TEXT NOT NULL,
        enabled INTEGER NOT NULL
    );
    """)

    # Purely synthetic records matching test assertions in token_repair_test.go & reset_test.go
    fixtures = [
        (
            "synth-sub-7li-001",
            "7li",
            "https://z.7li7li.com/api/v1/client/subscribe?token=synthetic_test_token_7li",
            1,
        ),
        (
            "synth-sub-mojie-002",
            "魔戒",
            "https://msub.xn--m7r52rosihxm.com/api/v1/client/subscribe?token=synthetic_test_token_mojie",
            1,
        ),
    ]

    cur.executemany(
        "INSERT INTO subscriptions (id, name, url, enabled) VALUES (?, ?, ?, ?);",
        fixtures,
    )
    cur.executemany(
        "INSERT INTO sources (id, name, url, enabled) VALUES (?, ?, ?, ?);",
        fixtures,
    )

    conn.commit()
    conn.close()

    os.chmod(target_path, 0o644)

    # Validate readability
    verify_conn = sqlite3.connect(f"file:{target_path}?mode=ro", uri=True)
    vcur = verify_conn.cursor()
    vcur.execute("SELECT name, url, enabled FROM subscriptions WHERE enabled = 1;")
    rows = vcur.fetchall()
    verify_conn.close()

    assert len(rows) == 2, f"Expected 2 rows in synthetic archive, got {len(rows)}"
    print(f"Synthetic cold archive fixture created successfully at: {target_path}")
    for name, url, enabled in rows:
        print(f"  - [{name}] enabled={enabled} url={url}")


def main() -> None:
    target_path = sys.argv[1] if len(sys.argv) > 1 else DEFAULT_ARCHIVE_PATH
    create_synthetic_archive(target_path)


if __name__ == "__main__":
    main()
