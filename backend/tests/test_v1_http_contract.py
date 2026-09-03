from __future__ import annotations

import json
from pathlib import Path

import pytest
import yaml

from app.models.subscription import Subscription
from tests.conftest import TestSession


CONTRACT_PATH = Path(__file__).parent / "fixtures" / "v1-http-contract.json"
FIXTURE_TOKEN = "fixture-contract-token"


async def _create_primary_subscription(client) -> None:
    created = await client.post(
        "/api/subscriptions",
        json={
            "name": "contract-primary",
            "url": "https://fixtures.invalid/subscriptions/contract",
            "is_primary": True,
            "manual_nodes": [
                {
                    "name": "contract-node",
                    "type": "ss",
                    "server": "198.51.100.24",
                    "port": 443,
                }
            ],
        },
    )
    assert created.status_code == 201, created.text

    async with TestSession() as session:
        subscription = await session.get(Subscription, created.json()["id"])
        assert subscription is not None
        subscription.subscription_userinfo = "upload=1; download=2; total=3; expire=4"
        subscription.profile_update_interval = "30"
        subscription.profile_web_page_url = "https://fixtures.invalid/profile"
        await session.commit()


async def _enable_contract_auth(client) -> None:
    configured = await client.patch(
        "/api/settings/security",
        json={
            "auth_enabled": True,
            "protect_api": True,
            "protect_exports": True,
            "token": FIXTURE_TOKEN,
        },
    )
    assert configured.status_code == 200, configured.text


@pytest.mark.asyncio
async def test_v1_management_and_export_http_contract(client):
    contract = json.loads(CONTRACT_PATH.read_text(encoding="utf-8"))
    await _create_primary_subscription(client)
    await _enable_contract_auth(client)

    management = contract["management_api"]
    missing = await client.get(management["path"])
    assert missing.status_code == management["statuses"]["missing_token"]

    ignored_query = await client.get(management["path"], params={"token": FIXTURE_TOKEN})
    assert ignored_query.status_code == management["statuses"]["query_token"]

    authorized = await client.get(
        management["path"],
        headers={"X-Clash-Token": FIXTURE_TOKEN},
    )
    assert authorized.status_code == management["statuses"]["header_token"]
    assert set(management["required_item_fields"]) <= set(authorized.json()[0])

    for export in contract["exports"]:
        missing_export = await client.get(export["path"])
        assert missing_export.status_code == export["missing_token_status"]

        response = await client.get(export["path"], params={"token": FIXTURE_TOKEN})
        assert response.status_code == export["authorized_status"]
        assert response.headers["content-type"].startswith(export["content_type"])
        assert response.headers["content-disposition"] == export["content_disposition"]
        for header, value in export["headers"].items():
            assert response.headers[header] == value

        if export["kind"] == "yaml":
            assert "proxies" in yaml.safe_load(response.text)
        else:
            assert "function main(params)" in response.text
