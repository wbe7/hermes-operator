"""Run only inside the pinned official image; no credentials or remote calls."""
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest

@unittest.skipUnless(importlib.util.find_spec('gateway'), 'requires official Hermes image')
class UpstreamSessionTests(unittest.TestCase):
    def test_native_session_store_survives_override_reset(self):
        from unittest.mock import patch
        from gateway.config import GatewayConfig, Platform
        from gateway.session import SessionStore, SessionSource
        from adapters.v20260914 import reset_model_overrides
        with tempfile.TemporaryDirectory() as directory, patch.dict(os.environ, {'HERMES_HOME': directory}):
            home = Path(directory)
            store = SessionStore(home/'sessions', GatewayConfig())
            source = SessionSource(platform=Platform.TELEGRAM, chat_id='123', user_id='123')
            entry = store.get_or_create_session(source)
            sid = entry.session_id
            store.set_model_override(entry.session_key, {'model': 'temporary', 'provider': 'custom', 'base_url':'https://temporary.invalid/v1'})
            store.append_to_transcript(sid, {'role':'user','content':'preserve history'})
            reset_model_overrides(home)
            again=SessionStore(home/'sessions', GatewayConfig())
            current=again.get_or_create_session(source)
            self.assertEqual(current.session_id,sid)
            self.assertIsNone(again.get_model_override(entry.session_key))
            self.assertIn('preserve history',json.dumps(again.load_transcript(sid)))
            mirror=json.loads((home/'sessions/sessions.json').read_text())
            self.assertEqual(mirror[entry.session_key]['session_id'],sid)
            self.assertIsNone(mirror[entry.session_key].get('model_override'))

    def test_custom_pool_cannot_replace_declared_key(self):
        from unittest.mock import patch
        import yaml
        from adapters.v20260914 import reset_provider_credentials
        with tempfile.TemporaryDirectory() as directory, patch.dict(os.environ, {'HERMES_HOME':directory}):
            home=Path(directory)
            config={'model': {'provider':'custom','default':'declared','base_url':'https://declared.invalid/v1','api_key':'declared-test-key'},'custom_providers':[{'name':'personal-custom','base_url':'https://declared.invalid/v1','api_key':'old-test-key'}]}
            (home/'config.yaml').write_text(yaml.safe_dump(config))
            (home/'auth.json').write_text(json.dumps({'credential_pool':{'custom:personal-custom':[{'id':'old','api_key':'old-test-key'}], 'unrelated':[{'api_key':'keep'}]}}))
            reset_provider_credentials(home,config)
            (home/'config.yaml').write_text(yaml.safe_dump(config))
            auth=json.loads((home/'auth.json').read_text())
            self.assertNotIn('custom:personal-custom',auth['credential_pool'])
            self.assertEqual(auth['credential_pool']['unrelated'],[{'api_key':'keep'}])
            import subprocess, sys
            code = 'from hermes_cli.config import load_config; from hermes_cli.runtime_provider import resolve_runtime_provider; c=load_config(); r=resolve_runtime_provider(requested="custom",target_model=c["model"]["default"]); assert r["provider"]=="custom"; assert r["base_url"]=="https://declared.invalid/v1"; assert r["api_key"]=="declared-test-key"'
            result=subprocess.run([sys.executable, '-c', code],env={**os.environ, 'CUSTOM_BASE_URL':'', 'HERMES_HOME':directory},capture_output=True)
            self.assertEqual(result.returncode,0, 'native resolver verification failed: '+result.stderr.decode())
