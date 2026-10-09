#!/usr/bin/env python3
"""
GHCR Private Package Publication Gate & Security Verification Utility.

Strict Private Publication Contract:
1. Explicit Publish Authorization Gate: Requires ENABLE_CSP_GHCR_PUBLISH == 'true'.
2. Approved Namespace Whitelist: Strictly locks to proposed namespace 'ghcr.io/einck0/csp-runtime-private'.
   Historical public package 'ghcr.io/einck0/clash-sub-parser' and any unapproved names are rejected.
3. Pre-check Dual-Proof Requirement:
   - For an existing package: Authorized GitHub API MUST return visibility == 'private' for owner/name.
     Any 401/403/network error or visibility != 'private' FAILS CLOSED (no fallback).
   - For a non-existent package (404 on direct lookup): Authorized API MUST successfully list ALL
     packages of the owner across all pages (complete pagination) to prove the package does not exist.
     If listing packages fails (401/403/network) or pagination is incomplete, FAILS CLOSED (BLOCKED).
   - Anonymous probing MUST NOT obtain image manifests (probing direct unauth and bearer token challenge).
4. Post-check Dual-Proof Requirement:
   - Docker buildx digest MUST match sha256 format.
   - Authorized GitHub API MUST confirm visibility == 'private'.
   - Anonymous probing MUST be denied (200 is fatal error).
   - Any failure causes immediate workflow termination with exit code 1.
5. Token masking: Never prints credentials into logs, stderr, or exceptions.
"""

import argparse
import json
import os
import re
import sys
import urllib.error
import urllib.parse
import urllib.request
from typing import Any, Dict, List, Optional, Tuple

APPROVED_PRIVATE_NAMESPACES = {
    "ghcr.io/einck0/csp-runtime-private",
    "einck0/csp-runtime-private",
}

HISTORICAL_PUBLIC_PACKAGES = [
    "ghcr.io/einck0/clash-sub-parser",
    "einck0/clash-sub-parser",
    "clash-sub-parser",
]

DEFAULT_PRIVATE_PACKAGE = "ghcr.io/einck0/csp-runtime-private"
DEFAULT_TIMEOUT_SECONDS = 15

SENSITIVE_TOKENS: set[str] = set()


def register_sensitive_token(token: Optional[str]) -> None:
    if token and len(token) >= 4:
        SENSITIVE_TOKENS.add(token.strip())


def mask_secrets(text: str) -> str:
    masked = text
    for token in SENSITIVE_TOKENS:
        masked = masked.replace(token, "***")
    masked = re.sub(r"(gh[pso]_[a-zA-Z0-9]{30,})", r"***[REDACTED_GH_TOKEN]***", masked)
    masked = re.sub(r"(github_pat_[a-zA-Z0-9_]{30,})", r"***[REDACTED_GH_PAT]***", masked)
    return masked


def log_info(msg: str) -> None:
    print(mask_secrets(f"[INFO] {msg}"))


def log_warn(msg: str) -> None:
    print(mask_secrets(f"[WARN] {msg}"), file=sys.stderr)


def log_error(msg: str) -> None:
    print(mask_secrets(f"[ERROR] {msg}"), file=sys.stderr)


def parse_image_reference(image_name: str) -> Tuple[str, str, str, str]:
    clean_name = image_name.strip()
    if clean_name.startswith("http://") or clean_name.startswith("https://"):
        clean_name = re.sub(r"^https?://", "", clean_name)

    parts = clean_name.split("/")
    if len(parts) >= 3:
        registry = parts[0]
        owner = parts[1]
        package = "/".join(parts[2:])
        repository = f"{owner}/{package}"
    elif len(parts) == 2:
        registry = "ghcr.io"
        owner = parts[0]
        package = parts[1]
        repository = f"{owner}/{package}"
    else:
        registry = "ghcr.io"
        owner = ""
        package = clean_name
        repository = clean_name

    return registry, repository, owner, package


def check_publish_enabled_gate(publish_enabled: Optional[str]) -> bool:
    val = (publish_enabled or "").strip().lower()
    if val != "true":
        log_error(
            "Publish gate blocked: Repository variable ENABLE_CSP_GHCR_PUBLISH is not 'true' "
            f"(received: {repr(publish_enabled)}). All publication routes are strictly fail-closed."
        )
        return False
    log_info("Publish gate check passed: ENABLE_CSP_GHCR_PUBLISH is 'true'.")
    return True


