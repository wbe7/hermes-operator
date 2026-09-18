"""Synthetic Go-renderer -> bootstrap -> native resolver/authorization contract."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
sys.path[:0] = ['/runtime', '/opt/hermes']
import yaml
from bootstrap import restore

native = r'''
import sys
sys.path[:0] = ['/runtime', '/opt/hermes']
from pathlib import Path
from bootstrap import load_runtime_environment
home=Path(sys.argv[1]); scenario=sys.argv[2]
load_runtime_environment(home)
from hermes_cli.config import load_config
from hermes_cli.runtime_provider import resolve_runtime_provider
from gateway.config import load_gateway_config, Platform
from gateway.pairing import PairingStore
from gateway.session import SessionSource
from gateway.authz_mixin import GatewayAuthorizationMixin
from plugins.platforms.telegram.adapter import TelegramAdapter
cfg=load_config()
r=resolve_runtime_provider(requested='custom',target_model=cfg['model']['default'])
assert r['provider']=='custom' and r['base_url']==('https://openrouter.ai/api/v1' if scenario.endswith('-host') else 'https://inference.invalid/v1')
assert r['api_key']==('no-key-required' if scenario.startswith('noauth') else 'SENTINEL${HOME}')
assert r['api_mode']=='chat_completions', (scenario, r['api_mode'])
assert cfg['model']['default']=='test-model'
assert not cfg['gateway']['proxy_url']
assert not __import__('os').environ['GATEWAY_PROXY_URL']
assert cfg['agent']['reasoning_effort']=='xhigh'
assert cfg['agent']['reasoning_overrides']==({'test-model':'high'} if scenario=='explicit' else {})
g=load_gateway_config(); assert not g.multiplex_profiles
assert [p.value for p in g.get_connected_platforms()]==['telegram']
t=g.platforms[Platform.TELEGRAM]
assert t.token=='SENTINEL-token'
assert t.extra['allow_from']==['123'] and t.extra['group_allow_from']==['123']
assert t.extra['allowed_chats']==(['-456'] if scenario=='explicit' else ['__operator_dm_only__'])
assert not t.extra['guest_mode']
assert t.channel_overrides['123'].model is None
assert t.channel_overrides['123'].system_prompt=='personal channel'
runner=GatewayAuthorizationMixin(); runner.config=g
runner.adapters={Platform.TELEGRAM:TelegramAdapter(t)}
runner.pairing_store=PairingStore()
assert not runner.pairing_store.is_approved('telegram','999')
assert runner.pairing_store.is_approved('discord','other')
for uid,expected in [('123',True),('999',False)]:
 source=SessionSource(platform=Platform.TELEGRAM,user_id=uid,chat_id=uid,chat_type='private')
 assert runner._is_user_authorized(source)==expected
'''

def apply(home, scenario):
    directory=Path('/fixtures')/scenario
    bundle=json.loads((directory/'input.json').read_text())
    creds={p.name:p.read_text() for p in (directory/'credentials').iterdir()}
    restore(home,bundle,creds)
    proc=subprocess.run([sys.executable,'-I','-c',native,str(home),scenario],capture_output=True)
    if proc.returncode:
        raise AssertionError(proc.stderr.decode())

for scenario in ('default','explicit','noauth','custom-host','noauth-host'):
    with tempfile.TemporaryDirectory() as directory:
        home=Path(directory)
        cfg=yaml.safe_load(Path('/checks/personal-paths.yaml').read_text())
        cfg['custom_providers']=[{'name':'legacy','base_url':'https://inference.invalid/v1','api_key':'stale-key','api_mode':'responses'}]
        cfg['gateway']={'proxy_url':'https://wrong.invalid','proxy_key':'old-key'}
        cfg['platforms']={'telegram':{'enabled':False,'extra':{'allow_from':['999'],'group_allow_from':['999']}}}
        cfg['telegram']={'enabled':False,'allow_from':['999'],'allowed_chats':['-999'],'extra':{'guest_mode':True,'allow_from':['999']},'channel_overrides':{'123':{'model':'root-old','provider':'root-old','system_prompt':'personal channel'}}}
        (home/'config.yaml').write_text(yaml.safe_dump(cfg))
        (home/'auth.json').write_text(json.dumps({'credential_pool':{'custom:legacy':[{'id':'old','api_key':'stale-key'}],'unrelated':[{'api_key':'keep'}]}}))
        (home/'SOUL.md').write_text('identity')
        (home/'workspace').mkdir(); (home/'workspace/personal.txt').write_text('work')
        (home/'.env').write_text("CUSTOM_BASE_URL='https://stale.invalid/v1'\nTELEGRAM_ALLOWED_USERS='999'\nTELEGRAM_ALLOW_ALL_USERS='true'\nTELEGRAM_GUEST_MODE='true'\nTELEGRAM_ALLOWED_CHATS='-999'\nDISCORD_BOT_TOKEN='synthetic-discord'\nGATEWAY_PROXY_URL='https://wrong.invalid'\nGATEWAY_MULTIPLEX_PROFILES='true'\n")
        (home/'gateway.json').write_text(json.dumps({'platforms':{'telegram':{'token':'old-token','extra':{'allow_from':['999'],'group_allow_from':['999'],'group_allowed_chats':['-999'],'guest_mode':True},'channel_overrides':{'123':{'model':'old','provider':'old','system_prompt':'personal channel'}}}},'multiplex_profiles':True}))
        for layout in ('pairing','platforms/pairing'):
            pairing=home/layout; pairing.mkdir(parents=True,exist_ok=True)
            (pairing/'telegram-approved.json').write_text('{"999":{"user_name":"old"}}')
            (pairing/'discord-approved.json').write_text('{"other":{"user_name":"keep"}}')
        apply(home,scenario)
        raw=yaml.safe_load((home/'config.yaml').read_text())
        assert raw['agent']['system_prompt']=='Keep my personal prompt'
        assert raw['display']['personality']=='helpful-personal'
        assert raw['compression']['personal_note']=='keep'
        assert raw['onboarding_complete'] is True
        assert raw['memory']['memory_enabled'] is False
        assert (home/'SOUL.md').read_text()=='identity'
        assert (home/'workspace/personal.txt').read_text()=='work'
        # Retiring CR extra leaves must preserve sibling personal keys.
        apply(home,'retired')
        raw=yaml.safe_load((home/'config.yaml').read_text())
        assert raw['compression']=={'personal_note':'keep'}
        assert 'unknown_safe' not in raw
print('5 renderer/native resolver scenarios + retirement/pairing/personalization passed')
