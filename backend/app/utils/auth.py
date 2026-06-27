import secrets
from dataclasses import dataclass

from fastapi import Request

AUTH_QUERY_PARAM = "token"
AUTH_COOKIE = "clash_auth_token"
AUTH_HEADER = "x-clash-token"
CSRF_HEADER = "x-clash-csrf"
UNSAFE_METHODS = {"POST", "PUT", "PATCH", "DELETE"}
PUBLIC_PATHS = {
    "/health",
    "/favicon.ico",
    "/robots.txt",
    "/api/settings/auth/check",
    "/api/settings/auth/login",
    "/api/settings/auth/logout",
}
PUBLIC_PREFIXES = ("/assets/",)
EXPORT_PATHS = {"/yaml", "/script"}
EXPORT_PREFIXES = ("/api/generate/yaml/", "/api/generate/script/")
EXPORT_SUFFIXES = ("/download", "/current")


@dataclass(frozen=True)
class AuthResult:
    ok: bool
    status_code: int = 401
    detail: str = "Unauthorized"


def is_public_path(path: str) -> bool:
    if path in PUBLIC_PATHS:
        return True
    return any(path.startswith(prefix) for prefix in PUBLIC_PREFIXES)


def is_export_path(path: str) -> bool:
    if path in EXPORT_PATHS:
        return True
    return any(path.startswith(prefix) and path.endswith(EXPORT_SUFFIXES) for prefix in EXPORT_PREFIXES)


def is_api_path(path: str) -> bool:
    return path.startswith("/api/")


def is_unsafe_method(method: str) -> bool:
    return method.upper() in UNSAFE_METHODS


def request_uses_cookie_auth(request: Request) -> bool:
    if not request.cookies.get(AUTH_COOKIE):
        return False
    if request.headers.get(AUTH_HEADER):
        return False
    authorization = request.headers.get("authorization", "")
    scheme, _, value = authorization.partition(" ")
    return not (scheme.lower() == "bearer" and value)


def request_has_csrf_header(request: Request) -> bool:
    return request.headers.get(CSRF_HEADER) == "1"


def extract_request_token(request: Request, allow_query: bool = False) -> str:
    query_token = request.query_params.get(AUTH_QUERY_PARAM)
    if allow_query and query_token:
        return query_token

    header_token = request.headers.get(AUTH_HEADER)
    if header_token:
        return header_token

    cookie_token = request.cookies.get(AUTH_COOKIE)
    if cookie_token:
        return cookie_token

    authorization = request.headers.get("authorization", "")
    scheme, _, value = authorization.partition(" ")
    if scheme.lower() == "bearer" and value:
        return value.strip()
    return ""


def validate_token(raw_token: str, expected_token: str) -> bool:
    if not raw_token or not expected_token:
        return False
    return secrets.compare_digest(str(raw_token), str(expected_token))


def is_frontend_path(path: str) -> bool:
    """Non-API, non-export, non-public paths (i.e. frontend SPA routes)."""
    if is_public_path(path) or is_api_path(path) or is_export_path(path):
        return False
    return path == "/" or not path.startswith("/") or any(
        path.startswith(p) for p in ("/node-groups", "/rules", "/dns", "/generate", "/settings")
    )


def request_needs_auth(path: str, security) -> bool:
    if not security.auth_enabled:
        return False
    if is_export_path(path):
        return security.protect_exports
    if is_api_path(path):
        return security.protect_api
    if is_frontend_path(path):
        return security.protect_frontend
    return False