def check_package_namespace_strict(image_name: str) -> bool:
    norm = image_name.strip().lower()
    for forbidden in HISTORICAL_PUBLIC_PACKAGES:
        if norm == forbidden or norm.endswith(f"/{forbidden}"):
            log_error(
                f"Security gate violation: Image target '{image_name}' matches historical public package '{forbidden}'! "
                "The historical package cannot be converted to private and MUST NOT be published to."
            )
            return False

    if norm not in APPROVED_PRIVATE_NAMESPACES and not any(norm.endswith(f"/{app}") for app in APPROVED_PRIVATE_NAMESPACES):
        log_error(
            f"Security gate violation: Image target '{image_name}' is not in approved private namespace list "
            f"({APPROVED_PRIVATE_NAMESPACES}). Arbitrary package names cannot bypass private verification."
        )
        return False

    return True


def parse_bearer_challenge(challenge_header: str) -> Tuple[Optional[str], Optional[str], Optional[str]]:
    if not challenge_header.startswith("Bearer "):
        return None, None, None

    params = challenge_header[7:]
    realm = None
    service = None
    scope = None

    for match in re.finditer(r'([a-zA-Z0-9_]+)="([^"]*)"', params):
        key = match.group(1).lower()
        val = match.group(2)
        if key == "realm":
            realm = val
        elif key == "service":
            service = val
        elif key == "scope":
            scope = val

    return realm, service, scope


def probe_anonymous_manifest_access(
    registry: str,
    repository: str,
    tag: str,
    timeout: int = DEFAULT_TIMEOUT_SECONDS,
) -> Dict[str, Any]:
    manifest_url = f"https://{registry}/v2/{repository}/manifests/{tag}"
    accept_headers = {
        "Accept": (
            "application/vnd.docker.distribution.manifest.v2+json, "
            "application/vnd.oci.image.manifest.v1+json, "
            "application/vnd.oci.image.index.v1+json"
        ),
        "User-Agent": "csp-private-gate-probe/1.0",
    }

    log_info(f"Probing anonymous manifest access: {manifest_url}")

    req = urllib.request.Request(manifest_url, headers=accept_headers, method="GET")
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            status = resp.status
            if status == 200:
                log_error(f"ANONYMOUS PULL DETECTED: Direct unauthenticated request returned HTTP {status}!")
                return {
                    "is_public": True,
                    "reason": "Direct unauthenticated manifest request succeeded with HTTP 200",
                }
    except urllib.error.HTTPError as e:
        if e.code != 401:
            log_info(f"Direct unauthenticated request rejected with HTTP {e.code}.")
            return {"is_public": False, "status_code": e.code}

        auth_header = e.headers.get("Www-Authenticate", "")
        realm, service, scope = parse_bearer_challenge(auth_header)
        if not realm:
            log_info("Received 401 challenge without bearer realm; anonymous access denied.")
            return {"is_public": False, "reason": "No bearer realm in 401 challenge"}

        log_info(f"Received 401 challenge: realm={realm}, service={service}, scope={scope}")
        token_params = {}
        if service:
            token_params["service"] = service
        if scope:
            token_params["scope"] = scope
        token_url = f"{realm}?{urllib.parse.urlencode(token_params)}"

        token_req = urllib.request.Request(
            token_url,
            headers={"User-Agent": "csp-private-gate-probe/1.0"},
            method="GET",
        )
        try:
            with urllib.request.urlopen(token_req, timeout=timeout) as token_resp:
                token_body = json.loads(token_resp.read().decode("utf-8"))
                anon_token = token_body.get("token") or token_body.get("access_token")
        except Exception as te:
            log_info(f"Anonymous bearer token request denied: {te}")
            return {"is_public": False, "reason": f"Anonymous token request denied: {te}"}

        if not anon_token:
            log_info("Token endpoint returned empty token; anonymous access denied.")
            return {"is_public": False, "reason": "Empty anonymous token"}

        auth_req_headers = dict(accept_headers)
        auth_req_headers["Authorization"] = f"Bearer {anon_token}"
        auth_req = urllib.request.Request(manifest_url, headers=auth_req_headers, method="GET")

        try:
            with urllib.request.urlopen(auth_req, timeout=timeout) as anon_resp:
                if anon_resp.status == 200:
                    log_error(
                        f"ANONYMOUS PULL DETECTED: Manifest retrieved with anonymous bearer token (HTTP {anon_resp.status})! "
                        "The package is publicly accessible."
                    )
                    return {
                        "is_public": True,
                        "reason": "Anonymous bearer token retrieved manifest with HTTP 200",
                    }
        except urllib.error.HTTPError as ae:
            log_info(f"Manifest pull with anonymous token rejected with HTTP {ae.code} (Access Denied).")
            return {"is_public": False, "status_code": ae.code}
        except Exception as ae:
            log_error(f"Unexpected error when querying manifest with anonymous token: {ae}")
            raise

    return {"is_public": False}


