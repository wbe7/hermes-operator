"""Exercise the actual embedded read-only snapshot against a synthetic SQLite home."""
import ast
import contextlib
import io
import json
from pathlib import Path
import sqlite3
import sys
import tempfile
import types
import unittest
from unittest.mock import patch

class SnapshotTests(unittest.TestCase):
    def test_same_count_history_personal_config_and_cron_changes_are_detected(self):
        module=ast.parse((Path(__file__).resolve().parents[3]/'hack/e2e-live.py').read_text())
        code=next(ast.literal_eval(n.value) for n in module.body if isinstance(n,ast.Assign) and any(isinstance(t,ast.Name) and t.id=='script' for t in n.targets))
        with tempfile.TemporaryDirectory() as folder:
            home=Path(folder);(home/'cron').mkdir();(home/'cron/jobs.json').write_text('{"fixture":1}')
            (home/'cron/ticker_heartbeat').write_text('1');(home/'gateway_state.json').write_text('{"pid":1}')
            (home/'input.json').write_text(json.dumps({'env':{},'config':{'model':{'default':'declared','provider':'custom'},'agent':{'reasoning_effort':'xhigh'}},'ownedPaths':[['model'],['agent']]}))
            db=sqlite3.connect(home/'state.db')
            db.execute('CREATE TABLE messages (id INTEGER,session_id TEXT,role TEXT,content TEXT,tool_call_id TEXT,tool_calls TEXT,tool_name TEXT,reasoning TEXT,reasoning_content TEXT,platform_message_id TEXT)')
            db.execute("INSERT INTO messages VALUES (1,'session','user','original',NULL,NULL,NULL,NULL,NULL,NULL)");db.commit()
            config={'model':{'default':'declared','provider':'custom'},'agent':{'reasoning_effort':'xhigh'},'personal':{'style':'short'}}
            bootstrap=types.ModuleType('bootstrap');bootstrap.load_runtime_environment=lambda home:None
            cfg=types.ModuleType('hermes_cli.config');cfg.load_config=lambda:config
            provider=types.ModuleType('hermes_cli.runtime_provider');provider.resolve_runtime_provider=lambda **kwargs:{}
            def snapshot():
                capture=io.StringIO()
                with patch.dict(sys.modules,{'bootstrap':bootstrap,'hermes_cli':types.ModuleType('hermes_cli'),'hermes_cli.config':cfg,'hermes_cli.runtime_provider':provider}),patch.object(sys,'path',list(sys.path)),contextlib.redirect_stdout(capture):
                    exec(code.replace('/opt/data',str(home)).replace('/operator/config/input.json',str(home/'input.json')), {})
                return json.loads(capture.getvalue())
            before=snapshot()
            (home/'cron/ticker_heartbeat').write_text('2')
            self.assertEqual(before,snapshot())
            db.execute("UPDATE messages SET content='edited' WHERE id=1");db.commit()
            changed=snapshot();self.assertEqual(before['sessions'][0][1],changed['sessions'][0][1]);self.assertNotEqual(before['sessions'][0][2],changed['sessions'][0][2])
            config['personal']['style']='long';self.assertNotEqual(before['personalConfigSHA256'],snapshot()['personalConfigSHA256'])
            (home/'cron/jobs.json').write_text('{"fixture":2}');self.assertNotEqual(before['files'],snapshot()['files'])
            db.close()

if __name__=='__main__':unittest.main()
