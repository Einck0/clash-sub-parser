import pytest


@pytest.mark.asyncio
async def test_rule_batch_rejects_non_integer_ids(client):
    response = await client.post(
        "/api/rules/batch",
        json={"delete": ["not-an-id"]},
    )

    assert response.status_code == 422


@pytest.mark.asyncio
async def test_rule_category_batch_rejects_non_integer_ids(client):
    response = await client.post(
        "/api/rule-categories/batch",
        json={"delete": ["not-an-id"]},
    )

    assert response.status_code == 422