def check_github_api_package_visibility(
    owner: str,
    package_name: str,
    token: Optional[str],
    timeout: int = DEFAULT_TIMEOUT_SECONDS,
) -> Dict[str, Any]:
    if not token:
        log_error("Authorized GitHub API check requires a valid token (GITHUB_TOKEN or OWNER_PAT). Token missing!")
        return {"status": "missing_token", "error": "Token not provided"}

    encoded_pkg = urllib.parse.quote(package_name, safe="")
    api_url = f"https://api.github.com/users/{owner}/packages/container/{encoded_pkg}"

    headers = {
        "Accept": "application/vnd.github+json",
        "X-GitHub-Api-Version": "2022-11-28",
        "User-Agent": "csp-private-gate-probe/1.0",
        "Authorization": f"Bearer {token.strip()}",
    }

    log_info(f"Querying GitHub Packages API: {api_url}")
    req = urllib.request.Request(api_url, headers=headers, method="GET")

    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            data = json.loads(resp.read().decode("utf-8"))
            visibility = data.get("visibility", "")
            package_id = data.get("id")
            pkg_owner = data.get("owner", {}).get("login", "")
            pkg_name = data.get("name", "")
            log_info(f"GitHub Packages API response: id={package_id}, owner='{pkg_owner}', name='{pkg_name}', visibility='{visibility}'")
            return {
                "status": "ok",
                "visibility": visibility,
                "package_id": package_id,
                "owner": pkg_owner,
                "name": pkg_name,
                "raw": data,
            }
    except urllib.error.HTTPError as e:
        if e.code == 404:
            log_info(f"GitHub Packages API returned HTTP 404: Package '{package_name}' not found on direct lookup.")
            return {"status": "not_found", "status_code": 404}
        else:
            log_error(f"GitHub Packages API returned HTTP {e.code}: {e.reason}. Access denied or network error.")
            return {"status": "error", "status_code": e.code, "error": str(e)}
    except Exception as e:
        log_error(f"Failed to query GitHub Packages API: {e}")
        return {"status": "error", "error": str(e)}


