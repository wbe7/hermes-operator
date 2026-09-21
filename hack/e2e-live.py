#!/usr/bin/env python3
"""Opt-in, namespace-scoped native gateway snapshot/restart verification.
Never imports SessionStore: its constructor overwrites gateway health identity.
Does not provision credentials, send messages, or start a second poller.
"""
import json
import os
from pathlib import Path
import subprocess
import time

required = ['E2E_LIVE_KUBECONFIG','E2E_LIVE_CONTEXT','E2E_LIVE_NAMESPACE','E2E_LIVE_HERMES','E2E_LIVE_DEDICATED','E2E_DIR']
for key in required:
    if not os.environ.get(key): raise SystemExit('required environment input: '+key)
assert os.environ['E2E_LIVE_DEDICATED']=='yes', 'explicit dedicated test identity assertion required'
ns=os.environ['E2E_LIVE_NAMESPACE']
assert ns.startswith('hermes-operator-test'), 'live namespace must be a dedicated hermes-operator-test namespace'
name=os.environ['E2E_LIVE_HERMES']
k=['kubectl','--kubeconfig',os.environ['E2E_LIVE_KUBECONFIG'],'--context',os.environ['E2E_LIVE_CONTEXT'],'-n',ns]
out=Path(os.environ['E2E_DIR']);out.mkdir(parents=True,exist_ok=True);out.chmod(0o700)
def call(args): return subprocess.check_output(k+args,text=True,stderr=subprocess.PIPE)
def pod(): return json.loads(call(['get','pod',name+'-hermes-0','-o','json']))
# Compare the effective native model resolver to the mounted declaration. Secret
# values are compared inside the agent and never included in the JSON result.
script=r'''
import copy,hashlib,json,sqlite3,sys
from pathlib import Path
sys.path[:0]=['/operator/runtime','/opt/hermes']
from bootstrap import load_runtime_environment
home=Path('/opt/data');load_runtime_environment(home)
from hermes_cli.config import load_config
from hermes_cli.runtime_provider import resolve_runtime_provider
c=load_config();b=json.loads(Path('/operator/config/input.json').read_text())
r=resolve_runtime_provider(requested=c['model']['provider'],target_model=c['model']['default'])
selected=b.get('env',{}).get('HERMES_MODEL_API_KEY')
if isinstance(selected,dict) and 'credential' in selected:
 assert r['api_key']==(Path('/operator/credentials')/selected['credential']).read_text()
# Read-only queries preserve gateway process identity; no SessionStore constructor.
db=sqlite3.connect('file:/opt/data/state.db?mode=ro',uri=True)
rows=[]
for sid, in db.execute('select distinct session_id from messages order by session_id'):
 history=db.execute('select role,content,tool_call_id,tool_calls,tool_name,reasoning,reasoning_content,platform_message_id from messages where session_id=? order by id',(sid,)).fetchall()
 rows.append([sid,len(history),hashlib.sha256(json.dumps(history,ensure_ascii=False).encode()).hexdigest()])
db.close()
personal=copy.deepcopy(c)
for path in b['ownedPaths']:
 parent=personal
 for part in path[:-1]:
  parent=parent.get(part,{}) if isinstance(parent,dict) else {}
 if isinstance(parent,dict):parent.pop(path[-1],None)
def compact(value):
 if isinstance(value,dict):return {k:compact(v) for k,v in value.items() if compact(v)!={}}
 return value
personal_digest=hashlib.sha256(json.dumps(compact(personal),sort_keys=True,ensure_ascii=False).encode()).hexdigest()
files={}
for folder in ['memories','skills','workspace','cron']:
 for p in (home/folder).rglob('*'):
  if p.is_file() and p.name not in ('ticker_heartbeat','ticker_last_success') and '__pycache__' not in p.parts:files[str(p.relative_to(home))]=hashlib.sha256(p.read_bytes()).hexdigest()
for filename in ['SOUL.md','USER.md']:
 p=home/filename
 if p.exists():files[filename]=hashlib.sha256(p.read_bytes()).hexdigest()
assert json.loads((home/'gateway_state.json').read_text())['pid']==1
print(json.dumps({'model':c['model']['default'],'provider':c['model']['provider'],'reasoning':c['agent']['reasoning_effort'],'files':files,'personalConfigSHA256':personal_digest,'sessions':rows,'expected':{'model':b['config']['model']['default'],'provider':b['config']['model']['provider'],'reasoning':b['config']['agent']['reasoning_effort']}}))
'''
def snapshot(): return json.loads(call(['exec',name+'-hermes-0','--','/opt/hermes/.venv/bin/python','-I','-c',script]))
before=pod();state=snapshot()
if os.environ.get('E2E_LIVE_RESTART')=='yes':
    call(['exec',name+'-hermes-0','--','/opt/hermes/.venv/bin/python','-I','-c','import os,signal;os.kill(1,signal.SIGTERM)'])
    deadline=time.monotonic()+720
    count=before['status']['containerStatuses'][0]['restartCount']
    while time.monotonic()<deadline:
        after=pod();status=after['status'].get('containerStatuses',[{}])[0]
        if status.get('restartCount',0)>count and status.get('ready'):break
        time.sleep(5)
    else:raise AssertionError('gateway did not recover within startup budget')
    assert after['metadata']['uid']==before['metadata']['uid']
    restored=snapshot()
    assert restored['files']==state['files'] and restored['sessions']==state['sessions']
    assert restored['personalConfigSHA256']==state['personalConfigSHA256']
    assert all(restored[key]==restored['expected'][key] for key in ['model','provider','reasoning'])
    result={'result':'passed','podUID':after['metadata']['uid'],'restartCountBefore':count,'restartCountAfter':status['restartCount'],'scope':'same Pod SIGTERM, native loader, read-only SQLite and personal file snapshot; no conversational model override seeded'}
else:result={'result':'passed','scope':'read-only native loader and state snapshot only','podUID':before['metadata']['uid']}
(out/'live-snapshot.json').write_text(json.dumps(result,indent=2))
(out/'live-state.json').write_text(json.dumps(state,indent=2))
print(json.dumps(result))
print('A02/A03: not-run; human DM/unauthorized identity/group checks require separate evidence')
