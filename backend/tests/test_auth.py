from starlette.requests import Request

from app.models.security_settings import SecuritySettings
from app.services.security_settings_service import hash_token, token_matches
from app.utils.auth import extract_request_token, request_has_csrf_header, request_uses_cookie_auth, is_export_path, is_public_path, request_needs_auth, validate_token


def make_request(query: str = "", headers: dict | None = None):
    raw_headers = []
    for key, value in (headers or {}).items():
        raw_headers.append((key.lower().encode(), value.encode()))
    return Request(
        {
            "type": "http",
            "method": "GET",
            "path": "/api/subscriptions",
            "headers": raw_headers,
            "query_string": query.encode(),
            "server": ("testserver", 80),
            "scheme": "http",
            "client": ("testclient", 123),
        }
    )


def security(**kwargs):
    data = {
        "auth_enabled": True,
        "protect_frontend": True,
        "protect_api": True,
        "protect_exports": True,
        "token_hash": hash_token("secret-token"),
    }
    data.update(kwargs)
    return SecuritySettings(id=1, **data)


def test_public_assets_and_health_skip_auth() -> None:
    assert is_public_path("/health")
    assert is_public_path("/assets/index.js")
    assert is_public_path("/api/settings/auth/check")
    assert is_public_path("/api/settings/auth/login")
    assert is_public_path("/api/settings/auth/logout")
    assert not is_public_path("/api/subscriptions")


def test_export_path_detection() -> None:
    assert is_export_path("/yaml")
    assert is_export_path("/script")
    assert is_export_path("/api/generate/yaml/download")
    assert is_export_path("/api/generate/script/current")
    assert not is_export_path("/api/generate/yaml")


def test_granular_auth_flags() -> None:
    # Frontend routes must be served so the app can show a token input box.
    # protect_frontend is enforced by the SPA gate, not by blocking index.html.
    assert not request_needs_auth("/", security(protect_frontend=True))
    assert not request_needs_auth("/", security(protect_frontend=False))
    assert request_needs_auth("/api/subscriptions", security(protect_api=True))
    assert not request_needs_auth("/api/subscriptions", security(protect_api=False))
    assert request_needs_auth("/yaml", security(protect_exports=True))
    assert not request_needs_auth("/yaml", security(protect_exports=False))
    assert not request_needs_auth("/api/subscriptions", security(auth_enabled=False))


def test_token_can_be_supplied_by_header_cookie_or_bearer() -> None:
    assert extract_request_token(make_request("token=query-token")) == ""
    assert extract_request_token(make_request(headers={"X-Clash-Token": "header-token"})) == "header-token"
    assert extract_request_token(make_request(headers={"Cookie": "clash_auth_token=cookie-token"})) == "cookie-token"
    assert extract_request_token(make_request(headers={"Authorization": "Bearer bearer-token"})) == "bearer-token"


def test_query_token_takes_precedence_for_exports() -> None:
    request = make_request("token=url-token", headers={"X-Clash-Token": "header-token"})
    assert extract_request_token(request, allow_query=True) == "url-token"
    assert validate_token("url-token", "url-token")


def test_query_token_is_ignored_for_management_api() -> None:
    request = make_request("token=url-token")
    assert extract_request_token(request, allow_query=False) == ""


def test_hash_token_matching() -> None:
    token_hash = hash_token("secret-token")
    assert token_matches("secret-token", token_hash)
    assert not token_matches("wrong", token_hash)


def test_cookie_auth_requires_explicit_csrf_header_for_unsafe_api_requests() -> None:
    cookie_request = make_request(headers={"Cookie": "clash_auth_token=cookie-token"})
    assert request_uses_cookie_auth(cookie_request)
    assert not request_has_csrf_header(cookie_request)

    csrf_request = make_request(
        headers={"Cookie": "clash_auth_token=cookie-token", "X-Clash-CSRF": "1"}
    )
    assert request_uses_cookie_auth(csrf_request)
    assert request_has_csrf_header(csrf_request)

    header_request = make_request(
        headers={"Cookie": "clash_auth_token=cookie-token", "X-Clash-Token": "header-token"}
    )
    assert not request_uses_cookie_auth(header_request)
