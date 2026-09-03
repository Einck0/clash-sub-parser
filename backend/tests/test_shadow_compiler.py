from __future__ import annotations

import pytest
from httpx import AsyncClient

from tests.conftest import TestSession
from app.services.shadow_compiler_service import ShadowCompilerService


@pytest.mark.asyncio
async def test_shadow_compiler_service_comparison(client: AsyncClient):
    async with TestSession() as session:
        service = ShadowCompilerService(session)
        assert service.is_shadow_enabled() is False

        # 测试影子比对无写副作用
        diff = await service.compare_and_record_diff(target="clash")
        assert "legacy_line_count" in diff
        assert "new_line_count" in diff
        assert "semantic_fingerprint" in diff
        assert "diagnostics_count" in diff
