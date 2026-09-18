import json
from pathlib import Path
import tempfile
import time
import unittest
from unittest.mock import patch
from adapters.v20260914 import _health

class ProbeTests(unittest.TestCase):
    def test_idle_snapshot_and_remote_outage_do_not_break_liveness(self):
        with tempfile.TemporaryDirectory() as directory:
            home = Path(directory)
            (home/'state').mkdir()
            record = {'pid': 42, 'start_time': 100, 'gateway_state': 'running', 'session_store': {'status': 'ok'}, 'platforms': {'telegram': {'state': 'connected', 'writer_pid': 42, 'writer_start_time': 100}}, 'updated_at': '2000-01-01T00:00:00Z'}
            (home/'state/gateway.heartbeat').write_text(json.dumps({'pid': 42, 'start_time': 100, 'monotonic': time.monotonic(), 'loop_tick_socket': True}))
            with patch('adapters.v20260914._runtime_status', return_value=(record, (42, 100))), patch('adapters.v20260914._tick', return_value=True), patch('adapters.v20260914._created_at', return_value=99):
                self.assertTrue(_health(home, False))
                self.assertTrue(_health(home, True))
                record['platforms']['telegram']['state'] = 'retrying'
                self.assertTrue(_health(home, False))
                self.assertFalse(_health(home, True))
                record['platforms']['telegram']['state'] = 'connected'
                record['platforms']['telegram']['writer_start_time'] = 99
                self.assertFalse(_health(home, True))

    def test_missing_or_malformed_state_fails_closed(self):
        with tempfile.TemporaryDirectory() as directory:
            with patch('adapters.v20260914._runtime_status', return_value=({}, None)):
                self.assertFalse(_health(Path(directory), False))
