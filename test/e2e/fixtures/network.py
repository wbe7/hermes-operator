import json,pathlib,subprocess,os
p=pathlib.Path(os.environ['E2E_DIR']);k=['kubectl','--kubeconfig',os.environ['KUBECONFIG'],'--context',os.environ['E2E_CONTEXT']]
def read(name):return json.loads(p.joinpath(name+'.json').read_text())
def check(stage):
 subprocess.run(['python3',str(pathlib.Path(__file__).with_name('probe-network.py')),stage],stdout=subprocess.DEVNULL,check=True)
 return read(stage)['results']
def apply(name):subprocess.run(k+['apply','-f',str(p/name)],check=True,stdout=subprocess.DEVNULL)
from acceptance import evidence, get
subprocess.run(['python3',str(pathlib.Path(__file__).with_name('prepare-network.py'))],check=True)
subprocess.run(k+['-n','hermes-operator-test','wait','--for=condition=Ready','pod','--all','--timeout=240s'],check=True)
network={'enforcementConfirmed':True,'podCIDRs':['10.244.0.0/16','fd00:10:244::/56'],'serviceCIDRs':['10.96.0.0/16','fd00:10:96::/112'],'nodeCIDRs':read('values')['networkPolicy']['nodeCIDRs'],'infrastructureCIDRs':[],'dns':{'podSelector':{'namespaceLabels':{'kubernetes.io/metadata.name':'kube-system'},'podLabels':{'k8s-app':'kube-dns'}},'resolverIPs':get('service','kube-dns','kube-system')['spec']['clusterIPs']}}
evidence('network-config',network)
env=dict(os.environ,GOTOOLCHAIN='go1.27.1',GOMODCACHE='/tmp/hermes-go-mod',GOCACHE='/tmp/hermes-go-cache')
subprocess.run(['go','run','./test/e2e/policy',str(p/'hermes-input.json'),str(p/'network-config.json'),str(p/'policy.json')],env=env,check=True)
b=check('baseline');private=['target8080'+str(f) for f in [4,6]]+['target8081'+str(f) for f in [4,6]]+['neighbor8080'+str(f) for f in [4,6]]+['service'+str(f) for f in [4,6]]+['hostingNodeAPI'+str(f) for f in [4,6]]
retained=['dnsTCP4','dnsTCP6','dnsUDP4','dnsUDP6','publicTCP4','publicUDP4']
for row in b.values():assert all(row[x] is True for x in private+retained)
print('Baseline: native IPv4/IPv6 private endpoints and DNS reachable; public IPv4 reachable',flush=True)
public6=all(row[x] is True for row in b.values() for x in ['publicTCP6','publicUDP6'])
if public6: retained += ['publicTCP6','publicUDP6']
print('External public IPv6 baseline: '+str(public6),flush=True)
apply('policy.json')
try:
 r=check('isolated')
 assert all(r['network-client'][x] is True for x in private+retained)
 assert all(r['network-isolated'][x] is not True for x in private)
 assert all(r['network-isolated'][x] is True for x in retained)
 print('PASS: both families deny Pod/Service/hosting-node; DNS4/6 TCP+UDP and public4 remain open',flush=True)
 h=read('hermes-input')
 target=json.loads(subprocess.check_output(k+['-n','hermes-operator-test','get','pod','network-target','-o','json']))
 h['spec']['network']={'allowPrivate':[{'ip':i['ip'],'ports':[{'port':8080,'protocol':'TCP'}]} for i in target['status']['podIPs']]}
 p.joinpath('hermes-allow.json').write_text(json.dumps(h))
 env=dict(os.environ,GOTOOLCHAIN='go1.27.1',GOMODCACHE='/tmp/hermes-go-mod',GOCACHE='/tmp/hermes-go-cache')
 subprocess.run(['go','run','./test/e2e/policy',str(p/'hermes-allow.json'),str(p/'network-config.json'),str(p/'policy-allow.json')],env=env,check=True,stdout=subprocess.DEVNULL)
 apply('policy-allow.json');r=check('exception')
 assert all(r['network-client'][x] is True for x in private+retained)
 opened=['target80804','target80806','service4','service6'];blocked=[x for x in private if x not in opened]
 assert all(r['network-isolated'][x] is True for x in opened+retained)
 assert all(r['network-isolated'][x] is not True for x in blocked)
 print('PASS: exact IPv4/IPv6 backend:8080 + its DNAT Service open; other port/neighbor/node denied',flush=True)
finally:
 apply('policy.json');print('Original compiled policy restored',flush=True)
r=check('restored');assert all(r['network-client'][x] is True for x in private+retained)
assert all(r['network-isolated'][x] is True for x in retained)
assert all(r['network-isolated'][x] is not True for x in private)
print('PASS: restored denial for both families',flush=True)
p.joinpath('verdict.json').write_text(json.dumps({'privateIPv4':'PASS','privateIPv6':'PASS','dnsTCPUDPv4v6':'PASS','exactExceptionsIPv4IPv6':'PASS','publicIPv4':'PASS','publicIPv6':'PASS' if public6 else 'not-run: unavailable in unrestricted baseline','restoration':'PASS'},indent=2))
