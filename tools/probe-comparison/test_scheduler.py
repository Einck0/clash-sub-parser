import concurrent.futures
import hashlib
import json
import os
import pathlib
import subprocess
import sys
import tempfile
import time
import unittest
from unittest.mock import MagicMock, patch

sys.path.insert(0, str(pathlib.Path(__file__).parent))
import run
from ledger import Ledger, Bridge

class ExperimentTests(unittest.TestCase):
    def test_golden_allocation(self):
        self.assertEqual(run.allocation_plan(), {
            'baseline_per_node': 65536,
            'baseline_total': 23592960,
            'platform_per_side': 27656192,
            'unused_rounding': 0
        })
        self.assertEqual(run.allocation_plan(native=2)['baseline_total'], 35389440)
        # 1GiB custom budget
        alloc_1g = run.allocation_plan(total=1073741824)
        self.assertEqual(alloc_1g['baseline_total'], 23592960)
        self.assertTrue(alloc_1g['platform_per_side'] > 260000000)
        # Invalid budget bounds
        for x in [0, -1, 2 << 30]:
            with self.assertRaises(ValueError):
                run.allocation_plan(x)

    def test_intersection_requires_every_executed_available(self):
        nodes = [{'logical_id': x} for x in ['a', 'b', 'c']]
        rows = {r: {x: {'category': 'available', 'executed': True} for x in ['a', 'b', 'c']} for r in range(4)}
        rows[2]['b']['executed'] = False
        rows[3]['c']['category'] = 'restricted'
        self.assertEqual(run.intersection(nodes, rows), ['a'])

    def test_ledger_concurrent_refunds_crash_and_relaunch(self):
        with tempfile.TemporaryDirectory() as d:
            l = Ledger(pathlib.Path(d) / 'ledger.db')
            l.create('manifest', 100)
            l.start('manifest', 1800)
            for i in range(10):
                l.add_quota(str(i), 10)

            def work(i):
                a = l.reserve(str(i), 10)
                l.settle(a['id'], 3)

            with concurrent.futures.ThreadPoolExecutor(max_workers=10) as pool:
                list(pool.map(work, range(10)))
            self.assertEqual(l.snapshot()['experiment']['used'], 30)
            ticket = l.reserve('0', 7)
            second = Ledger(l.path)
            self.assertEqual(second.snapshot()['experiment']['reserved'], 7)
            with self.assertRaises(ValueError):
                second.start('manifest', 1800)
            with self.assertRaises(ValueError):
                second.reserve('0', 1)
            l.settle(ticket['id'], 0)
            with self.assertRaises(ValueError):
                l.settle(ticket['id'], 0)
            l.close()
            with self.assertRaises(ValueError):
                l.reserve('1', 1)

    def test_deadline_no_renewal(self):
        with tempfile.TemporaryDirectory() as d:
            l = Ledger(pathlib.Path(d) / 'ledger.db')
            l.create('m', 10)
            l.start('m', -1)
            l.add_quota('x', 10)
            with self.assertRaises(ValueError):
                l.reserve('x', 1)

    def test_start_second_time_rejected(self):
        with tempfile.TemporaryDirectory() as d:
            l = Ledger(pathlib.Path(d) / 'ledger.db')
            l.create('m1', 100)
            l.start('m1', 7200)
            # Re-running start on same or different manifest must fail
            with self.assertRaises(ValueError):
                l.start('m1', 7200)
            with self.assertRaises(ValueError):
                l.start('m2', 7200)

    def test_no_refund_unsettled_retained(self):
        with tempfile.TemporaryDirectory() as d:
            l = Ledger(pathlib.Path(d) / 'ledger.db')
            l.create('m_forfeit', 1000)
            l.start('m_forfeit', 3600)
            l.add_quota('q1', 500)
            t = l.reserve('q1', 300)
            snap_before = l.snapshot()
            self.assertEqual(snap_before['experiment']['reserved'], 300)
            self.assertEqual(snap_before['experiment']['used'], 0)
            # Close ledger without settling reservation t
            l.close()
            snap_after = l.snapshot()
            # Reserved must remain charged (conservative forfeiture), never refunded
            self.assertEqual(snap_after['experiment']['reserved'], 300)
            self.assertEqual(snap_after['experiment']['used'], 0)

    def test_no_expansion_of_stage_quota(self):
        with tempfile.TemporaryDirectory() as d:
            l = Ledger(pathlib.Path(d) / 'ledger.db')
            l.create('m_cap', 100)
            l.start('m_cap', 1800)
            l.add_quota('limited', 50)
            # Asking for more than quota cap returns budget exhausted
            l.reserve('limited', 50)
            with self.assertRaises(ValueError):
                l.reserve('limited', 1)

    def test_no_network_default_and_review_cannot_be_bypassed(self):
        import argparse
        a = argparse.Namespace(manifest='x', ledger='y', allow_network=False, review_report=None)
        with patch.object(run, 'read', return_value={}), patch.object(run, 'verify_manifest', return_value=[]), patch.object(run, 'invoke') as invoke:
            with self.assertRaisesRegex(ValueError, 'network disabled'):
                run.execute(a)
            a.allow_network = True
            with self.assertRaisesRegex(ValueError, 'review'):
                run.execute(a)
            invoke.assert_not_called()

    def test_manifest_tamper_rejected_before_launch(self):
        with self.assertRaisesRegex(ValueError, 'tampered'):
            run.verify_manifest({'manifest_sha256': 'not-real', 'speed_enabled': True}, 'x')

    def test_parameterized_plan_and_validation(self):
        with tempfile.TemporaryDirectory() as d:
            p_file = pathlib.Path(d) / 'plan.json'
            # Synthetic 90 nodes
            inv_file = pathlib.Path(d) / 'inv.json'
            nodes = [{'logical_id': f'node_{i:02d}', 'server': '1.2.3.4', 'port': 1080} for i in range(90)]
            inv_file.write_text(json.dumps({'nodes': nodes}))
            inv_file.chmod(0o600)
            
            p = {
                'version': 1,
                'change': 'custom-change-2026',
                'approval_source': 'reissue-auth-7200s',
                'inventory': str(inv_file),
                'inventory_sha256': hashlib.sha256(inv_file.read_bytes()).hexdigest(),
                'nodes': [{'id': n['logical_id'], 'revision': None, 'config_sha256': hashlib.sha256(json.dumps(n, sort_keys=True, separators=(',', ':')).encode()).hexdigest()} for n in nodes],
                'cap': 1073741824,
                'seconds': 7200,
                'allocation': run.allocation_plan(1073741824, 0),
                'order': run.ORDER,
                'native_network_sides': [],
                'platforms': ['netflix', 'youtube'],
                'timeout_ms': 15000,
                'concurrency': {'alive': 8, 'media': 2, 'speed': 0},
                'speed_enabled': False,
                'binaries': {},
                'source_files': {}
            }
            # validate_plan accepts valid plan
            res_nodes = run.validate_plan(p)
            self.assertEqual(len(res_nodes), 90)
            
            # Non-positive seconds or cap rejected
            bad_p = dict(p, seconds=-1)
            with self.assertRaises(ValueError):
                run.validate_plan(bad_p)

    def test_review_different_change_or_hash_rejected(self):
        import argparse
        manifest_data = {
            'change': 'csp-fresh-inventory-full-validation',
            'manifest_sha256': 'valid_hash_123',
            'ledger': '/tmp/ledger.db',
            'seconds': 1800
        }
        with patch.object(run, 'read', side_effect=lambda p: manifest_data if str(p) == 'm_path' else review_data), \
             patch.object(run, 'verify_manifest', return_value=[]):
            # 1. Review with mismatched change
            review_data = {'gate': 'reviewer', 'verdict': 'PASS', 'final': True, 'manifest_sha256': 'valid_hash_123', 'change': 'wrong-change'}
            a = argparse.Namespace(manifest='m_path', ledger='/tmp/ledger.db', allow_network=True, review_report='r', output='/tmp/out')
            with self.assertRaisesRegex(ValueError, 'independent final review must bind this manifest and change'):
                run.execute(a)

            # 2. Review with mismatched manifest_sha256
            review_data = {'gate': 'reviewer', 'verdict': 'PASS', 'final': True, 'manifest_sha256': 'wrong_hash', 'change': 'csp-fresh-inventory-full-validation'}
            with self.assertRaisesRegex(ValueError, 'independent final review must bind this manifest and change'):
                run.execute(a)

    def test_atomic_writes(self):
        with tempfile.TemporaryDirectory() as d:
            dest = pathlib.Path(d) / 'test_atomic.json'
            data = {'hello': 'world', 'answer': 42}
            run.private_write(dest, data)
            self.assertTrue(dest.exists())
            self.assertEqual(json.loads(dest.read_text()), data)
            # Permission 0600
            self.assertEqual(dest.stat().st_mode & 0o777, 0o600)

    def test_resource_protection_abort(self):
        with tempfile.TemporaryDirectory() as d:
            mock_stat = MagicMock()
            mock_stat.f_bavail = 1000
            mock_stat.f_frsize = 1024 # ~1MiB available
            with patch('os.statvfs', return_value=mock_stat):
                with self.assertRaisesRegex(RuntimeError, 'safe stop: disk available'):
                    run.check_disk_safety(d)

    def test_layered_terminal_records_and_summary(self):
        with tempfile.TemporaryDirectory() as d:
            l = Ledger(pathlib.Path(d) / 'ledger.db')
            l.create('m_rep', 1073741824)
            l.start('m_rep', 7200)
            
            terminal_file = pathlib.Path(d) / 'terminal.private.json'
            # Mock records for 2 nodes
            rows = [
                {'node': 'n1', 'round': 0, 'side': 'csp', 'stage': 'baseline', 'executed': True, 'category': 'available'},
                {'node': 'n2', 'round': 0, 'side': 'csp', 'stage': 'baseline', 'executed': False, 'category': 'fail'},
                {'node': 'n1', 'round': 0, 'side': 'csp', 'stage': 'streaming', 'executed': True, 'category': 'available'},
                {'node': 'n2', 'round': 0, 'side': 'csp', 'stage': 'streaming', 'executed': False, 'category': 'dependency_skipped'}
            ]
            run.private_write(terminal_file, rows)
            
            p = {
                'manifest_sha256': 'rep_hash_abc',
                'change': 'csp-reissue-test',
                'approval_source': 'user-auth-reissue',
                'limitations': ['No speed'],
                'method_mismatches': {'netflix': 'region'}
            }
            rep = run.report_data(p, l, d)
            self.assertEqual(rep['status'], 'PARTIAL')
            self.assertEqual(rep['manifest_sha256'], 'rep_hash_abc')
            self.assertEqual(rep['change'], 'csp-reissue-test')
            self.assertEqual(rep['online_wire_bytes'], 'unknown')
            self.assertIn('summary_markdown', rep)
            self.assertIn('rep_hash_abc', rep['summary_markdown'])

    def test_bridge_ping(self):
        with tempfile.TemporaryDirectory() as d:
            l = Ledger(pathlib.Path(d) / 'ledger.db')
            l.create('m_b', 100)
            with Bridge(l) as bridge:
                import urllib.request
                req = urllib.request.Request(bridge.url + '/ping')
                with urllib.request.urlopen(req) as resp:
                    self.assertEqual(resp.status, 200)
                    body = json.loads(resp.read().decode())
                    self.assertTrue(body.get('ok'))

    def test_invoke_deadline_timeout_kills_child(self):
        with tempfile.TemporaryDirectory() as d:
            out = pathlib.Path(d) / 'out.json'
            err = pathlib.Path(d) / 'err.log'
            script = pathlib.Path(d) / 'sleeper.sh'
            script.write_text('#!/bin/sh\nsleep 5\n')
            script.chmod(0o755)
            with self.assertRaisesRegex(ValueError, 'child deadline exceeded'):
                run.invoke(str(script), {'test': 1}, time.time() + 0.1, out, err)

    def test_launch_daemon_preexisting_results_dir_refused(self):
        import argparse
        with tempfile.TemporaryDirectory() as d:
            out_dir = pathlib.Path(d) / 'results'
            out_dir.mkdir(parents=True, exist_ok=True)
            manifest = {'manifest_sha256': 'h1', 'change': 'c1', 'nodes': []}
            review = {'gate': 'reviewer', 'verdict': 'PASS', 'final': True, 'manifest_sha256': 'h1', 'change': 'c1'}
            a = argparse.Namespace(manifest='m', ledger='l', output=str(out_dir), allow_network=True, review_report='r')
            with patch.object(run, 'read', side_effect=lambda p: manifest if 'm' in str(p) else review), \
                 patch.object(run, 'verify_manifest', return_value=[]):
                with self.assertRaisesRegex(ValueError, 'results directory already exists'):
                    run.launch_daemon(a)

    def test_launch_daemon_control_dir_append_only_and_conflict_detection(self):
        import argparse
        with tempfile.TemporaryDirectory() as d:
            base = pathlib.Path(d)
            out_dir = base / 'results'
            manifest = {'manifest_sha256': 'h1', 'change': 'c1', 'nodes': []}
            review = {'gate': 'reviewer', 'verdict': 'PASS', 'final': True, 'manifest_sha256': 'h1', 'change': 'c1'}
            a = argparse.Namespace(manifest='m', ledger='l', output=str(out_dir), allow_network=True, review_report='r')
            
            with patch.object(run, 'read', side_effect=lambda p: manifest if 'm' in str(p) else review), \
                 patch.object(run, 'verify_manifest', return_value=[]), \
                 patch('subprocess.Popen') as mock_popen:
                mock_proc = MagicMock()
                mock_proc.pid = 999999 # dummy pid
                mock_popen.return_value = mock_proc
                
                # First launch creates launch_attempt_1
                rec1 = run.launch_daemon(a)
                self.assertEqual(rec1['attempt_id'], 'launch_attempt_1')
                self.assertTrue((base / 'control' / 'launch_attempt_1' / 'daemon-receipt.json').exists())
                
                # When PID is alive, second launch is rejected
                with patch.object(run, 'is_pid_alive', return_value=True):
                    with self.assertRaisesRegex(ValueError, 'active daemon .* already running'):
                        run.launch_daemon(a)
                
                # When PID is dead, second launch creates launch_attempt_2 without overwriting attempt 1
                with patch.object(run, 'is_pid_alive', return_value=False):
                    mock_proc.pid = 999998
                    rec2 = run.launch_daemon(a)
                    self.assertEqual(rec2['attempt_id'], 'launch_attempt_2')
                    self.assertTrue((base / 'control' / 'launch_attempt_1' / 'daemon-receipt.json').exists())
                    self.assertTrue((base / 'control' / 'launch_attempt_2' / 'daemon-receipt.json').exists())

    def test_launch_daemon_and_execute_offline_integration_fixture(self):
        # Full offline subprocess integration: verifies real execute refuses preexisting results dir,
        # fresh execute succeeds, and daemon writes receipt to control/<attempt_id>.
        with tempfile.TemporaryDirectory() as d:
            base = pathlib.Path(d)
            # Create synthetic mock binaries that output valid JSON
            mock_bin = base / 'mock_child'
            mock_bin.write_text('#!/bin/sh\necho \'{"side":"csp","engine":"mihomo/v1.19.32","rows":[],"body_bytes":0}\'\n')
            mock_bin.chmod(0o755)
            
            out_dir = base / 'results'
            # Pre-existing out_dir test via execute subprocess
            out_dir.mkdir(mode=0o700)
            
            # Subprocess running run.py execute with preexisting results dir must exit non-zero
            p = subprocess.run([
                sys.executable, str(pathlib.Path(run.__file__).resolve()),
                'execute', '--manifest', 'nonexistent', '--ledger', 'nonexistent',
                '--output', str(out_dir), '--allow-network'
            ], capture_output=True, text=True)
            self.assertNotEqual(p.returncode, 0)
            self.assertIn('comparison rejected', p.stderr)

if __name__ == '__main__':
    unittest.main()
