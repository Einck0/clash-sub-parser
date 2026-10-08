"""SQLite transactional body reservations; HTTP is loopback IPC, not a probe engine."""
import contextlib
import json
import secrets
import sqlite3
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

SCHEMA = '''
CREATE TABLE experiment (manifest TEXT PRIMARY KEY, cap INTEGER, started REAL, deadline REAL, used INTEGER DEFAULT 0, reserved INTEGER DEFAULT 0, state TEXT DEFAULT 'new');
CREATE TABLE quota (id TEXT PRIMARY KEY, cap INTEGER, used INTEGER DEFAULT 0, reserved INTEGER DEFAULT 0);
CREATE TABLE reservation (id TEXT PRIMARY KEY, scope TEXT, amount INTEGER, used INTEGER, state TEXT);
CREATE TABLE attempt (id TEXT PRIMARY KEY, state TEXT, receipt TEXT);
'''

class Ledger:
    def __init__(self, path):
        self.path = str(path)

    @contextlib.contextmanager
    def transaction(self):
        db = sqlite3.connect(self.path, timeout=5, isolation_level=None)
        try:
            db.execute('BEGIN IMMEDIATE')
            yield db
            db.commit()
        except BaseException:
            db.rollback()
            raise
        finally:
            db.close()

    def create(self, manifest, cap):
        with self.transaction() as db:
            db.executescript(SCHEMA)
            db.execute('INSERT INTO experiment(manifest,cap) VALUES (?,?)', (manifest, cap))

    def start(self, manifest, seconds):
        with self.transaction() as db:
            row = db.execute('SELECT manifest,state FROM experiment').fetchone()
            if row != (manifest, 'new'):
                raise ValueError('ledger binding/state mismatch: no automatic resume or retry')
            now = time.time()
            db.execute("UPDATE experiment SET started=?,deadline=?,state='running'", (now, now + seconds))
            return now + seconds

    def add_quota(self, scope, cap):
        with self.transaction() as db:
            db.execute('INSERT INTO quota(id,cap) VALUES (?,?)', (scope, cap))

    def reserve(self, scope, amount):
        if not 0 < amount <= 64 << 20:
            raise ValueError('invalid reservation')
        with self.transaction() as db:
            cap, used, reserved, deadline, state = db.execute('SELECT cap,used,reserved,deadline,state FROM experiment').fetchone()
            q = db.execute('SELECT cap,used,reserved FROM quota WHERE id=?', (scope,)).fetchone()
            if state != 'running' or time.time() >= deadline or q is None:
                raise ValueError('cancelled or unknown quota')
            n = min(amount, cap-used-reserved, q[0]-q[1]-q[2])
            if n <= 0:
                raise ValueError('body budget exhausted')
            token = secrets.token_hex(16)
            db.execute("INSERT INTO reservation VALUES (?,?,?,NULL,'reserved')", (token,scope,n))
            db.execute('UPDATE experiment SET reserved=reserved+?', (n,))
            db.execute('UPDATE quota SET reserved=reserved+? WHERE id=?', (n,scope))
            return {'id':token,'amount':n}

    def settle(self, token, used):
        with self.transaction() as db:
            row = db.execute('SELECT scope,amount,state FROM reservation WHERE id=?', (token,)).fetchone()
            if row is None or row[2] != 'reserved' or not 0 <= used <= row[1]:
                raise ValueError('invalid or duplicate settlement')
            scope, amount, _ = row
            db.execute("UPDATE reservation SET used=?,state='settled' WHERE id=?", (used,token))
            db.execute('UPDATE experiment SET used=used+?,reserved=reserved-?', (used,amount))
            db.execute('UPDATE quota SET used=used+?,reserved=reserved-? WHERE id=?', (used,amount,scope))

    def begin_attempt(self, key):
        with self.transaction() as db:
            db.execute("INSERT INTO attempt VALUES (?,'queued',NULL)", (key,))

    def finish_attempt(self, key, receipt):
        with self.transaction() as db:
            n=db.execute("UPDATE attempt SET state='finished',receipt=? WHERE id=? AND state='queued'", (json.dumps(receipt),key)).rowcount
            if n!=1: raise ValueError('duplicate or unknown attempt')

    def snapshot(self):
        with self.transaction() as db:
            db.row_factory=sqlite3.Row
            return {'experiment':dict(db.execute('SELECT * FROM experiment').fetchone()),
                    'quotas':[dict(x) for x in db.execute('SELECT * FROM quota ORDER BY id')],
                    'attempts':[dict(x) for x in db.execute('SELECT * FROM attempt ORDER BY id')]}

    def close(self):
        with self.transaction() as db:
            db.execute("UPDATE experiment SET state='closed'")
            # Unsettled reservations remain charged. Never refund on crash.

class Bridge:
    def __init__(self, ledger):
        self.token=secrets.token_hex(32)
        bridge=self
        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *args): pass
            def do_GET(self):
                if self.path == '/ping':
                    self.send_response(200)
                    self.send_header('Content-Length', '11')
                    self.end_headers()
                    self.wfile.write(b'{"ok":true}')
                else:
                    self.send_response(404)
                    self.end_headers()
            def do_POST(self):
                try:
                    if self.headers.get('Authorization') != 'Bearer '+bridge.token:
                        raise ValueError('unauthorized')
                    size=int(self.headers.get('Content-Length','0'))
                    if not 0<size<=4096: raise ValueError('invalid input')
                    x=json.loads(self.rfile.read(size))
                    if self.path=='/reserve': result=ledger.reserve(x['scope'],x['amount'])
                    elif self.path=='/settle': ledger.settle(x['id'],x['used']);result={}
                    else: raise ValueError('unknown operation')
                    data=json.dumps(result).encode();self.send_response(200)
                except Exception:
                    data=b'{"error":"budget_or_ledger_rejected"}';self.send_response(409)
                self.send_header('Content-Length',str(len(data)));self.end_headers();self.wfile.write(data)
        self.server=ThreadingHTTPServer(('127.0.0.1',0),Handler)
        self.thread=threading.Thread(target=self.server.serve_forever,daemon=True)
    def __enter__(self):
        self.thread.start();return self
    @property
    def url(self): return 'http://127.0.0.1:'+str(self.server.server_port)
    def __exit__(self,*args):
        self.server.shutdown();self.server.server_close();self.thread.join()
