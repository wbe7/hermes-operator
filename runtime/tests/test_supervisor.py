import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import unittest

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
                p.terminate();self.assertEqual(p.wait(timeout=5),0)
                self.assertFalse(marker.exists())
            finally:
                if p.poll() is None:p.kill();p.wait()
