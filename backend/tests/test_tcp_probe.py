import pytest

from app.services import tcp_probe_service as svc


@pytest.mark.asyncio
async def test_probe_nodes_ok_fail_skip(monkeypatch):
    async def fake_probe(server, port, timeout_s):
        if server == "good.example":
            return {"status": "ok", "connect_ms": 12, "error": None}
        if server == "slow.example":
            return {"status": "timeout", "connect_ms": 2000, "error": "timeout"}
        return {"status": "fail", "connect_ms": 5, "error": "refused"}

    monkeypatch.setattr(svc, "_probe_one", fake_probe)

    out = await svc.probe_nodes(
        [
            {"name": "a", "server": "good.example", "port": 443, "type": "ss"},
            {"name": "b", "server": "bad.example", "port": 1, "type": "ss"},
            {"name": "c", "server": "slow.example", "port": 80, "type": "vmess"},
            {"name": "broken", "type": "ss"},  # missing server/port
        ],
        timeout_ms=500,
        concurrency=5,
    )
    assert out["summary"]["total"] == 4
    assert out["summary"]["ok"] == 1
    assert out["summary"]["fail"] == 1
    assert out["summary"]["timeout"] == 1
    assert out["summary"]["skip"] == 1
    by_name = {r["name"]: r for r in out["results"]}
    assert by_name["a"]["status"] == "ok"
    assert by_name["b"]["status"] == "fail"
    assert by_name["c"]["status"] == "timeout"
    assert by_name["broken"]["status"] == "skip"


@pytest.mark.asyncio
async def test_probe_tcp_endpoint(client, monkeypatch):
    async def fake_probe_nodes(nodes, timeout_ms=2000, concurrency=20):
        return {
            "results": [
                {
                    "name": "n1",
                    "server": "1.1.1.1",
                    "port": 443,
                    "type": "ss",
                    "status": "ok",
                    "connect_ms": 8,
                    "error": None,
                }
            ],
            "summary": {"total": 1, "ok": 1, "fail": 0, "timeout": 0, "skip": 0},
            "timeout_ms": timeout_ms,
        }

    monkeypatch.setattr("app.routers.probe.probe_nodes", fake_probe_nodes)
    response = await client.post(
        "/api/probe/tcp",
        json={
            "nodes": [{"name": "n1", "server": "1.1.1.1", "port": 443, "type": "ss"}],
            "timeout_ms": 1000,
        },
    )
    assert response.status_code == 200
    data = response.json()
    assert data["summary"]["ok"] == 1
    assert data["results"][0]["status"] == "ok"


@pytest.mark.asyncio
async def test_probe_tcp_requires_nodes(client):
    response = await client.post("/api/probe/tcp", json={"nodes": []})
    assert response.status_code == 400
