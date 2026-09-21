"""Run only inside the pinned official image; no credentials or remote calls."""
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
from contextlib import ExitStack

@unittest.skipUnless(importlib.util.find_spec('gateway'), 'requires official Hermes image')
class UpstreamSessionTests(unittest.TestCase):
    def test_native_session_store_survives_override_reset(self):
        from unittest.mock import patch
        from gateway.config import GatewayConfig, Platform
        from gateway.session import SessionStore, SessionSource
        from adapters.v20260914 import reset_model_overrides
        with tempfile.TemporaryDirectory() as directory, patch.dict(os.environ, {'HERMES_HOME': directory}), ExitStack() as handles:
            home = Path(directory)
            store = SessionStore(home/'sessions', GatewayConfig())
            handles.callback(store.close_all_db_handles)
            source = SessionSource(platform=Platform.TELEGRAM, chat_id='123', user_id='123')
            entry = store.get_or_create_session(source)
            sid = entry.session_id
            store.set_model_override(entry.session_key, {'model': 'temporary', 'provider': 'custom', 'base_url':'https://temporary.invalid/v1'})
            store.append_to_transcript(sid, {'role':'user','content':'preserve history'})
            reset_model_overrides(home)
            again=SessionStore(home/'sessions', GatewayConfig())
            handles.callback(again.close_all_db_handles)
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

    def test_literal_credentials_and_legacy_gateway_in_fresh_resolver(self):
        import subprocess, sys
        from bootstrap import restore, leaves
        with tempfile.TemporaryDirectory() as directory:
            home=Path(directory)
            literal='prefix${HOME}${HERMES_MISSING_TEST_VARIABLE}suffix'
            (home/'.env').write_text("UNRELATED='"+literal+"'\nHOME='/tmp/wrong'\nHERMES_HOME='/tmp/wrong'\nHERMES_PROFILE='wrong'\nCUSTOM_BASE_URL='https://old.invalid/v1'\nMESSAGING_CWD='/stale'\nTERMINAL_CWD='/stale'\n")
            legacy={'multiplex_profiles':True,'platforms':{'telegram':{'channel_overrides':{'123':{'model':'legacy','provider':'legacy','system_prompt':'keep'}}}}}
            legacy['platforms']['telegram'].update(token='old-test-token', extra={'allow_from':['999'], 'group_allow_from':['999'], 'allowed_chats':['-999']})
            (home/'gateway.json').write_text(json.dumps(legacy))
            config={'model':{'provider':'custom','default':'declared','base_url':'https://declared.invalid/v1','api_key':{'credential':'model-api-key'}},'gateway':{'multiplex_profiles':False},'agent':{'reasoning_effort':'xhigh','reasoning_overrides':{}}}
            config['terminal']={'cwd':'/opt/data/workspace'}
            config['gateway']['platforms']={'telegram':{'enabled':True,'extra':{'allow_from':['123'],'group_allow_from':['123'],'allowed_chats':['__operator_dm_only__']}}}
            env={'HERMES_MODEL_API_KEY':{'credential':'model-api-key'},'CUSTOM_BASE_URL':'','TELEGRAM_BOT_TOKEN':{'credential':'telegram-bot-token'}}
            bundle={'schema':1,'release':'v2026.9.14','config':config,'env':env,'ownedPaths':[list(p[:-1]) if p[-1]=='credential' else list(p) for p,_ in leaves(config)],'ownedEnv':list(env)}
            restore(home,bundle,{'model-api-key':literal,'telegram-bot-token':'new-synthetic-test-token'})
            code='''import os,sys
from pathlib import Path
from bootstrap import load_runtime_environment
home=Path(sys.argv[1]); load_runtime_environment(home)
assert 'MESSAGING_CWD' not in os.environ and 'TERMINAL_CWD' not in os.environ
from hermes_cli.env_loader import load_hermes_dotenv
load_hermes_dotenv()
from hermes_cli.config import load_config
from hermes_cli.runtime_provider import resolve_runtime_provider
from gateway.config import load_gateway_config, Platform
c=load_config(); r=resolve_runtime_provider(requested='custom',target_model=c['model']['default'])
assert r['api_key']=='prefix${HOME}${HERMES_MISSING_TEST_VARIABLE}suffix'
assert os.environ['UNRELATED']=='prefix${HOME}${HERMES_MISSING_TEST_VARIABLE}suffix'
assert os.environ['HERMES_HOME']==str(home) and os.environ['HOME']==str(home)
assert 'HERMES_PROFILE' not in os.environ
assert c['terminal']['cwd']=='/opt/data/workspace'
# Native config loading bridges terminal.cwd into the tool environment itself.
assert os.environ.get('TERMINAL_CWD')=='/opt/data/workspace'
from hermes_cli.config import warn_deprecated_cwd_env_vars
warn_deprecated_cwd_env_vars()
from tools.terminal_tool import _get_env_config
assert _get_env_config()['cwd']=='/opt/data/workspace'
assert c['model']['default']=='declared' and r['base_url']=='https://declared.invalid/v1'
g=load_gateway_config(); assert not g.multiplex_profiles
t=g.platforms[Platform.TELEGRAM]
assert t.token=='new-synthetic-test-token'
assert t.extra['allow_from']==['123'] and t.extra['group_allow_from']==['123']
assert t.extra['allowed_chats']==['__operator_dm_only__']
channel=t.channel_overrides['123']
assert channel.model is None and channel.provider is None and channel.system_prompt=='keep'
'''
            result=subprocess.run([sys.executable,'-c',code,directory],env={**os.environ,'HERMES_HOME':directory},capture_output=True)
            self.assertEqual(result.returncode,0,result.stderr.decode())
            self.assertNotIn('Deprecated .env settings', result.stderr.decode())
