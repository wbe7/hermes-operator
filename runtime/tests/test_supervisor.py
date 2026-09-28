import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import unittest
from unittest.mock import patch, MagicMock
import psutil
from supervisor import dashboard_healthy

class SupervisorTests(unittest.TestCase):
    def test_failed_child_stops_sibling_and_exits_failure(self):
        with tempfile.TemporaryDirectory() as d:
            marker=Path(d)/'processes.json'
            script='from supervisor import supervise; from pathlib import Path; import sys; raise SystemExit(supervise({"gateway":[sys.executable,"-c","import time;time.sleep(60)"],"dashboard":[sys.executable,"-c","import time;time.sleep(0.5);raise SystemExit(7)"]},Path(sys.argv[1]),grace=0.5))'
            p=subprocess.run([sys.executable,'-c',script,str(marker)],capture_output=True,timeout=10)
            self.assertEqual(p.returncode,1,p.stderr.decode())
            self.assertNotIn("Traceback",p.stderr.decode())
            self.assertFalse(marker.exists(),'stale live process marker remains')

    def test_termination_is_forwarded(self):
        with tempfile.TemporaryDirectory() as d:
            marker=Path(d)/'processes.json'
            script='from supervisor import supervise; from pathlib import Path; import sys; raise SystemExit(supervise({"gateway":[sys.executable,"-c","import time;time.sleep(60)"],"dashboard":[sys.executable,"-c","import time;time.sleep(60)"]},Path(sys.argv[1]),grace=0.5))'
            p=subprocess.Popen([sys.executable,'-c',script,str(marker)],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
            try:
                until=time.monotonic()+5
                while not marker.exists() and p.poll() is None and time.monotonic()<until:time.sleep(.05)
                self.assertTrue(marker.exists(),'supervisor failed to start')
                identities=json.loads(marker.read_text())
                p.terminate();self.assertEqual(p.wait(timeout=5),0)
                for identity in identities.values():
                    self.assertFalse(psutil.pid_exists(identity["pid"]),"child survived supervisor")
                self.assertFalse(marker.exists())
            finally:
                if p.poll() is None:p.kill();p.wait()

    def test_dashboard_probe_requires_auth_and_matching_process(self):
        with tempfile.TemporaryDirectory() as d:
            marker=Path(d)/'processes.json'
            identity={'pid':os.getpid(),'started':psutil.Process().create_time()}
            marker.write_text(json.dumps({'dashboard':identity}))
            for status, expected in [({'auth_required':True,'auth_providers':['basic']},True),({'auth_required':False,'auth_providers':['basic']},False),({'auth_required':True,'auth_providers':[]},False)]:
                with patch('urllib.request.build_opener') as factory:
                    factory.return_value.open.return_value.__enter__.return_value.read.return_value=json.dumps(status).encode()
                    self.assertEqual(dashboard_healthy(marker),expected)
            identity['started']-=1
            marker.write_text(json.dumps({'dashboard':identity}))
            self.assertFalse(dashboard_healthy(marker))
