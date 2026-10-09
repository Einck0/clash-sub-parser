import hashlib
import json
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

CSP_ROOT = Path("/home/service/clash-sub-parser")
ARCHIVE_DIR = CSP_ROOT / "docs" / "archive" / "openspec"
MANIFEST_PATH = CSP_ROOT / "docs" / "archive" / "manifest.json"
OPENSPEC_DIR = CSP_ROOT / "openspec"
SPEC_LEDGER_PATH = CSP_ROOT / "docs" / "specification" / "migration-ledger.json"
MIG_LEDGER_PATH = CSP_ROOT / "docs" / "migration" / "migration-ledger.json"
SHARED_AUDIT_TOOL = Path("/home/service/pi/bin/speckit-coverage-audit")
SCHEMA_PATH = Path("/home/service/pi/docs/migration/schema/migration-ledger.schema.json")
CHECK_PREREQ_SCRIPT = CSP_ROOT / ".specify" / "scripts" / "bash" / "check-prerequisites.sh"


def sha256_file(p: Path) -> str:
    h = hashlib.sha256()
    with open(p, "rb") as f:
        while chunk := f.read(65536):
            h.update(chunk)
    return h.hexdigest()


def test_csp_openspec_archive_census_and_byte_parity():
    assert MANIFEST_PATH.is_file(), "Archive manifest must exist"
    with open(MANIFEST_PATH, "r", encoding="utf-8") as f:
        manifest = json.load(f)

    assert manifest["project"] == "clash-sub-parser"
    assert manifest["total_files_count"] == 189
    assert manifest["total_bytes"] == 987997
    assert manifest["census_parity_verified"] is True
    assert manifest.get("state") == "retired"
    assert manifest.get("status") == "completed"
    assert len(manifest["before_census"]) == 189
    assert len(manifest["after_census"]) == 189

    # Original openspec active path must be retired and absent from project root
    assert not OPENSPEC_DIR.exists(), "Original openspec active path must be retired and absent from project root"

    for item in manifest["after_census"]:
        target_path = CSP_ROOT / item["archive_pointer"]
        assert target_path.is_file(), f"Archived file missing: {target_path}"
        actual_hash = sha256_file(target_path)
        assert actual_hash == item["sha256"], f"Hash mismatch for {target_path}"
        assert target_path.stat().st_size == item["size_bytes"], f"Size mismatch for {target_path}"


def test_csp_archive_pointer_resolution():
    with open(MANIFEST_PATH, "r", encoding="utf-8") as f:
        manifest = json.load(f)

    pointer_map = {m["source_logical_id"]: m for m in manifest["pointer_mappings"]}
    assert len(pointer_map) == 189

    # Verify both ledger mirrors exist and are identical
    assert SPEC_LEDGER_PATH.is_file(), "docs/specification/migration-ledger.json must exist"
    assert MIG_LEDGER_PATH.is_file(), "docs/migration/migration-ledger.json must exist"
    assert SPEC_LEDGER_PATH.read_text(encoding="utf-8") == MIG_LEDGER_PATH.read_text(encoding="utf-8"), "Ledger mirrors must be identical"

    with open(SPEC_LEDGER_PATH, "r", encoding="utf-8") as f:
        ledger = json.load(f)

    assert ledger["summary"]["total_source_requirements"] == 300
    assert len(ledger["entries"]) == 1109

    for entry in ledger["entries"]:
        src = entry["source"]
        src_rel = src["relpath"]
        expected_src_hash = src["hash"]

        # If source is in openspec/, pointer mapping must resolve it directly
        if src_rel.startswith("openspec/"):
            assert src_rel in pointer_map, f"Missing pointer mapping for {src_rel}"
            mapping = pointer_map[src_rel]
            archived_rel = mapping["archive_pointer"]
            assert src.get("archive_pointer") == archived_rel, f"Entry archive_pointer mismatch for {entry['id']}"

            archived_file = CSP_ROOT / archived_rel
            assert archived_file.is_file(), f"Target archived file missing: {archived_file}"
            resolved_hash = sha256_file(archived_file)
            assert resolved_hash == expected_src_hash, f"Resolved hash mismatch for {src_rel}"
            assert mapping["physical_sha256"] == expected_src_hash


