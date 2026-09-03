from __future__ import annotations

import hashlib

from app.schemas.logical_config import CompilationResult

COMPILER_VERSION = "v1.0.0"


class CompilerCache:
    """编译器缓存管理，按配置版本与库存世代做失效"""

    def __init__(self) -> None:
        self._cache: dict[str, CompilationResult] = {}
        self._config_index: dict[str, set[str]] = {}
        self._inventory_index: dict[str, set[str]] = {}

    @staticmethod
    def compute_cache_key(
        config_revision: str,
        inventory_generation: str,
        policy_revision: str = "default",
        target: str = "preview",
    ) -> str:
        raw = f"{COMPILER_VERSION}|{config_revision}|{inventory_generation}|{policy_revision}|{target}"
        return hashlib.sha256(raw.encode("utf-8")).hexdigest()

    def get(
        self,
        config_revision: str,
        inventory_generation: str,
        policy_revision: str = "default",
        target: str = "preview",
    ) -> CompilationResult | None:
        key = self.compute_cache_key(config_revision, inventory_generation, policy_revision, target)
        return self._cache.get(key)

    def set(
        self,
        config_revision: str,
        inventory_generation: str,
        result: CompilationResult,
        policy_revision: str = "default",
        target: str = "preview",
    ) -> None:
        key = self.compute_cache_key(config_revision, inventory_generation, policy_revision, target)
        self._cache[key] = result
        self._config_index.setdefault(config_revision, set()).add(key)
        self._inventory_index.setdefault(inventory_generation, set()).add(key)

    def invalidate_by_config(self, config_revision: str) -> int:
        keys = self._config_index.pop(config_revision, set())
        for k in keys:
            self._cache.pop(k, None)
            for inv_keys in self._inventory_index.values():
                inv_keys.discard(k)
        return len(keys)

    def invalidate_by_inventory(self, inventory_generation: str) -> int:
        keys = self._inventory_index.pop(inventory_generation, set())
        for k in keys:
            self._cache.pop(k, None)
            for cfg_keys in self._config_index.values():
                cfg_keys.discard(k)
        return len(keys)

    def clear(self) -> None:
        self._cache.clear()
        self._config_index.clear()
        self._inventory_index.clear()


compiler_cache = CompilerCache()