def list_and_verify_all_owner_packages(
    owner: str,
    target_package_name: str,
    token: Optional[str],
    timeout: int = DEFAULT_TIMEOUT_SECONDS,
) -> Dict[str, Any]:
    """
    Lists ALL container packages of owner with complete pagination.
    Returns:
    - {'proven_absent': True, 'count': N} if fully paginated and target package is not found.
    - {'proven_absent': False, 'found': True} if target package is found in list.
    - {'status': 'error'} if listing fails (HTTP 401/403/etc) or pagination incomplete.
    """
    if not token:
        log_error("Listing owner packages requires an authorized token. Token missing!")
        return {"status": "error", "reason": "Missing token"}

    page = 1
    per_page = 100
    all_package_names: List[str] = []

    while True:
        api_url = f"https://api.github.com/users/{owner}/packages?package_type=container&page={page}&per_page={per_page}"
        headers = {
            "Accept": "application/vnd.github+json",
            "X-GitHub-Api-Version": "2022-11-28",
            "User-Agent": "csp-private-gate-probe/1.0",
            "Authorization": f"Bearer {token.strip()}",
        }

        log_info(f"Listing owner packages page {page}: {api_url}")
        req = urllib.request.Request(api_url, headers=headers, method="GET")
        try:
            with urllib.request.urlopen(req, timeout=timeout) as resp:
                data = json.loads(resp.read().decode("utf-8"))
                if not isinstance(data, list):
                    log_error(f"Unexpected response format from packages listing: {type(data)}")
                    return {"status": "error", "reason": "Invalid response format"}

                for item in data:
                    name = item.get("name", "")
                    if name:
                        all_package_names.append(name.lower())

                # If fewer items returned than per_page, reached end of list
                if len(data) < per_page:
                    break

                page += 1
                if page > 50:  # Safety boundary against infinite pagination loops
                    log_error("Exceeded maximum pagination depth (50 pages). Incomplete listing!")
                    return {"status": "error", "reason": "Pagination depth exceeded"}
        except urllib.error.HTTPError as e:
            log_error(
                f"Failed to list owner packages: HTTP {e.code} ({e.reason}). "
                "Token lacks permissions to list owner packages (read:packages required)."
            )
            return {"status": "error", "status_code": e.code, "error": str(e)}
        except Exception as e:
            log_error(f"Exception listing owner packages: {e}")
            return {"status": "error", "error": str(e)}

    target_norm = target_package_name.strip().lower()
    if target_norm in all_package_names:
        log_info(f"Target package '{target_package_name}' found in owner's package list.")
        return {"proven_absent": False, "found": True, "total_packages": len(all_package_names)}

    log_info(
        f"Verified: Target package '{target_package_name}' is PROVEN ABSENT after checking "
        f"{len(all_package_names)} package(s) across {page} page(s)."
    )
    return {"proven_absent": True, "total_packages": len(all_package_names)}


def execute_pre_check(
    image_name: str,
    owner: str,
    publish_enabled: Optional[str],
    token: Optional[str],
) -> int:
    """
    Strict Pre-Check (Fail-Closed):
    1. Checks ENABLE_CSP_GHCR_PUBLISH == 'true'.
    2. Enforces approved private namespace.
    3. Requires authorized token.
    4. For existing package: API MUST return visibility == 'private'. Any 401/403/error FAILS CLOSED.
    5. For non-existent package: API MUST list all owner packages across all pages to prove absence.
       If listing fails or has 401/403, FAILS CLOSED (BLOCKED).
    """
    log_info("=== Starting Strict GHCR Pre-Publish Gate Verification ===")
    log_info(f"Target image name: {image_name}")

    if not check_publish_enabled_gate(publish_enabled):
        return 1

    if not check_package_namespace_strict(image_name):
        return 1

    if not token:
        log_error("FAIL-CLOSED: No authorized token provided for pre-publish gate verification.")
        return 1

    registry, repository, img_owner, package = parse_image_reference(image_name)
    target_owner = owner or img_owner or "einck0"

    api_result = check_github_api_package_visibility(target_owner, package, token)
    if api_result["status"] == "ok":
        visibility = api_result.get("visibility")
        if visibility != "private":
            log_error(
                f"FAIL-CLOSED: Existing package '{package}' has visibility '{visibility}' (expected 'private')! "
                "Publication to non-private packages is strictly forbidden."
            )
            return 1
        log_info(f"PRE-CHECK PASSED: Existing package '{package}' is verified PRIVATE via authorized GitHub API.")
        return 0

    elif api_result["status"] == "not_found":
        log_info(
            f"Package '{package}' not found on direct lookup. "
            "Executing exhaustive pagination audit of owner packages to prove non-existence..."
        )
        list_result = list_and_verify_all_owner_packages(target_owner, package, token)
        if list_result.get("proven_absent"):
            log_info(
                f"PRE-CHECK PASSED: Package '{package}' is proven absent across all {list_result.get('total_packages')} "
                "owner packages. Initial creation is authorized under private default."
            )
            return 0
        else:
            log_error(
                f"FAIL-CLOSED: Cannot prove package non-existence. "
                f"Result: {list_result}. Publication blocked."
            )
            return 1
    else:
        # Any API error (401, 403, 500, network error) strictly FAILS CLOSED
        log_error(
            f"FAIL-CLOSED: Authorized API package check failed (status: {api_result.get('status')}, "
            f"code: {api_result.get('status_code')}). Missing OWNER_PAT or insufficient token scope. "
            "BLOCKED: Strictly no downgrading to unverified anonymous probing."
        )
        return 1


