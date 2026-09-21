"""Bounded local acceptance. Synthetic fixtures never count as native gateway readiness."""
import ipaddress
import json
import os
from pathlib import Path
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[3]
OUT = Path(os.environ['E2E_DIR'])
CONTEXT = os.environ['E2E_CONTEXT']
assert CONTEXT == 'kind-hermes-operator-e2e', 'local mutations require the dedicated context'
K = ['kubectl', '--kubeconfig', os.environ['KUBECONFIG'], '--context', CONTEXT]
NS = 'hermes-e2e-a'
IMAGE = 'python@sha256:c4634f578a412db396771b61b064c6e546c9d6414c7fb5b1b05d5871f1885f7b'

def run(args, data=None):
    return subprocess.check_output(args, input=None if data is None else json.dumps(data), text=True, stderr=subprocess.PIPE)
def k(*args, data=None): return run(K + list(args), data)
def get(kind, name, ns=NS): return json.loads(k('-n', ns, 'get', kind, name, '-o', 'json'))
def apply(obj): return k('apply', '-f', '-', data=obj)
def wait(check, message, seconds=180):
    end = time.monotonic() + seconds
    while time.monotonic() < end:
        try:
            value = check()
            if value: return value
        except subprocess.CalledProcessError: pass
        time.sleep(2)
    raise AssertionError(message)
def patch(kind, name, spec, ns=NS):
    return k('-n', ns, 'patch', kind, name, '--type=merge', '-p', json.dumps({'spec': spec}))
def uid(kind, name, ns=NS): return get(kind, name, ns)['metadata']['uid']
def evidence(name, value): (OUT / (name + '.json')).write_text(json.dumps(value, indent=2))
def helm(verb):
    return run(['helm', '--kubeconfig', os.environ['KUBECONFIG'], '--kube-context', CONTEXT, verb,
                '--install', 'hermes-operator', str(ROOT/'charts/hermes-operator'), '-n', 'hermes-system', '--create-namespace',
                '-f', str(ROOT/'test/e2e/fixtures/operator-values.yaml'), '-f', str(OUT/'values.json'), '--wait', '--timeout', '3m'])
def install():
    nodes=json.loads(k('get','nodes','-o','json'))['items']
    networks=sorted({str(ipaddress.ip_network(a['address']+('/32' if ':' not in a['address'] else '/128'),strict=False)) for n in nodes for a in n['status']['addresses'] if a['type']=='InternalIP'})
    evidence('values', {'networkPolicy': {'nodeCIDRs': networks}})
    k('apply','--server-side','-f',str(ROOT/'config/crd/bases/hermes.wbe7.github.io_hermes.yaml'))
    k('wait','--for=condition=Established','crd/hermes.hermes.wbe7.github.io','--timeout=60s')
    helm('upgrade')
    evidence('environment',{'nodes':[{'version':n['status']['nodeInfo']['kubeletVersion'],'architecture':n['status']['nodeInfo']['architecture']} for n in nodes]})
def cr(name, ns=NS, retention='Retain', existing=None):
    storage={'existingClaim':existing} if existing else {'create':{'size':'1Gi'},'deletionPolicy':retention}
    return {'apiVersion':'hermes.wbe7.github.io/v1alpha1','kind':'Hermes','metadata':{'name':name,'namespace':ns},'spec':{'version':'v2026.9.14','model':{'provider':'custom','name':'fixture','baseURL':'http://192.0.2.1:8080/v1','auth':'None'},'telegram':{'allowedUserIDs':['123456789']},'storage':storage,'scheduling':{'nodeSelector':{'hermes-e2e-unschedulable':'true'}}}}
def secret(name, ns):
    apply({'apiVersion':'v1','kind':'Secret','metadata':{'name':name+'-hermes-secret','namespace':ns},'stringData':{'TELEGRAM_BOT_TOKEN':'000000:synthetic-offline-token'}})
