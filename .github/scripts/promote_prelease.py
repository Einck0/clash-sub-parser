#!/usr/bin/env python3
"""Publish rolling prelease channel release and assets using the job's normal GH_TOKEN.

Only gh is an external boundary. No release-supplied text is evaluated by a shell.
Maintains the single prelease release and moving channel tag, verified fail-closed.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile

IMAGE = "ghcr.io/einck0/clash-sub-parser"
BINARY = "csp-linux-amd64"
CHECKSUM = BINARY + ".sha256"
METADATA = "release-metadata.json"
ASSETS = (BINARY, CHECKSUM, METADATA)
CHANNEL_TAG = "prelease"


def gh(*args, missing_ok=False):
    result = subprocess.run(["gh", *args], capture_output=True, text=True, timeout=120)
    if result.returncode:
        if missing_ok and any(pattern in result.stderr for pattern in ("(HTTP 404)", "Not Found")):
            return None
        # Do not echo arbitrary API responses, credentials or release content.
        raise RuntimeError("GitHub operation failed: " + args[0])
    return result.stdout


def enforce_guard():
    ref = os.environ.get("GITHUB_REF", "")
    event = os.environ.get("GITHUB_EVENT_NAME", "")
    if ref != "refs/heads/prelease":
        raise ValueError(f"Refusal: promote_prelease must only run on refs/heads/prelease, got '{ref}'")
    if event == "pull_request":
        raise ValueError("Refusal: promote_prelease must never run on pull_request")
    if event not in ("push", "workflow_dispatch"):
        raise ValueError(f"Refusal: promote_prelease unexpected event '{event}'")


def identity():
    enforce_guard()
    repository = os.environ["GITHUB_REPOSITORY"]
    revision = os.environ["GITHUB_SHA"]
    if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repository):
        raise ValueError("Invalid repository")
    if not re.fullmatch(r"[a-f0-9]{40}", revision):
        raise ValueError("Expected complete source SHA")
    return repository, revision, CHANNEL_TAG


def digest(value):
    if not re.fullmatch(r"sha256:[a-f0-9]{64}", value):
        raise ValueError("Invalid image digest")
    return value


def binary_hash(directory):
    binary = directory / BINARY
    if not binary.is_file():
        raise ValueError("Binary artifact missing")
    actual = hashlib.sha256(binary.read_bytes()).hexdigest()
    checksum_file = directory / CHECKSUM
    if not checksum_file.is_file():
        raise ValueError("Checksum manifest missing")
    content = checksum_file.read_text()
    if content not in (f"{actual}  {BINARY}\n", f"{actual} *{BINARY}\n"):
        raise ValueError("Binary checksum mismatch")
    return actual


def ensure_channel_tag(repository, revision, tag):
    """Point the fixed channel tag to target commit SHA using Git ref API.

    Only channel tags (refs/tags/...) are permitted; moving branches (refs/heads/...) is strictly forbidden.
    """
    if tag != CHANNEL_TAG:
        raise ValueError("Refusal: tag must be prelease channel tag")
    ref_path = f"repos/{repository}/git/refs/tags/{tag}"
    existing_ref = gh("api", f"repos/{repository}/git/ref/tags/{tag}", missing_ok=True)
    if existing_ref is None:
        gh("api", f"repos/{repository}/git/refs", "-f", f"ref=refs/tags/{tag}", "-f", f"sha={revision}")
    else:
        # Update existing tag ref with force=true
        gh("api", "-X", "PATCH", ref_path, "-f", f"sha={revision}", "-F", "force=true")


def existing_release(repository, revision, tag, directory):
    endpoint = f"repos/{repository}"
    response = gh("api", f"{endpoint}/releases/tags/{tag}", missing_ok=True)
    ref = gh("api", f"{endpoint}/git/ref/tags/{tag}", missing_ok=True)
    if response is None:
        return None
    release = json.loads(response)
    if (release.get("tag_name") != tag or release.get("name") != tag
            or release.get("target_commitish") != revision
            or release.get("prerelease") is not True or release.get("draft") is not False):
        return None
    if ref is None:
        return None
    target = json.loads(ref).get("object", {})
    if target.get("type") != "commit" or target.get("sha") != revision:
        return None
    if sorted(asset["name"] for asset in release.get("assets", [])) != sorted(ASSETS):
        return None
    gh("release", "download", tag, "--repo", repository, "--dir", str(directory))
    metadata_path = directory / METADATA
    if not metadata_path.is_file():
        return None
    try:
        metadata = json.loads(metadata_path.read_text())
    except Exception:
        return None
    image_digest = digest(metadata.get("image_digest", ""))
    if (metadata.get("schema") != 1 or metadata.get("source_sha") != revision
            or metadata.get("repository") != repository or metadata.get("tag") != tag
            or metadata.get("artifact") != BINARY
            or metadata.get("binary_sha256") != binary_hash(directory)
            or metadata.get("image") != f"{IMAGE}@{image_digest}"):
        return None
    for key in ("run_id", "run_attempt"):
        if not re.fullmatch(r"[1-9][0-9]*", str(metadata.get(key, ""))):
            return None
    if metadata.get("run_url") != f"https://github.com/{repository}/actions/runs/{metadata['run_id']}":
        return None
    return metadata


def publish(directory):
    repository, revision, tag = identity()
    image_digest = digest(os.environ["IMAGE_DIGEST"])
    checksum = binary_hash(directory)
    with tempfile.TemporaryDirectory(prefix="csp-prelease-check-") as temporary:
        previous = existing_release(repository, revision, tag, Path(temporary))
        if previous is not None:
            if previous["image_digest"] == image_digest and previous["binary_sha256"] == checksum:
                print("Verified existing immutable prerelease: " + tag)
                return

    ensure_channel_tag(repository, revision, tag)

    run_id = os.environ["GITHUB_RUN_ID"]
    run_attempt = os.environ["GITHUB_RUN_ATTEMPT"]
    if not all(re.fullmatch(r"[1-9][0-9]*", value) for value in (run_id, run_attempt)):
        raise ValueError("Invalid run identity")
    metadata = {
        "schema": 1,
        "repository": repository,
        "source_sha": revision,
        "tag": tag,
        "artifact": BINARY,
        "binary_sha256": checksum,
        "image": f"{IMAGE}@{image_digest}",
        "image_digest": image_digest,
        "run_id": run_id,
        "run_attempt": run_attempt,
        "run_url": f"https://github.com/{repository}/actions/runs/{run_id}",
    }
    (directory / METADATA).write_text(json.dumps(metadata, indent=2, sort_keys=True) + "\n")

    rel_exists = gh("api", f"repos/{repository}/releases/tags/{tag}", missing_ok=True)
    if rel_exists is None:
        gh("release", "create", tag, "--repo", repository, "--target", revision,
           "--title", tag, "--prerelease", "--latest=false",
           "--notes", "Tested prelease channel artifact and verified runtime image; not a production deployment.",
           *(str(directory / asset) for asset in ASSETS))
    else:
        gh("release", "edit", tag, "--repo", repository, "--target", revision,
           "--title", tag, "--prerelease", "--latest=false",
           "--notes", "Tested prelease channel artifact and verified runtime image; not a production deployment.")
        gh("release", "upload", tag, "--repo", repository, "--clobber",
           *(str(directory / asset) for asset in ASSETS))

    with tempfile.TemporaryDirectory(prefix="csp-prelease-verify-") as temporary:
        verified = existing_release(repository, revision, tag, Path(temporary))
        if verified != metadata:
            raise ValueError("Published release identity verification failed")
    print("Published and verified prelease channel release: " + tag)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=("publish",), default="publish", nargs="?")
    parser.add_argument("--directory", type=Path, default=Path("bin"))
    args = parser.parse_args()
    try:
        publish(args.directory)
    except (KeyError, ValueError, RuntimeError, OSError, subprocess.TimeoutExpired) as error:
        # Error messages from our validations are bounded and contain no API data.
        raise SystemExit(f"Prerelease publish failed ({type(error).__name__}); no assets overwritten") from None
