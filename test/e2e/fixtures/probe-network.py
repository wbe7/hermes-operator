import os
import json,subprocess,pathlib,sys,ipaddress
p=pathlib.Path(os.environ['E2E_DIR']);k=['kubectl','--kubeconfig',os.environ['KUBECONFIG'],'--context',os.environ['E2E_CONTEXT']];ns='hermes-operator-test'
def get(kind,name,namespace=ns):return json.loads(subprocess.check_output(k+['-n',namespace,'get',kind,name,'-o','json']))
def byfamily(ips):return {ipaddress.ip_address(x).version:x for x in ips}
target=byfamily([x['ip'] for x in get('pod','network-target')['status']['podIPs']]);neighbor=byfamily([x['ip'] for x in get('pod','network-neighbor')['status']['podIPs']]);service=byfamily(get('service','network-target')['spec']['clusterIPs']);dns=byfamily(get('service','kube-dns','kube-system')['spec']['clusterIPs'])
node=get('node',get('pod','network-client')['spec']['nodeName']);nodeips=byfamily([x['address'] for x in node['status']['addresses'] if x['type']=='InternalIP'])
assert os.environ['E2E_CONTEXT']=='kind-hermes-operator-e2e'
standin=json.loads((p/'metadata-fixture.json').read_text())
assert standin['installed'] is True and standin['destination']=='169.254.169.254:18080'
checks=[['metadataStandin','169.254.169.254',18080]];udp=[]
for family in [4,6]:
 for name,host,port in [('target8080',target[family],8080),('target8081',target[family],8081),('neighbor8080',neighbor[family],8080),('service',service[family],8080),('dnsTCP',dns[family],53)]:checks.append([name+str(family),host,port])
 udp.append(['dnsUDP'+str(family),dns[family]])
 if family in nodeips:checks.append(['hostingNodeAPI'+str(family),nodeips[family],6443])
checks.extend([['publicTCP4','1.1.1.1',443],['publicTCP6','2606:4700:4700::1111',443]]);udp.extend([['publicUDP4','1.1.1.1'],['publicUDP6','2606:4700:4700::1111']])
script='''import socket,json,struct
out={}
for name,host,port in CHECKS:
 try:
  with socket.create_connection((host,port),timeout=2):pass
  out[name]=True
 except OSError as e:out[name]={'error':type(e).__name__,'errno':e.errno}
q=struct.pack('!HHHHHH',42,0x100,1,0,0,0)+b'\\x07example\\x03com\\x00'+struct.pack('!HH',1,1)
for name,host in UDPS:
 try:
  with socket.socket(socket.AF_INET6 if ':' in host else socket.AF_INET,socket.SOCK_DGRAM) as s:
   s.settimeout(2);s.sendto(q,(host,53));data,_=s.recvfrom(512);out[name]=len(data)>12 and data[:2]==q[:2]
 except OSError as e:out[name]={'error':type(e).__name__,'errno':e.errno}
print(json.dumps(out))
'''.replace('CHECKS',repr(checks)).replace('UDPS',repr(udp))
results={}
for name in ['network-client','network-isolated']:
 r=subprocess.run(k+['-n',ns,'exec',name,'--','python','-I','-c',script],capture_output=True,text=True,timeout=80)
 if r.returncode:raise RuntimeError(r.stderr)
 results[name]=json.loads(r.stdout)
result={'endpoints':checks,'udp':udp,'results':results};p.joinpath(sys.argv[1]+'.json').write_text(json.dumps(result,indent=2));print(json.dumps(result),flush=True)