def lifecycle():
    result={}
    for ns in [NS,'hermes-e2e-b']:
        apply({'apiVersion':'v1','kind':'Namespace','metadata':{'name':ns,'labels':{'pod-security.kubernetes.io/enforce':'restricted'}}})
        apply(cr('sample',ns))
        wait(lambda: any(c['type']=='Ready' and c['status']=='False' for c in get('hermes','sample',ns).get('status',{}).get('conditions',[])), 'missing Secret must be NotReady')
        secret('sample',ns)
        wait(lambda: get('statefulset','sample-hermes',ns), 'controller must create workload')
        wait(lambda: get('pod','sample-hermes-0',ns), 'actual pending Pod must exist')
        result[ns]={'crUID':uid('hermes','sample',ns),'podUID':uid('pod','sample-hermes-0',ns),'pvcUID':uid('pvc','sample-hermes-data',ns)}
    assert result[NS]['pvcUID'] != result['hermes-e2e-b']['pvcUID']
    patch('hermes','sample',{'suspend':True})
    k('-n',NS,'wait','--for=delete','pod/sample-hermes-0','--timeout=120s')
    patch('hermes','sample',{'suspend':False})
    wait(lambda:get('pod','sample-hermes-0'), 'resume Pod')
    assert uid('pod','sample-hermes-0') != result[NS]['podUID']
    assert uid('pvc','sample-hermes-data') == result[NS]['pvcUID']
    k('-n',NS,'delete','secret','sample-hermes-secret')
    wait(lambda:get('statefulset','sample-hermes')['spec']['replicas']==0,'Secret revocation scales to zero')
    secret('sample',NS)
    wait(lambda:get('statefulset','sample-hermes')['spec']['replicas']==1,'Secret recovery resumes')
    old=get('hermes','sample')['status']['appliedRevision']
    patch('hermes','sample',{'model':{'name':'updated-fixture'}})
    wait(lambda:get('hermes','sample')['status'].get('appliedRevision')!=old,'CR revision changes')
    old=get('hermes','sample')['status']['appliedRevision']
    apply({'apiVersion':'v1','kind':'Secret','metadata':{'name':'sample-hermes-secret','namespace':NS},'stringData':{'TELEGRAM_BOT_TOKEN':'111111:rotated-synthetic-offline-token'}})
    wait(lambda:get('hermes','sample')['status'].get('appliedRevision')!=old,'used Secret rotation changes revision')
    patch('hermes','sample',{'version':'unsupported-e2e'})
    wait(lambda:any(c.get('reason')=='UnsupportedVersion' for c in get('hermes','sample')['status']['conditions']),'unsupported version diagnostic')
    k('-n',NS,'delete','networkpolicy','sample-hermes')
    wait(lambda:get('statefulset','sample-hermes')['spec']['replicas']==0,'invalid CR must not retain workload without policy')
    k('-n',NS,'wait','--for=delete','pod/sample-hermes-0','--timeout=120s')
    assert uid('pvc','sample-hermes-data') == result[NS]['pvcUID']
    patch('hermes','sample',{'version':'v2026.9.14'})
    wait(lambda:get('statefulset','sample-hermes')['spec']['replicas']==1 and get('networkpolicy','sample-hermes'),'valid CR restores policy and workload')
    def current_pod():
        pod=get('pod','sample-hermes-0')
        revision=get('hermes','sample')['status']['appliedRevision']
        return pod if not pod['metadata'].get('deletionTimestamp') and pod['metadata'].get('annotations',{}).get('hermes.wbe7.github.io/revision')==revision else None
    pod=wait(current_pod,'current Pod before direct image drift')
    # UID test avoids modifying a replacement if reconciliation races this patch.
    k('-n',NS,'patch','pod','sample-hermes-0','--type=json','-p',json.dumps([
        {'op':'test','path':'/metadata/uid','value':pod['metadata']['uid']},
        {'op':'replace','path':'/spec/containers/0/image','value':'invalid.invalid/hermes-review:never'}]))
    wait(lambda:uid('pod','sample-hermes-0')!=pod['metadata']['uid'] and current_pod(),'direct Pod image drift must be replaced')
    assert get('pod','sample-hermes-0')['spec']['containers'][0]['image']==get('statefulset','sample-hermes')['spec']['template']['spec']['containers'][0]['image']
    previous=get('hermes','sample')['status']['appliedRevision']
    patch('statefulset','sample-hermes',{'updateStrategy':{'type':'RollingUpdate','rollingUpdate':{'partition':1}}})
    patch('hermes','sample',{'model':{'name':'after-strategy-drift'}})
    wait(lambda:get('statefulset','sample-hermes')['spec']['updateStrategy'].get('rollingUpdate',{}).get('partition',0)==0,'partition must be reset')
    wait(lambda:get('hermes','sample')['status']['appliedRevision']!=previous and current_pod(),'partition drift must not trap rollout on old revision')
    k('-n','hermes-system','rollout','restart','deployment/hermes-operator')
    k('-n','hermes-system','rollout','status','deployment/hermes-operator','--timeout=180s')
    # Explicit CRD upgrade always precedes the real Helm upgrade.
    k('apply','--server-side','-f',str(ROOT/'config/crd/bases/hermes.wbe7.github.io_hermes.yaml'))
    helm('upgrade')
    assert uid('pvc','sample-hermes-data') == result[NS]['pvcUID']
    assert uid('hermes','sample') == result[NS]['crUID']
    evidence('lifecycle',{'result':'passed','scope':'real controller, Pending workload; no native gateway Ready claim','identities':result})