def test_csp_audit_tool_pointer_support_inspection():
    assert SHARED_AUDIT_TOOL.is_file(), "Shared audit tool must exist"
    assert SCHEMA_PATH.is_file(), "Migration ledger schema must exist"

    # 1. Verify immutable manifest content and structure
    assert MANIFEST_PATH.is_file(), "Archive manifest must exist"
    manifest = json.loads(MANIFEST_PATH.read_text(encoding="utf-8"))
    assert manifest["source_directory"] == "openspec"
    assert manifest["destination_directory"] == "docs/archive/openspec"
    assert manifest["census_parity_verified"] is True
    assert manifest["total_files_count"] == 189
    assert manifest["total_bytes"] == 987997
    assert manifest.get("state") == "retired"
    assert manifest.get("status") == "completed"

    # 2. Final actual root: original openspec is absent, audit passes with 100% integrity
    assert not OPENSPEC_DIR.exists(), "Original openspec active path must be retired on actual project root"
    cmd = [
        sys.executable,
        str(SHARED_AUDIT_TOOL),
        "--ledger", str(MIG_LEDGER_PATH),
        "--project-root", str(CSP_ROOT),
        "--schema", str(SCHEMA_PATH),
    ]
    proc = subprocess.run(cmd, capture_output=True, text=True)
    assert proc.returncode == 0, f"Audit failed on CSP root with original absent:\n{proc.stdout}\n{proc.stderr}"
    assert "Status: PASS" in proc.stdout, f"Status not PASS in audit output: {proc.stdout}"
    assert '"hash_integrity_pct": 100.0' in proc.stdout
    assert '"discovered_source_specs": 70' in proc.stdout
    assert '"discovered_source_tasks_files": 27' in proc.stdout

    # 3. Isolated tempfixture where original openspec is absent: test actual audit behavior and mock invalid pointer fails
    with tempfile.TemporaryDirectory(prefix="csp_audit_fixture_") as tmpdir:
        tmp_root = Path(tmpdir)
        shutil.copytree(CSP_ROOT / "docs" / "archive", tmp_root / "docs" / "archive")
        shutil.copytree(CSP_ROOT / "docs" / "migration", tmp_root / "docs" / "migration")
        shutil.copytree(CSP_ROOT / "specs", tmp_root / "specs")

        # Copy supplemental docs outside openspec that are referenced by ledger
        for sub in ["docs/decisions", "docs/REDESIGN_HANDOFF.md", "docs/template_redesign.md", ".exp.md", "docs/CONTEXT_RESTART.md"]:
            cand = CSP_ROOT / sub
            if cand.is_file():
                target = tmp_root / sub
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(cand, target)
            elif cand.is_dir():
                shutil.copytree(cand, tmp_root / sub)

        # Crucial assertion: openspec/ must NOT exist in this isolated fixture
        assert not (tmp_root / "openspec").exists()

        fix_cmd = [
            sys.executable,
            str(SHARED_AUDIT_TOOL),
            "--ledger", str(tmp_root / "docs" / "migration" / "migration-ledger.json"),
            "--project-root", str(tmp_root),
            "--schema", str(SCHEMA_PATH),
        ]
        # Valid isolated run passes
        proc_valid = subprocess.run(fix_cmd, capture_output=True, text=True)
        assert proc_valid.returncode == 0, f"Audit failed on valid isolated fixture:\n{proc_valid.stdout}"
        assert "Status: PASS" in proc_valid.stdout

        # Negative check 1: Mock invalid pointer pointing to non-existent file -> MUST FAIL
        tmp_manifest_path = tmp_root / "docs" / "archive" / "manifest.json"
        tmp_manifest = json.loads(tmp_manifest_path.read_text(encoding="utf-8"))
        orig_pointer = tmp_manifest["pointer_mappings"][0]["archive_pointer"]
        tmp_manifest["pointer_mappings"][0]["archive_pointer"] = "docs/archive/openspec/non_existent_spec.md"
        tmp_manifest_path.write_text(json.dumps(tmp_manifest, indent=2), encoding="utf-8")

        proc_missing = subprocess.run(fix_cmd, capture_output=True, text=True)
        assert proc_missing.returncode != 0, "Audit must fail when archive pointer target is missing"
        assert "MISSING_ARCHIVE_PAYLOAD" in proc_missing.stdout or "INVALID_ARCHIVE_MANIFEST" in proc_missing.stdout

        # Negative check 2: Mock invalid pointer with corrupted payload hash -> MUST FAIL
        tmp_manifest["pointer_mappings"][0]["archive_pointer"] = orig_pointer
        tmp_manifest_path.write_text(json.dumps(tmp_manifest, indent=2), encoding="utf-8")
        corrupt_target = tmp_root / orig_pointer
        corrupt_target.write_bytes(b"CORRUPTED_PREFIX_" + corrupt_target.read_bytes())

        proc_corrupt = subprocess.run(fix_cmd, capture_output=True, text=True)
        assert proc_corrupt.returncode != 0, "Audit must fail when archived payload hash is corrupted"
        assert "ARCHIVE_MANIFEST_HASH_MISMATCH" in proc_corrupt.stdout or "HASH_MISMATCH" in proc_corrupt.stdout


def test_csp_native_prerequisites_feature_detection():
    assert CHECK_PREREQ_SCRIPT.is_file(), "Official check-prerequisites.sh script must exist"
    env = dict(subprocess.os.environ)
    env["SPECIFY_FEATURE_DIRECTORY"] = "specs/001-inventory"

    res = subprocess.run(
        [str(CHECK_PREREQ_SCRIPT), "--json", "--require-tasks", "--include-tasks"],
        cwd=str(CSP_ROOT),
        capture_output=True,
        text=True,
        env=env,
        check=False,
    )
    assert res.returncode == 0, f"check-prerequisites failed: {res.stderr}"
    data = json.loads(res.stdout)
    assert "specs/001-inventory" in data["FEATURE_DIR"]
    assert "tasks.md" in data["AVAILABLE_DOCS"]
