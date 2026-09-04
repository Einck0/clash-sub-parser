from __future__ import annotations

import importlib.util
from pathlib import Path
import sys


REPOSITORY_ROOT = Path(__file__).parents[2]
SCRIPT_PATH = REPOSITORY_ROOT / "scripts" / "run_full_migration_audit.py"


def load_audit_module():
    spec = importlib.util.spec_from_file_location("migration_audit", SCRIPT_PATH)
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def test_full_migration_audit_preserves_safeguarded_snapshot(tmp_path) -> None:
    audit = load_audit_module()
    source_hash = audit.sha256_file(audit.DEFAULT_SOURCE)

    manifest = audit.run_audit(
        audit.DEFAULT_SOURCE,
        tmp_path / "rehearsal.db",
        tmp_path / "manifest.json",
    )

    assert audit.sha256_file(audit.DEFAULT_SOURCE) == source_hash
    assert manifest["source_hash_unchanged"] is True
    assert manifest["target"]["tables"] == manifest["source"]["tables"]
    assert manifest["target"]["tables"]["nodes"] >= 2080
    assert manifest["relationship_orphans"] == {
        "node_source_links_missing_nodes": 0,
        "node_source_links_missing_revisions": 0,
        "source_revisions_missing_sources": 0,
    }
    assert manifest["identity_id_mappings"]["nodes"] == 2080
    assert set(manifest["compiler_output_bytes"]) == set(audit.SUPPORTED_TARGETS)
    assert all(size > 0 for size in manifest["compiler_output_bytes"].values())
