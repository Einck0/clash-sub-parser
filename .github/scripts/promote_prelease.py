#!/usr/bin/env python3
"""Reuse or publish immutable dev prereleases using the job's normal GH_TOKEN.

Only gh is an external boundary. No release-supplied text is evaluated by a shell.
Existing releases are verified and reused, never edited, deleted or clobbered.
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

IMAGE = "ghcr.io/einck0/csp-runtime-private"
BINARY = "csp-linux-amd64"
CHECKSUM = BINARY + ".sha256"
METADATA = "release-metadata.json"
ASSETS = (BINARY, CHECKSUM, METADATA)


def gh(*args, missing_ok=False):
    result = subprocess.run(["gh", *args], capture_output=True, text=True, timeout=120)
    if result.returncode:
        if missing_ok and "(HTTP 404)" in result.stderr:
            return None
        # Do not echo arbitrary API responses, credentials or release content.
        raise RuntimeError("GitHub operation failed: " + args[0])
    return result.stdout


def identity():
    repository = os.environ["GITHUB_REPOSITORY"]
    revision = os.environ["GITHUB_SHA"]
    if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repository):
        raise ValueError("Invalid repository")
    if not re.fullmatch(r"[a-f0-9]{40}", revision):
        raise ValueError("Expected complete source SHA")
    return repository, revision, "prelease-" + revision[:7]


def digest(value):
    if not re.fullmatch(r"sha256:[a-f0-9]{64}", value):
        raise ValueError("Invalid image digest")
    return value


def binary_hash(directory):
    binary = directory / BINARY
    actual = hashlib.sha256(binary.read_bytes()).hexdigest()
    if (directory / CHECKSUM).read_text() != f"{actual}  {BINARY}\n":
        raise ValueError("Binary checksum mismatch")
    return actual


def existing_release(repository, revision, tag, directory):
    endpoint = f"repos/{repository}"
    response = gh("api", f"{endpoint}/releases/tags/{tag}", missing_ok=True)
    ref = gh("api", f"{endpoint}/git/ref/tags/{tag}", missing_ok=True)
    if response is None:
        if ref is not None:
            raise ValueError("Existing tag has no complete prerelease; refusing overwrite")
        return None
    release = json.loads(response)
    if (release.get("tag_name") != tag or release.get("name") != tag
            or release.get("target_commitish") != revision
            or release.get("prerelease") is not True or release.get("draft") is not False):
        raise ValueError("Existing prerelease identity mismatch")
    target = json.loads(ref)["object"] if ref is not None else {}
    if target.get("type") != "commit" or target.get("sha") != revision:
        raise ValueError("Existing tag does not target exact source SHA")
    if sorted(asset["name"] for asset in release["assets"]) != sorted(ASSETS):
        raise ValueError("Existing prerelease assets are incomplete or unexpected")
    gh("release", "download", tag, "--repo", repository, "--dir", str(directory))
    metadata = json.loads((directory / METADATA).read_text())
    image_digest = digest(metadata["image_digest"])
    if (metadata.get("schema") != 1 or metadata.get("source_sha") != revision
            or metadata.get("repository") != repository or metadata.get("tag") != tag
            or metadata.get("artifact") != BINARY
            or metadata.get("binary_sha256") != binary_hash(directory)
            or metadata.get("image") != f"{IMAGE}@{image_digest}"):
        raise ValueError("Existing release metadata or binary mismatch")
    for key in ("run_id", "run_attempt"):
        if not re.fullmatch(r"[1-9][0-9]*", str(metadata.get(key, ""))):
            raise ValueError("Invalid release run identity")
    if metadata.get("run_url") != f"https://github.com/{repository}/actions/runs/{metadata['run_id']}":
        raise ValueError("Invalid release run URL")
    return metadata


def output(**values):
    with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as stream:
        for key, value in values.items():
            stream.write(f"{key}={value}\n")


def reuse(directory):
    repository, revision, tag = identity()
    with tempfile.TemporaryDirectory(prefix="csp-prelease-") as temporary:
        verified = Path(temporary)
        metadata = existing_release(repository, revision, tag, verified)
        if metadata is None:
            output(reused="false")
            return
        directory.mkdir(parents=True, exist_ok=True)
        for asset in ASSETS:
            shutil.copyfile(verified / asset, directory / asset)
        output(reused="true", image_digest=metadata["image_digest"])


def publish(directory):
    repository, revision, tag = identity()
    image_digest = digest(os.environ["IMAGE_DIGEST"])
    checksum = binary_hash(directory)
    with tempfile.TemporaryDirectory(prefix="csp-prelease-") as temporary:
        verified = Path(temporary)
        previous = existing_release(repository, revision, tag, verified)
        if previous is not None:
            if previous["image_digest"] != image_digest or previous["binary_sha256"] != checksum:
                raise ValueError("Immutable prerelease differs; refusing overwrite")
            # Preserve the original run identity and byte-for-byte immutable assets.
            for asset in (BINARY, CHECKSUM):
                if (verified / asset).read_bytes() != (directory / asset).read_bytes():
                    raise ValueError("Immutable release asset differs")
            print("Verified existing immutable prerelease: " + tag)
            return
    run_id = os.environ["GITHUB_RUN_ID"]
    run_attempt = os.environ["GITHUB_RUN_ATTEMPT"]
    if not all(re.fullmatch(r"[1-9][0-9]*", value) for value in (run_id, run_attempt)):
        raise ValueError("Invalid run identity")
    metadata = {
        "schema": 1, "repository": repository, "source_sha": revision, "tag": tag,
        "artifact": BINARY, "binary_sha256": checksum,
        "image": f"{IMAGE}@{image_digest}", "image_digest": image_digest,
        "run_id": run_id, "run_attempt": run_attempt,
        "run_url": f"https://github.com/{repository}/actions/runs/{run_id}",
    }
    (directory / METADATA).write_text(json.dumps(metadata, indent=2, sort_keys=True) + "\n")
    gh("release", "create", tag, "--repo", repository, "--target", revision,
       "--title", tag, "--prerelease", "--latest=false",
       "--notes", "Tested dev artifact and verified runtime image; not a production deployment.",
       *(str(directory / asset) for asset in ASSETS))
    with tempfile.TemporaryDirectory(prefix="csp-prelease-") as temporary:
        verified = existing_release(repository, revision, tag, Path(temporary))
        if verified != metadata:
            raise ValueError("Published release identity verification failed")
    print("Published and verified immutable prerelease: " + tag)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=("reuse", "publish"))
    parser.add_argument("--directory", type=Path, default=Path("bin"))
    args = parser.parse_args()
    try:
        (reuse if args.mode == "reuse" else publish)(args.directory)
    except (KeyError, ValueError, RuntimeError, OSError, subprocess.TimeoutExpired) as error:
        # Error messages from our validations are bounded and contain no API data.
        raise SystemExit(f"Prerelease {args.mode} failed ({type(error).__name__}); no assets overwritten") from None
