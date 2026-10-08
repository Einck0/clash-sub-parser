import concurrent.futures
import importlib.util
import pathlib
import sys
import tempfile
import time
import unittest
from unittest.mock import patch
sys.path.insert(0,str(pathlib.Path(__file__).parent))
import run
from ledger import Ledger, Bridge

class ExperimentTests(unittest.TestCase):
 def test_golden_allocation(self):
  self.assertEqual(run.allocation_plan(),{'baseline_per_node':65536,'baseline_total':23592960,'platform_per_side':27656192,'unused_rounding':0})
  self.assertEqual(run.allocation_plan(native=2)['baseline_total'],35389440)
  for x in [0,512<<20]:
   with self.assertRaises(ValueError):run.allocation_plan(x)
 def test_intersection_requires_every_executed_available(self):
  nodes=[{'logical_id':x} for x in ['a','b','c']]
  rows={r:{x:{'category':'available','executed':True} for x in ['a','b','c']} for r in range(4)}
  rows[2]['b']['executed']=False;rows[3]['c']['category']='restricted'
  self.assertEqual(run.intersection(nodes,rows),['a'])
 def test_ledger_concurrent_refunds_crash_and_relaunch(self):
  with tempfile.TemporaryDirectory() as d:
   l=Ledger(pathlib.Path(d)/'ledger.db');l.create('manifest',100);l.start('manifest',1800)
   for i in range(10):l.add_quota(str(i),10)
   def work(i):
    a=l.reserve(str(i),10);l.settle(a['id'],3)
   with concurrent.futures.ThreadPoolExecutor(max_workers=10) as pool:list(pool.map(work,range(10)))
   self.assertEqual(l.snapshot()['experiment']['used'],30)
   ticket=l.reserve('0',7)
   second=Ledger(l.path)
   self.assertEqual(second.snapshot()['experiment']['reserved'],7)
   with self.assertRaises(ValueError):second.start('manifest',1800)
   with self.assertRaises(ValueError):second.reserve('0',1)
   l.settle(ticket['id'],0)
   with self.assertRaises(ValueError):l.settle(ticket['id'],0)
   l.close()
   with self.assertRaises(ValueError):l.reserve('1',1)
 def test_deadline_no_renewal(self):
  with tempfile.TemporaryDirectory() as d:
   l=Ledger(pathlib.Path(d)/'ledger.db');l.create('m',10);l.start('m',-1);l.add_quota('x',10)
   with self.assertRaises(ValueError):l.reserve('x',1)
 def test_no_network_default_and_review_cannot_be_bypassed(self):
  import argparse
  a=argparse.Namespace(manifest='x',ledger='y',allow_network=False,review_report=None)
  with patch.object(run,'read',return_value={}),patch.object(run,'verify_manifest',return_value=[]),patch.object(run,'invoke') as invoke:
   with self.assertRaisesRegex(ValueError,'network disabled'):run.execute(a)
   a.allow_network=True
   with self.assertRaisesRegex(ValueError,'review'):run.execute(a)
   invoke.assert_not_called()
 def test_manifest_tamper_rejected_before_launch(self):
  with self.assertRaisesRegex(ValueError,'tampered'):run.verify_manifest({'manifest_sha256':'not-real','speed_enabled':True},'x')
if __name__=='__main__':unittest.main()
