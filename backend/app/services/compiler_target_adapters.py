from __future__ import annotations

import json
from typing import Any
import yaml

from app.schemas.logical_config import CompilationResult, LogicalConfigurationBundle


def render_compiled_yaml(
    result: CompilationResult,
    bundle: LogicalConfigurationBundle,
    raw_nodes: list[dict[str, Any]],
) -> str:
    """根据编译后的确定性结果渲染 Clash YAML 配置文件"""
    proxy_groups = []
    for g in result.groups:
        grp_dict: dict[str, Any] = {
            "name": g.name,
            "type": g.group_type,
            "proxies": g.resolved_node_names,
        }
        raw_cfg = g.raw_config
        if g.group_type in ("url-test", "fallback", "load-balance"):
            url_cfg = raw_cfg.get("url_test_config") or {}
            grp_dict["url"] = url_cfg.get("url") or "https://cp.cloudflare.com/generate_204"
            grp_dict["interval"] = url_cfg.get("interval") or 300
            if "tolerance" in url_cfg:
                grp_dict["tolerance"] = url_cfg["tolerance"]
        proxy_groups.append(grp_dict)

    rules_list = []
    for r in result.rules:
        rtype = r.get("type", "MATCH")
        payload = r.get("payload", "")
        target = r.get("target", "DIRECT")
        if rtype == "MATCH":
            rules_list.append(f"MATCH,{target}")
        else:
            rules_list.append(f"{rtype},{payload},{target}")

    config: dict[str, Any] = {
        "port": 7890,
        "socks-port": 7891,
        "allow-lan": True,
        "mode": "rule",
        "log-level": "info",
        "proxies": raw_nodes,
        "proxy-groups": proxy_groups,
        "rules": rules_list,
    }

    if bundle.dns.enabled and bundle.dns.raw_yaml:
        try:
            dns_parsed = yaml.safe_load(bundle.dns.raw_yaml)
            if isinstance(dns_parsed, dict):
                config["dns"] = dns_parsed
        except Exception:
            pass

    return yaml.dump(config, allow_unicode=True, sort_keys=False)


def render_compiled_script(
    result: CompilationResult,
    bundle: LogicalConfigurationBundle,
) -> str:
    """根据编译后的确定性结果渲染 Clash 与 Mihomo JavaScript 脚本"""
    groups_data = [
        {
            "name": g.name,
            "type": g.group_type,
            "proxies": g.resolved_node_names,
        }
        for g in result.groups
    ]
    rules_data = [
        f"{r.get('type')},{r.get('payload')},{r.get('target')}" if r.get("type") != "MATCH" else f"MATCH,{r.get('target')}"
        for r in result.rules
    ]

    script = f"""// Clash Sub Parser 编译生成的纯函数脚本
function main(config) {{
  const customGroups = {json.dumps(groups_data, ensure_ascii=False, indent=2)};
  const customRules = {json.dumps(rules_data, ensure_ascii=False, indent=2)};

  config['proxy-groups'] = customGroups;
  config['rules'] = customRules;
  return config;
}}
"""
    return script