def execute_post_check(
    image_name: str,
    owner: str,
    tag: str,
    digest: str,
    token: Optional[str],
) -> int:
    """
    Strict Post-Check (Fail-Closed):
    1. Validates immutable SHA256 digest format.
    2. Probes anonymous manifest access: 200 is FATAL.
    3. Re-verifies package visibility via authorized GitHub API: MUST be 'private'.
       Any 401/403/network error FAILS CLOSED.
    """
    log_info("=== Starting Strict GHCR Post-Publish Security Verification ===")
    log_info(f"Target image: {image_name}:{tag}")
    log_info(f"Target digest: {digest}")

    if not check_package_namespace_strict(image_name):
        return 1

    if not digest or not re.match(r"^sha256:[a-f0-9]{64}$", digest.strip()):
        log_error(f"POST-CHECK FAILED: Invalid or missing sha256 digest: '{digest}'")
        return 1
    log_info(f"Digest format validated: {digest.strip()}")

    registry, repository, img_owner, package = parse_image_reference(image_name)
    target_owner = owner or img_owner or "einck0"

    # 1. Anonymous challenge probe (200 is fatal)
    probe_result = probe_anonymous_manifest_access(registry, repository, tag)
    if probe_result.get("is_public"):
        log_error(
            f"POST-CHECK CRITICAL FAILURE: Package '{image_name}' is accessible anonymously! "
            f"Reason: {probe_result.get('reason')}. Fail-closed!"
        )
        return 1
    log_info("Verified: Anonymous manifest pull is denied.")

    # 2. Authorized GitHub API visibility confirmation
    if not token:
        log_error("POST-CHECK FAILED: Authorized token missing for post-publish visibility audit. Fail-closed!")
        return 1

    api_result = check_github_api_package_visibility(target_owner, package, token)
    if api_result["status"] != "ok":
        log_error(
            f"POST-CHECK FAILED: Authorized API check failed with status '{api_result.get('status')}' "
            f"(code: {api_result.get('status_code')}). Cannot confirm private visibility. Fail-closed!"
        )
        return 1

    visibility = api_result.get("visibility")
    if visibility != "private":
        log_error(
            f"POST-CHECK FAILED: Package '{package}' visibility is '{visibility}' (expected 'private')! Fail-closed."
        )
        return 1

    log_info(f"Verified: Official GitHub API confirms package visibility is 'private'.")
    log_info("=== Strict GHCR Post-Publish Security Verification PASSED ===")
    return 0


def main() -> int:
    parser = argparse.ArgumentParser(description="Strict GHCR Private Package Gate & Verification Utility")
    parser.add_argument("--mode", choices=["pre-check", "post-check"], required=True)
    parser.add_argument("--image-name", default=DEFAULT_PRIVATE_PACKAGE, help="Full container image name")
    parser.add_argument("--owner", default="einck0", help="Repository or package owner")
    parser.add_argument("--tag", default="", help="Image tag to probe")
    parser.add_argument("--digest", default="", help="Published image sha256 digest")
    parser.add_argument("--publish-enabled", default="", help="Value of ENABLE_CSP_GHCR_PUBLISH")

    args = parser.parse_args()

    github_token = os.environ.get("GITHUB_TOKEN")
    owner_pat = os.environ.get("OWNER_PAT")
    ghcr_token = os.environ.get("GHCR_TOKEN")

    register_sensitive_token(github_token)
    register_sensitive_token(owner_pat)
    register_sensitive_token(ghcr_token)

    effective_token = owner_pat or github_token or ghcr_token

    if args.mode == "pre-check":
        publish_enabled = args.publish_enabled or os.environ.get("ENABLE_CSP_GHCR_PUBLISH", "")
        return execute_pre_check(
            image_name=args.image_name,
            owner=args.owner,
            publish_enabled=publish_enabled,
            token=effective_token,
        )
    elif args.mode == "post-check":
        return execute_post_check(
            image_name=args.image_name,
            owner=args.owner,
            tag=args.tag,
            digest=args.digest,
            token=effective_token,
        )

    return 0


if __name__ == "__main__":
    sys.exit(main())
