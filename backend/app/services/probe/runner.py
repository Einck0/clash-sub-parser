"""sing-box 隔离进程执行器与端口池管理

为每个待测节点分配受控的回环端口并拉起轻量 sing-box mixed 入站，
探测结束后保证优雅终止子进程并回收端口与临时配置
"""
from __future__ import annotations

import asyncio
import json
import logging
import os
import shutil
import socket
import tempfile
from contextlib import asynccontextmanager
from typing import Any, AsyncGenerator

from app.services.probe.converter import clash_to_singbox_outbound, generate_singbox_config

logger = logging.getLogger(__name__)


def find_singbox_binary() -> str | None:
    """寻找可用的 sing-box 二进制路径"""
    custom_path = os.environ.get("SINGBOX_PATH")
    if custom_path and os.path.exists(custom_path) and os.access(custom_path, os.X_OK):
        return custom_path
    bin_path = shutil.which("sing-box")
    if bin_path and os.path.exists(bin_path) and os.access(bin_path, os.X_OK):
        return bin_path
    return None


class AsyncPortPool:
    """本地回环端口池，管理并发探测端口的借还与占用检测"""

    def __init__(self, start_port: int = 21000, end_port: int = 21100) -> None:
        self.start_port = start_port
        self.end_port = end_port
        self._available_ports: set[int] = set(range(start_port, end_port))
        self._in_use_ports: set[int] = set()
        self._lock = asyncio.Lock()

    @staticmethod
    def is_port_available(port: int) -> bool:
        """检测指定本地端口是否未被任何服务监听"""
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
            s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
            try:
                s.bind(("127.0.0.1", port))
                return True
            except OSError:
                return False

    @asynccontextmanager
    async def acquire(self) -> AsyncGenerator[int, None]:
        """从端口池租用一个当前可用的本地端口"""
        port: int | None = None
        async with self._lock:
            candidates = sorted(list(self._available_ports))
            for p in candidates:
                if self.is_port_available(p):
                    self._available_ports.remove(p)
                    self._in_use_ports.add(p)
                    port = p
                    break

        if port is None:
            raise RuntimeError("No available loopback ports in probe pool")

        try:
            yield port
        finally:
            async with self._lock:
                self._in_use_ports.discard(port)
                self._available_ports.add(port)


# 全局共享端口池
port_pool = AsyncPortPool(start_port=21000, end_port=21100)


async def _wait_for_port_ready(port: int, max_wait_s: float = 1.5, step_s: float = 0.05) -> bool:
    """轮询等待 sing-box mixed 入站就绪"""
    deadline = asyncio.get_event_loop().time() + max_wait_s
    while asyncio.get_event_loop().time() < deadline:
        try:
            reader, writer = await asyncio.wait_for(
                asyncio.open_connection("127.0.0.1", port),
                timeout=step_s,
            )
            writer.close()
            try:
                await writer.wait_closed()
            except Exception:
                pass
            return True
        except Exception:
            await asyncio.sleep(step_s)
    return False


@asynccontextmanager
async def spawn_node_runner(
    node: dict[str, Any],
    runner_bin: str | None = None,
) -> AsyncGenerator[dict[str, Any], None]:
    """为单个节点拉起 sing-box runner 上下文

    yields:
      {
        "proxy_url": "http://127.0.0.1:21000",
        "port": 21000,
        "direct_supported": False,
      }
    若节点为原生 HTTP 与 SOCKS 且 sing-box 不可用，支持直接直连
    """
    node_type = str(node.get("type") or "").strip().lower()
    server = str(node.get("server") or "").strip()
    port = int(node.get("port") or 0)

    singbox_bin = runner_bin or find_singbox_binary()

    # HTTP 和 SOCKS5 节点兜底直连模式
    if not singbox_bin and node_type in ("http", "socks5", "socks"):
        auth_str = ""
        user = node.get("username")
        pwd = node.get("password")
        if user:
            auth_str = f"{user}:{pwd or ''}@"
        scheme = "http" if node_type == "http" else "socks5"
        yield {
            "proxy_url": f"{scheme}://{auth_str}{server}:{port}",
            "port": port,
            "is_runner": False,
        }
        return

    if not singbox_bin:
        raise RuntimeError("sing-box binary not found on system path")

    outbound = clash_to_singbox_outbound(node)
    if not outbound:
        raise ValueError(f"Unsupported node configuration or invalid parameters: {node.get('name')}")

    async with port_pool.acquire() as listen_port:
        config_dict = generate_singbox_config(outbound, listen_port=listen_port)

        # 写入临时配置文件
        temp_dir = tempfile.mkdtemp(prefix="singbox_probe_")
        config_path = os.path.join(temp_dir, "config.json")
        proc: asyncio.subprocess.Process | None = None
        try:
            with open(config_path, "w", encoding="utf-8") as f:
                json.dump(config_dict, f)

            # 启动 sing-box 子进程
            proc = await asyncio.create_subprocess_exec(
                singbox_bin,
                "run",
                "-c",
                config_path,
                stdout=asyncio.subprocess.DEVNULL,
                stderr=asyncio.subprocess.PIPE,
            )

            # 等待本地入站就绪
            ready = await _wait_for_port_ready(listen_port, max_wait_s=2.0)
            if not ready:
                # 检查子进程是否已退出并记录报错
                if proc.returncode is not None:
                    _, stderr = await proc.communicate()
                    err_msg = stderr.decode("utf-8", errors="ignore").strip()
                    raise RuntimeError(f"sing-box startup failed (code {proc.returncode}): {err_msg}")
                raise TimeoutError(f"sing-box failed to listen on 127.0.0.1:{listen_port} in time")

            yield {
                "proxy_url": f"http://127.0.0.1:{listen_port}",
                "port": listen_port,
                "is_runner": True,
            }

        finally:
            # 优雅销毁子进程
            if proc is not None and proc.returncode is None:
                try:
                    proc.terminate()
                    try:
                        await asyncio.wait_for(proc.wait(), timeout=0.5)
                    except asyncio.TimeoutError:
                        proc.kill()
                        await proc.wait()
                except Exception as exc:
                    logger.debug("Failed to terminate sing-box proc: %s", exc)

            # 清理临时文件
            try:
                shutil.rmtree(temp_dir, ignore_errors=True)
            except Exception:
                pass
