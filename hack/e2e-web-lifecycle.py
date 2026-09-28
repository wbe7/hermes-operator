#!/usr/bin/env python3
"""Opt-in destructive test operations on a named disposable test installation.
Restart preserves PVC; rotation revokes existing Web sessions; toggle requires Web-only.
Credentials stay in memory. Read-only HTTP checks are in e2e-web.py.
"""
import os,json,urllib.request,urllib.error,http.cookiejar,subprocess,base64,time,secrets,sys
required=['E2E_WEB_KUBECONFIG','E2E_WEB_CONTEXT','E2E_WEB_NAMESPACE','E2E_WEB_HERMES']
for key in required:
 if not os.environ.get(key):raise SystemExit('required environment input: '+key)
ns=os.environ['E2E_WEB_NAMESPACE']
assert ns.startswith('hermes-operator-test') and os.environ.get('E2E_WEB_DEDICATED')=='yes', 'dedicated test identity required'
assert len(set(sys.argv[1:]) & {'--restart','--rotate','--toggle'})==1 and len(sys.argv)==2, 'choose exactly one explicit lifecycle action'
k=['kubectl','--kubeconfig',os.environ['E2E_WEB_KUBECONFIG'],'--context',os.environ['E2E_WEB_CONTEXT'],'-n',ns]
name=os.environ['E2E_WEB_HERMES']
def get(kind,name):return json.loads(subprocess.check_output(k+['get',kind,name,'-o','json']))
def call(args,data=None):return subprocess.run(k+args,input=None if data is None else json.dumps(data).encode(),check=True,stdout=subprocess.DEVNULL)
def ready(old_uid=None):
 until=time.monotonic()+240
 while time.monotonic()<until:
  h=get('hermes',name)
  cond={c['type']:c for c in h['status'].get('conditions',[])}
  if cond.get('Ready',{}).get('status')=='True' and cond['Ready'].get('observedGeneration')==h['metadata']['generation']:
   p=get('pod',name+'-hermes-0')
   if old_uid is None or p['metadata']['uid']!=old_uid:return
  time.sleep(3)
 raise AssertionError('rollout timeout')
h=get('hermes',name);url=h['status']['web']['url'];secretname=h['spec'].get('credentials',{}).get('secretName') or name+'-hermes-secret';s=get('secret',secretname);original=s['data'].copy()
jar=http.cookiejar.CookieJar();o=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
def req(path,data=None):
 r=urllib.request.Request(url+path,data=None if data is None else json.dumps(data).encode(),headers={'Content-Type':'application/json'})
 try:
  with o.open(r,timeout=20)as f:return f.status,f.read()
 except urllib.error.HTTPError as e:return e.code,e.read()
def login(data):return req('/auth/password-login',{'provider':'basic','username':h['spec']['web'].get('auth',{}).get('username') or 'admin','password':base64.b64decode(data['WEB_PASSWORD']).decode()})[0]
assert login(original)==200
uid=get('pod',name+'-hermes-0')['metadata']['uid']
if '--restart' in sys.argv:
 call(['delete','pod',name+'-hermes-0','--wait=false']);ready(uid)
 assert get('secret',secretname)['data']==original
 assert req('/api/config')[0]==200,'session lost after restart'
 print('restart: stable keys and native login session survived')
if '--rotate' in sys.argv:
 s=get('secret',secretname)
 for key in ('WEB_PASSWORD','WEB_SESSION_SECRET'):s['data'][key]=base64.b64encode(secrets.token_urlsafe(32).encode()).decode()
 call(['replace','-f','-'],s);ready(uid)
 assert req('/api/config')[0]==401,'old session survived signing key rotation'
 assert login(original)==401,'old password retained'
 assert login(s['data'])==200,'new password failed'
 print('rotation: old session/password rejected; new login works')
if '--toggle' in sys.argv:
 assert not h['spec'].get('telegram'), 'toggle test requires Web-only initial spec'
 call(['patch','hermes',name,'--type','merge','-p',json.dumps({'spec':{'web':{'enabled':False}}})]);ready(uid)
 assert subprocess.run(k+['get','httproute',name+'-hermes-web'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL).returncode!=0
 assert get('secret',secretname)['data']==original
 state=json.loads(subprocess.check_output(k+['exec',name+'-hermes-0','--','/opt/hermes/.venv/bin/python','-I','-c','import json;print(json.dumps(json.load(open("/opt/data/gateway_state.json"))))']))
 assert state['gateway_state']=='running'
 assert not state.get('platforms',{}).get('telegram')
 print('disabled: route removed; zero-channel gateway ready; keys preserved')
 uid=get('pod',name+'-hermes-0')['metadata']['uid'];call(['patch','hermes',name,'--type','merge','-p',json.dumps({'spec':{'web':{'enabled':True}}})]);ready(uid)
 assert get('secret',secretname)['data']==original
 assert req('/api/config')[0]==200
 print('reenabled: same credentials/session work')
