import pytest


@pytest.mark.asyncio
async def test_readiness_checks_database(client):
    response = await client.get("/ready")

    assert response.status_code == 200
    assert response.json() == {"status": "ready"}