def consumer(claim):
    apply({'apiVersion':'v1','kind':'Pod','metadata':{'name':'volume-check','namespace':NS},'spec':{'automountServiceAccountToken':False,'securityContext':{'runAsNonRoot':True,'runAsUser':10000,'runAsGroup':10000,'fsGroup':10000,'seccompProfile':{'type':'RuntimeDefault'}},'containers':[{'name':'fixture','image':IMAGE,'command':['python','-c','import time,signal,sys;signal.signal(signal.SIGTERM,lambda *a:sys.exit(0));time.sleep(1800)'],'securityContext':{'allowPrivilegeEscalation':False,'capabilities':{'drop':['ALL']}},'volumeMounts':[{'name':'home','mountPath':'/data'}]}],'volumes':[{'name':'home','persistentVolumeClaim':{'claimName':claim}}]}})
    k('-n',NS,'wait','--for=condition=Ready','pod/volume-check','--timeout=240s')
def persistence():
    claim='sample-hermes-data'; original=uid('pvc',claim)
    patch('hermes','sample',{'suspend':True})
    k('-n',NS,'wait','--for=delete','pod/sample-hermes-0','--timeout=120s')
    consumer(claim)
    k('-n',NS,'exec','volume-check','--','python','-c',"from pathlib import Path;Path('/data/e2e-marker').write_text('persisted')")
    k('-n',NS,'delete','pod','volume-check','--wait=true')
    k('-n',NS,'delete','hermes','sample','--wait=true','--timeout=120s')
    assert uid('pvc',claim)==original
    apply(cr('sample'))
    wait(lambda:any(c.get('reason')=='StorageIdentityMismatch' for c in get('hermes','sample').get('status',{}).get('conditions',[])), 'same name must not adopt retained data')
    k('-n',NS,'delete','hermes','sample','--wait=true','--timeout=120s')
    secret('adopted',NS);apply(cr('adopted',existing=claim))
    wait(lambda:get('hermes','adopted').get('status',{}).get('storageRef',{}), 'existing claim status')
    consumer(claim)
    k('-n',NS,'exec','volume-check','--','python','-c',"from pathlib import Path;assert Path('/data/e2e-marker').read_text()=='persisted'")
    k('-n',NS,'delete','pod','volume-check','--wait=true')
    k('-n',NS,'delete','hermes','adopted','--wait=true','--timeout=120s')
    assert uid('pvc',claim)==original
    secret('delete-me',NS);apply(cr('delete-me',retention='Delete'))
    wait(lambda:get('pvc','delete-me-hermes-data'),'Delete fixture PVC')
    k('-n',NS,'delete','hermes','delete-me','--wait=true','--timeout=120s')
    k('-n',NS,'wait','--for=delete','pvc/delete-me-hermes-data','--timeout=120s')
    # Clean up active CRs before uninstall; Helm does not own retained claims.
    k('-n','hermes-e2e-b','delete','hermes','sample','--wait=true','--timeout=120s')
    run(['helm','--kubeconfig',os.environ['KUBECONFIG'],'--kube-context',CONTEXT,'uninstall','hermes-operator','-n','hermes-system','--wait'])
    assert uid('pvc',claim)==original
    evidence('storage',{'result':'passed','claimUID':original,'scope':'actual PVC marker across consumer replacement, Retain, existingClaim, Delete and Helm uninstall; native Hermes state tested separately'})

if __name__=='__main__':
    try: globals()[sys.argv[1]]()
    except subprocess.CalledProcessError as exc:
        # Local-only fixtures contain no real credentials. Do not dump API objects.
        print(exc.stderr, file=sys.stderr);raise
