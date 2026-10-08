#!/usr/bin/env python3
"""No-speed, stage-first experiment. Default commands do not make probe requests."""
import argparse
import concurrent.futures
import hashlib
import json
import os
import pathlib
import re
import subprocess
import sys
import threading
import time
from datetime import datetime, timezone
from ledger import Bridge, Ledger

DEFAULT_CAP = 128 << 20
MAX_CAP = 1 << 30
DEFAULT_DEADLINE = 1800
MAX_DEADLINE = 14400
DEFAULT_CHANGE = 'csp-fresh-inventory-full-validation'
ORDER = ['csp', 'same-engine', 'same-engine', 'csp']
PLATFORMS = {'streaming': ['netflix', 'youtube', 'disney'], 'ai': ['openai', 'claude', 'gemini'], 'ip_risk': ['iprisk']}
ALL_PLATFORM_MATRIX = {
    'streaming': {
        'netflix': 'supported (method mismatch: regional subrequests)',
        'youtube': 'supported',
        'disney': 'supported',
        'spotify': 'not_supported (native only)',
        'tiktok': 'not_supported (native only)'
    },
    'ai': {
        'openai': 'supported',
        'claude': 'supported',
        'gemini': 'supported'
    },
    'ip_risk': {
        'iprisk': 'supported (method mismatch: country lookup vs identity lookup)'
    },
    'speed': {
        'speed': 'disabled (no-speed contract)'
    }
}
PIN = '3c320fd58aff5235e16218c050ec5b8ce587e233'
ROOT = pathlib.Path(__file__).resolve().parents[2]

def is_pid_alive(pid):
    if not isinstance(pid, int) or pid <= 0:
        return False
    try:
        os.kill(pid, 0)
        return True
    except (OSError, ProcessLookupError):
        return False

def canonical(x): return json.dumps(x, sort_keys=True, separators=(',', ':')).encode()
def digest(x): return hashlib.sha256(x).hexdigest()
def read(path): return json.loads(pathlib.Path(path).read_bytes())

def private_write(path, x):
    p = pathlib.Path(path)
    p.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    tmp = p.parent / f'.tmp_{p.name}_{os.getpid()}_{int(time.time()*1000)}'
    fd = os.open(tmp, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    try:
        with os.fdopen(fd, 'w') as f:
            json.dump(x, f, indent=2)
            f.flush()
            os.fsync(f.fileno())
        os.replace(tmp, p)
    except Exception:
        if tmp.exists():
            tmp.unlink(missing_ok=True)
        raise

def check_disk_safety(path):
    p = pathlib.Path(path).resolve()
    target_dir = p if p.is_dir() else p.parent
    target_dir.mkdir(parents=True, exist_ok=True, mode=0o700)
    stat = os.statvfs(str(target_dir))
    avail = stat.f_bavail * stat.f_frsize
    if avail < (1536 << 20):
        raise RuntimeError(f'safe stop: disk available {avail // (1024*1024)}MiB < 1536MiB threshold')
    return avail

def inventory(path):
    p = pathlib.Path(path)
    if p.stat().st_mode & 0o077: raise ValueError('inventory must be private 0600')
    x = read(p)['nodes']
    if len(x) != 90 or len({n['logical_id'] for n in x}) != 90: raise ValueError('ALL90 unique inventory required')
    return x

def allocation_plan(total=DEFAULT_CAP, native=0):
    if not 0 < total <= MAX_CAP or native not in [0, 2]: raise ValueError('invalid approved cap/native sides')
    baseline = 90 * (4 + native) * 65536
    if total <= baseline: raise ValueError('insufficient baseline plus platform allocation')
    return {'baseline_per_node': 65536, 'baseline_total': baseline, 'platform_per_side': (total - baseline) // 4, 'unused_rounding': (total - baseline) % 4}

def binary(path, engine):
    p = pathlib.Path(path).resolve()
    info = subprocess.run(['go', 'version', '-m', str(p)], check=True, capture_output=True, text=True).stdout
    if not re.search(r'github.com/metacubex/mihomo\s+v' + re.escape(engine) + r'\s', info): raise ValueError('binary engine mismatch')
    return {'path': str(p), 'sha256': digest(p.read_bytes()), 'engine': 'mihomo/v' + engine, 'build_info': info}

def create_plan(a):
    nodes = inventory(a.inventory)
    if a.deadline_seconds <= 0 or a.response_body_budget_bytes <= 0:
        raise ValueError('deadline and budget must be positive')
    if (a.baseline_body_limit_bytes, a.alive_concurrency, a.media_concurrency, a.speed_concurrency) != (65536, 8, 2, 0):
        raise ValueError('frozen 64KiB/8/2/0 contract required')
    platform_names = a.platforms.split(',')
    if not platform_names or len(set(platform_names)) != len(platform_names) or set(platform_names) - set(sum(PLATFORMS.values(), [])):
        raise ValueError('unsupported platform selection')
    change_name = getattr(a, 'change', None) or DEFAULT_CHANGE
    approval_source = getattr(a, 'approval_source', '') or ''
    p = {
        'version': 1,
        'change': change_name,
        'approval_source': approval_source,
        'inventory': str(pathlib.Path(a.inventory).resolve()),
        'inventory_sha256': digest(pathlib.Path(a.inventory).read_bytes()),
        'nodes': [{'id': n['logical_id'], 'revision': n.get('connection_revision'), 'config_sha256': digest(canonical(n))} for n in nodes],
        'cap': a.response_body_budget_bytes,
        'seconds': a.deadline_seconds,
        'allocation': allocation_plan(a.response_body_budget_bytes, a.native_baseline_sides),
        'order': ORDER,
        'native_network_sides': list(range(a.native_baseline_sides)),
        'platforms': platform_names,
        'platform_support_matrix': ALL_PLATFORM_MATRIX,
        'timeout_ms': 15000,
        'concurrency': {'alive': 8, 'media': 2, 'speed': 0},
        'speed_enabled': False,
        'binaries': {'csp': binary(a.csp, '1.19.32'), 'same-engine': binary(a.reference, '1.19.32')},
        'source_commit': PIN,
        'conditions': {'baseline_target': 'http://cp.cloudflare.com/generate_204', 'target_tls': 'verify', 'proxy_tls': 'frozen node configuration', 'http2': False, 'redirect': 'reject', 'keepalive': False, 'baseline_ua': 'Mozilla/5.0 (compatible; CSP-Probe/1.0)'},
        'quota_algorithm': 'equal-side/equal-group/equal-eligible-node-floor-v1',
        'cleanup_seconds': 10,
        'limitations': [
            'Native defaults are separate; native media not attempted',
            'Platform algorithms may differ: method mismatch not parity',
            'Body excludes headers/TLS/NIC; no throughput test',
            'Online wire bytes unknown; only application response body metered'
        ]
    }
    if a.native_baseline_sides:
        p['binaries']['native'] = binary(a.native, '1.19.31')
    reference_root = pathlib.Path(a.reference_source).resolve()
    if (reference_root / 'SOURCE.sha256').read_text().split()[-1] != PIN:
        raise ValueError('reference source provenance mismatch')
    p['reference_source'] = str(reference_root)
    p['source_files'] = {}
    p['target_literals'] = {}
    files = [ROOT / 'internal/application/probe/runner.go', ROOT / 'internal/probe/mihomo/client.go', ROOT / 'tools/probe-comparison/csp/main.go', ROOT / 'tools/probe-comparison/run.py', ROOT / 'tools/probe-comparison/ledger.py', reference_root / 'check/check.go', reference_root / 'check/reference_entry.go', reference_root / 'go.mod', reference_root / 'go.sum', reference_root / 'samecore.mod', reference_root / 'samecore.sum']
    for name in platform_names:
        files.extend([ROOT / 'internal/probe/platform' / (name + '.go'), reference_root / 'check/platform' / (name + '.go')])
    for f in files:
        p['source_files'][str(f)] = digest(f.read_bytes())
        literals = sorted(set(re.findall(r'https?://[^\s"`<>]+', f.read_text())))
        if literals:
            p['target_literals'][str(f)] = literals
    p['method_mismatches'] = {'netflix': 'native additional region subrequest/algorithm', 'iprisk': 'native country lookup vs CSP identity lookup'}
    p['samecore_diff'] = {}
    for name in ['mod', 'sum']:
        result = subprocess.run(['diff', '-u', str(reference_root / ('go.' + name)), str(reference_root / ('samecore.' + name))], capture_output=True, text=True)
        if result.returncode not in [0, 1]:
            raise ValueError('dependency diff unavailable')
        p['samecore_diff'][name] = result.stdout
    private_write(a.output, p)
    return p

def validate_plan(p):
    if p.get('version') != 1 or p.get('speed_enabled') is not False or p.get('order') != ORDER or p.get('timeout_ms') != 15000 or p.get('concurrency') != {'alive': 8, 'media': 2, 'speed': 0}:
        raise ValueError('invalid frozen no-speed contract')
    if p.get('seconds', 0) <= 0 or p.get('cap', 0) <= 0:
        raise ValueError('invalid budget or deadline')
    if p['allocation'] != allocation_plan(p['cap'], len(p['native_network_sides'])):
        raise ValueError('allocation mismatch')
    nodes = inventory(p['inventory'])
    if digest(pathlib.Path(p['inventory']).read_bytes()) != p['inventory_sha256']:
        raise ValueError('inventory changed')
    expected = [{'id': n['logical_id'], 'revision': n.get('connection_revision'), 'config_sha256': digest(canonical(n))} for n in nodes]
    if expected != p['nodes']:
        raise ValueError('node configuration mismatch')
    if not p['platforms'] or set(p['platforms']) - set(sum(PLATFORMS.values(), [])):
        raise ValueError('invalid platforms')
    for side, b in p['binaries'].items():
        if digest(pathlib.Path(b['path']).read_bytes()) != b['sha256']:
            raise ValueError('binary changed')
    for f, h in p['source_files'].items():
        if digest(pathlib.Path(f).read_bytes()) != h:
            raise ValueError('source changed')
    return nodes

def freeze(a):
    p = read(a.plan)
    nodes = validate_plan(p)
    raw = subprocess.run([p['binaries']['csp']['path']], input=canonical({'mode': 'freeze', 'nodes': nodes}), capture_output=True, check=True).stdout
    mappings = json.loads(raw)['mappings']
    if [m['name'] for m in mappings] != [n['logical_id'] for n in nodes]:
        raise ValueError('mapping alignment mismatch')
    p['mappings_sha256'] = digest(canonical(mappings))
    p['ledger'] = str(pathlib.Path(a.ledger).resolve())
    p['manifest_sha256'] = digest(canonical(p))
    private_write(a.manifest, p)
    if pathlib.Path(a.ledger).exists():
        raise ValueError('ledger must be new')
    ledger = Ledger(a.ledger)
    ledger.create(p['manifest_sha256'], p['cap'])
    pathlib.Path(a.ledger).chmod(0o600)
    return p

def verify_manifest(p, ledger_path):
    check = dict(p)
    h = check.pop('manifest_sha256')
    if digest(canonical(check)) != h:
        raise ValueError('manifest tampered')
    if str(pathlib.Path(ledger_path).resolve()) != p['ledger']:
        raise ValueError('ledger path mismatch')
    return validate_plan(p)

def intersection(nodes, baseline):
    ids = [n['logical_id'] for n in nodes]
    return [i for i in ids if all(baseline[r].get(i, {}).get('category') == 'available' and baseline[r][i].get('executed') for r in range(4))]

def normalize(payload, side):
    rows = payload.get('rows', [])
    rows = list(rows.values()) if isinstance(rows, dict) else rows
    out = {}
    for row in rows:
        node = row.get('node') or row.get('node_logical_id')
        a = row.get('attempt') or row
        out[node] = {
            'executed': bool(a.get('executed')),
            'queued': True,
            'category': a.get('category', 'unknown'),
            'reason': a.get('reason', ''),
            'status': a.get('status', 0),
            'node_latency': 'unknown',
            'raw': row
        }
    return out

def invoke(binary_path, input_, deadline, stdout, stderr, heartbeat_fn=None):
    check_disk_safety(pathlib.Path(stdout).parent)
    with open(stderr, 'xb') as err:
        os.chmod(stderr, 0o600)
        proc = subprocess.Popen([binary_path], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=err)
        input_bytes = canonical(input_)
        
        def write_stdin():
            try:
                proc.stdin.write(input_bytes)
                proc.stdin.close()
            except (BrokenPipeError, OSError):
                pass
        
        in_t = threading.Thread(target=write_stdin, daemon=True)
        in_t.start()
        
        out_chunks = []
        def read_stdout():
            while True:
                chunk = proc.stdout.read(65536)
                if not chunk:
                    break
                out_chunks.append(chunk)
        
        out_t = threading.Thread(target=read_stdout, daemon=True)
        out_t.start()
        
        last_hb = time.time()
        while proc.poll() is None:
            now = time.time()
            if now >= deadline:
                proc.kill()
                proc.wait()
                in_t.join(timeout=1)
                out_t.join(timeout=1)
                raise ValueError('owned child deadline exceeded; conservative reservations retained')
            if now - last_hb >= 8.0:
                if heartbeat_fn:
                    hb_line = heartbeat_fn(now)
                    sys.stderr.write(hb_line + '\n')
                    sys.stderr.flush()
                last_hb = now
            time.sleep(0.5)
        
        in_t.join(timeout=2)
        out_t.join(timeout=2)
        if proc.returncode != 0:
            raise ValueError(f'child failed with exit code {proc.returncode}; no retry')
        payload = json.loads(b''.join(out_chunks))
        private_write(stdout, payload)
        return payload

def native_dedup_keys(mappings):
    seen = set(); kept = []; dropped = []
    for m in mappings:
        server = m.get('server') or ''
        servername = m.get('servername') or ''
        password = m.get('password') or m.get('uuid') or ''
        sni = m.get('sni') or ''
        network = m.get('network') or ''
        key = f"{server}:{m.get('port')}:{servername}:{password}:{sni}:{network}"
        if key not in seen:
            seen.add(key); kept.append(m)
        else:
            dropped.append(m)
    return kept, dropped

def execute(a):
    p = read(a.manifest)
    nodes = verify_manifest(p, a.ledger)
    if not a.allow_network:
        raise ValueError('network disabled: explicit --allow-network and independent review required')
    review = read(a.review_report) if a.review_report else {}
    expected_change = p.get('change', DEFAULT_CHANGE)
    if review.get('gate') != 'reviewer' or review.get('verdict') not in ['PASS', 'PASSED'] or review.get('final') is not True or review.get('manifest_sha256') != p['manifest_sha256'] or review.get('change') != expected_change:
        raise ValueError('independent final review must bind this manifest and change')
    out = pathlib.Path(a.output)
    out.mkdir(mode=0o700, parents=True, exist_ok=False)
    check_disk_safety(out)
    ledger = Ledger(a.ledger)
    deadline = ledger.start(p['manifest_sha256'], p['seconds'])
    child_deadline = deadline - p['cleanup_seconds']
    start_time = time.time()
    mappings = json.loads(subprocess.run([p['binaries']['csp']['path']], input=canonical({'mode': 'freeze', 'nodes': nodes}), capture_output=True, check=True).stdout)['mappings']
    if digest(canonical(mappings)) != p['mappings_sha256']:
        ledger.close()
        raise ValueError('mapping changed')
    baseline = {}
    terminal = []
    groups = [(s, [n for n in ns if n in p['platforms']]) for s, ns in PLATFORMS.items()]
    groups = [(s, ns) for s, ns in groups if ns]
    all_stages = ['baseline'] + [s for s, _ in groups]
    try:
        with Bridge(ledger) as bridge:
            def launch(round_, side, stage, selected, names, limit):
                key = f'{round_}/{stage}'
                qs = {}
                for n in selected:
                    nid = n['logical_id']
                    scope = key + '/' + nid
                    ledger.add_quota(scope, limit)
                    qs[nid] = {'scope': scope, 'limit': limit}
                ledger.begin_attempt(key)
                common = {
                    'deadline': datetime.fromtimestamp(child_deadline, timezone.utc).isoformat().replace('+00:00', 'Z'),
                    'timeout_ms': 15000,
                    'quotas': qs,
                    'ledger_url': bridge.url,
                    'ledger_token': bridge.token,
                    'platforms': names,
                    'attempt': key
                }
                if side == 'csp':
                    common.update(nodes=selected, stage=stage, mode='stage')
                else:
                    common.update(mappings=[m for m in mappings if m['name'] in qs], stage='baseline' if stage == 'baseline' else 'media', mode='native' if side == 'native' else 'stage')
                
                def make_hb(now):
                    snap = ledger.snapshot()
                    exp = snap['experiment']
                    used = exp.get('used', 0)
                    res = exp.get('reserved', 0)
                    rem = max(0.0, deadline - now)
                    return f"[HEARTBEAT] side={side} stage={stage} elapsed={now - start_time:.1f}s consumed={used}B reserved={res}B nodes={len(selected)} deadline_remaining={rem:.1f}s"
                
                payload = invoke(p['binaries'][side]['path'], common, child_deadline, out / (key.replace('/', '-') + '.json'), out / (key.replace('/', '-') + '.stderr'), heartbeat_fn=make_hb)
                if payload.get('engine') != p['binaries'][side]['engine']:
                    raise ValueError('actual engine mismatch')
                ledger.finish_attempt(key, {'body_bytes': payload['body_bytes'], 'engine': payload['engine']})
                result = normalize(payload, side)
                for n in selected:
                    nid = n['logical_id']
                    row = result.get(nid, {'executed': False, 'queued': False, 'category': 'not_scheduled', 'reason': 'missing child record', 'node_latency': 'unknown'})
                    terminal.append({'node': nid, 'round': round_, 'side': side, 'stage': stage, **row})
                return result

            for r, side in enumerate(ORDER):
                baseline[r] = launch(r, side, 'baseline', nodes, [], 65536)
            eligible = intersection(nodes, baseline)
            private_write(out / 'eligibility.json', {'algorithm': p['quota_algorithm'], 'manifest_sha256': p['manifest_sha256'], 'eligible': eligible, 'discordant': [i['logical_id'] for i in nodes if i['logical_id'] not in eligible]})
            selected = [n for n in nodes if n['logical_id'] in eligible]
            if selected:
                limit = p['allocation']['platform_per_side'] // len(groups) // len(selected)
                for stage, names in groups:
                    for r, side in enumerate(ORDER):
                        if time.time() >= child_deadline:
                            raise ValueError('experiment deadline exhausted')
                        launch(r, side, stage, selected, names, limit)
            for n in nodes:
                if n['logical_id'] not in eligible:
                    for stage, _ in groups:
                        for r, side in enumerate(ORDER):
                            terminal.append({
                                'node': n['logical_id'],
                                'round': r,
                                'side': side,
                                'stage': stage,
                                'executed': False,
                                'queued': False,
                                'category': 'dependency_skipped',
                                'node_latency': 'unknown',
                                'causes': [baseline[i].get(n['logical_id'], {}).get('category', 'missing') for i in range(4)]
                            })
            for index in p['native_network_sides']:
                kept, dropped = native_dedup_keys(mappings)
                kept_nodes = [n for n in nodes if n['logical_id'] in {m['name'] for m in kept}]
                launch(4 + index, 'native', 'baseline', kept_nodes, [], 65536)
                for m in dropped:
                    terminal.append({
                        'node': m['name'],
                        'round': 4 + index,
                        'side': 'native',
                        'stage': 'baseline',
                        'executed': False,
                        'queued': False,
                        'category': 'skipped_duplicates',
                        'node_latency': 'unknown',
                        'reason': 'native connection deduplicated'
                    })
    finally:
        ledger.close()
        existing = {(r['node'], r['round'], r['stage']) for r in terminal}
        now_ts = datetime.now(timezone.utc).isoformat()
        for n in nodes:
            nid = n['logical_id']
            for r, side in enumerate(ORDER):
                for stage in all_stages:
                    if (nid, r, stage) not in existing:
                        terminal.append({
                            'node': nid,
                            'round': r,
                            'side': side,
                            'stage': stage,
                            'executed': False,
                            'queued': False,
                            'category': 'not_scheduled',
                            'node_latency': 'unknown',
                            'reason': 'unlaunched before stop',
                            'finished': now_ts
                        })
        private_write(out / 'terminal.private.json', terminal)
        private_write(out / 'ledger-final.json', ledger.snapshot())
        runner_exit = {
            'status': 'COMPLETED',
            'manifest_sha256': p['manifest_sha256'],
            'finished_at': now_ts,
            'elapsed_seconds': time.time() - start_time
        }
        private_write(out / 'runner-exit.json', runner_exit)
    return report_data(p, ledger, out)

def report_data(p, ledger, results):
    rows = read(pathlib.Path(results) / 'terminal.private.json')
    snap = ledger.snapshot()
    e = snap['experiment']
    units = {}
    cats = {}
    for r in rows:
        key = (r['stage'], r['side'])
        units.setdefault(key, {'attempted': 0, 'executed': 0, 'available': 0})
        units[key]['attempted'] += 1
        units[key]['executed'] += int(bool(r.get('executed')))
        units[key]['available'] += int(r.get('category') == 'available')
        c = r.get('category', 'unknown')
        cats[c] = cats.get(c, 0) + 1
    
    summary_md = f"""# CSP 对照实验脱敏汇总报告

- **Manifest SHA256**: `{p['manifest_sha256']}`
- **Change**: `{p.get('change', DEFAULT_CHANGE)}`
- **授权来源**: `{p.get('approval_source', 'none')}`
- **响应体消耗**: {e['used']} 字节
- **保守未结算预留**: {e['reserved']} 字节
- **网络线上流量**: 未知（仅计量应用层响应体）
- **核准总上限**: {e['cap']} 字节
- **耗时**: {time.time() - e['started'] if e['started'] else 0:.1f} 秒
- **总节点分母**: 90
- **阶段状态**:
"""
    for (stage, side), data in sorted(units.items()):
        summary_md += f"  - `{stage}` ({side}): 尝试 {data['attempted']} | 执行 {data['executed']} | 可用 {data['available']}\n"
    summary_md += f"""
- **局限与说明**:
  - Native 与 CSP 默认参数相互隔离；Native media 阶段不尝试
  - 平台检测方法差异：Netflix 存在区域子请求差异，IPRisk 存在国家定位与身份定位差异
  - 响应体排除了 TLS/TCP 握手与标头流量，未做持续吞吐测速
"""
    return {
        'status': 'PARTIAL',
        'manifest_sha256': p['manifest_sha256'],
        'change': p.get('change', DEFAULT_CHANGE),
        'approval_source': p.get('approval_source', ''),
        'body_bytes': e['used'],
        'unsettled_reserved_bytes': e['reserved'],
        'online_wire_bytes': 'unknown',
        'cap': e['cap'],
        'elapsed_seconds': time.time() - e['started'] if e['started'] else 0,
        'nodes': 90,
        'causal_categories': cats,
        'stage_units': [{'stage': s, 'side': side, **data} for (s, side), data in sorted(units.items())],
        'attempts': [{k: v for k, v in r.items() if k != 'raw'} for r in rows],
        'platform_support_matrix': p.get('platform_support_matrix', ALL_PLATFORM_MATRIX),
        'limitations': p['limitations'],
        'method_mismatches': p['method_mismatches'],
        'native_media': 'not_attempted',
        'no_permanent_dead_inference': True,
        'summary_markdown': summary_md
    }

def launch_daemon(a):
    p = read(a.manifest)
    nodes = verify_manifest(p, a.ledger)
    if not a.allow_network:
        raise ValueError('network disabled: explicit --allow-network and independent review required')
    review = read(a.review_report) if a.review_report else {}
    expected_change = p.get('change', DEFAULT_CHANGE)
    if review.get('gate') != 'reviewer' or review.get('verdict') not in ['PASS', 'PASSED'] or review.get('final') is not True or review.get('manifest_sha256') != p['manifest_sha256'] or review.get('change') != expected_change:
        raise ValueError('independent final review must bind this manifest and change')
    out_dir = pathlib.Path(a.output).resolve()
    if out_dir.exists():
        raise ValueError(f"results directory already exists: {out_dir}; execute requires fresh results directory to avoid overwrite")
    parent_dir = out_dir.parent
    parent_dir.mkdir(parents=True, exist_ok=True, mode=0o700)
    check_disk_safety(parent_dir)
    control_base = parent_dir / 'control'
    control_base.mkdir(parents=True, exist_ok=True, mode=0o700)
    
    # Check concurrent active process
    for d in control_base.iterdir():
        if d.is_dir() and d.name.startswith('launch_attempt_'):
            rec_file = d / 'daemon-receipt.json'
            if rec_file.exists():
                try:
                    rec = json.loads(rec_file.read_text())
                    old_pid = rec.get('pid')
                    if old_pid and is_pid_alive(old_pid):
                        raise ValueError(f"active daemon (pid {old_pid}) already running for this ledger/manifest in {d.name}")
                except (json.JSONDecodeError, OSError):
                    pass
    
    existing_attempts = sorted([d for d in control_base.iterdir() if d.is_dir() and d.name.startswith('launch_attempt_')])
    attempt_id = f"launch_attempt_{len(existing_attempts) + 1}"
    attempt_dir = control_base / attempt_id
    attempt_dir.mkdir(mode=0o700, parents=True, exist_ok=False)
    log_path = pathlib.Path(a.log).resolve() if getattr(a, 'log', None) else attempt_dir / 'daemon-progress.log'
    
    cmd = [
        sys.executable, str(pathlib.Path(__file__).resolve()), 'execute',
        '--manifest', str(pathlib.Path(a.manifest).resolve()),
        '--ledger', str(pathlib.Path(a.ledger).resolve()),
        '--output', str(out_dir),
        '--allow-network',
        '--review-report', str(pathlib.Path(a.review_report).resolve())
    ]
    
    receipt_path = attempt_dir / 'daemon-receipt.json'
    with open(log_path, 'a') as log_file:
        proc = subprocess.Popen(cmd, start_new_session=True, stdout=log_file, stderr=subprocess.STDOUT)
    
    receipt = {
        'pid': proc.pid,
        'launched_at': datetime.now(timezone.utc).isoformat(),
        'cmd': cmd,
        'manifest_sha256': p['manifest_sha256'],
        'change': expected_change,
        'attempt_id': attempt_id,
        'log_file': str(log_path),
        'output_dir': str(out_dir),
        'status': 'started'
    }
    private_write(receipt_path, receipt)
    print(json.dumps({'status': 'DAEMON_STARTED', 'pid': proc.pid, 'receipt': str(receipt_path)}))
    return receipt

def main():
    p = argparse.ArgumentParser()
    sub = p.add_subparsers(dest='command', required=True)
    
    plan = sub.add_parser('plan')
    plan.add_argument('--inventory', required=True)
    plan.add_argument('--output', required=True)
    for name in ['csp', 'reference', 'reference-source']:
        plan.add_argument('--' + name, required=True)
    plan.add_argument('--native')
    plan.add_argument('--platforms', default=','.join(sum(PLATFORMS.values(), [])))
    plan.add_argument('--change', default=DEFAULT_CHANGE)
    plan.add_argument('--approval-source', default='')
    for name, default in [
        ('response-body-budget-bytes', DEFAULT_CAP),
        ('deadline-seconds', DEFAULT_DEADLINE),
        ('baseline-body-limit-bytes', 65536),
        ('alive-concurrency', 8),
        ('media-concurrency', 2),
        ('speed-concurrency', 0),
        ('native-baseline-sides', 0)
    ]:
        plan.add_argument('--' + name, type=int, default=default)
    
    f = sub.add_parser('freeze')
    f.add_argument('--plan', required=True)
    f.add_argument('--manifest', required=True)
    f.add_argument('--ledger', required=True)
    
    e = sub.add_parser('execute')
    e.add_argument('--manifest', required=True)
    e.add_argument('--ledger', required=True)
    e.add_argument('--output', required=True)
    e.add_argument('--allow-network', action='store_true')
    e.add_argument('--review-report')
    
    r = sub.add_parser('report')
    r.add_argument('--manifest', required=True)
    r.add_argument('--ledger', required=True)
    r.add_argument('--results', required=True)
    r.add_argument('--output', required=True)
    
    d = sub.add_parser('launch-daemon')
    d.add_argument('--manifest', required=True)
    d.add_argument('--ledger', required=True)
    d.add_argument('--output', required=True)
    d.add_argument('--allow-network', action='store_true')
    d.add_argument('--review-report', required=True)
    d.add_argument('--log')
    
    a = p.parse_args()
    try:
        if a.command == 'plan':
            result = create_plan(a)
        elif a.command == 'freeze':
            result = freeze(a)
        elif a.command == 'execute':
            result = execute(a)
        elif a.command == 'launch-daemon':
            result = launch_daemon(a)
        else:
            m = read(a.manifest)
            verify_manifest(m, a.ledger)
            result = report_data(m, Ledger(a.ledger), a.results)
            private_write(a.output, result)
            # Also write public summary.md and summary.json alongside output
            out_p = pathlib.Path(a.output)
            (out_p.parent / 'summary.md').write_text(result.get('summary_markdown', ''))
            private_write(out_p.parent / 'summary.json', {k: v for k, v in result.items() if k != 'attempts'})
        print(json.dumps({'status': result.get('status', 'UNVERIFIED'), 'manifest_sha256': result.get('manifest_sha256'), 'command': a.command}))
    except (ValueError, OSError, subprocess.SubprocessError, KeyError, RuntimeError) as exc:
        print('comparison rejected: ' + str(exc), file=sys.stderr)
        return 1
    return 0

if __name__ == '__main__':
    sys.exit(main())
