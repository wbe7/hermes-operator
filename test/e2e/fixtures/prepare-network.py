import os
assert os.environ.get('E2E_CONTEXT') == 'kind-hermes-operator-e2e'
import json,pathlib,subprocess
p=pathlib.Path(os.environ['E2E_DIR']);k=['kubectl','--kubeconfig',os.environ['KUBECONFIG'],'--context',os.environ['E2E_CONTEXT']];ns='hermes-operator-test'
def apply(o):subprocess.run(k+['apply','-f','-'],input=json.dumps(o),text=True,check=True)
apply({'apiVersion':'v1','kind':'Namespace','metadata':{'name':ns,'labels':{'pod-security.kubernetes.io/enforce':'restricted','pod-security.kubernetes.io/enforce-version':'v1.36'}}})
subprocess.run(k+['apply','--server-side','-f','config/crd/bases/hermes.wbe7.github.io_hermes.yaml'],check=True)
subprocess.run(k+['wait','--for=condition=Established','crd/hermes.hermes.wbe7.github.io','--timeout=30s'],check=True)
apply({'apiVersion':'hermes.wbe7.github.io/v1alpha1','kind':'Hermes','metadata':{'name':'network-smoke','namespace':ns},'spec':{'version':'v2026.9.14','model':{'provider':'custom','name':'fixture','baseURL':'http://192.0.2.1:8000/v1','auth':'None'},'telegram':{'allowedUserIDs':['123456789']},'storage':{'create':{'size':'1Gi'}},'suspend':True}})
h=json.loads(subprocess.check_output(k+['-n',ns,'get','hermes','network-smoke','-o','json']));p.joinpath('hermes-input.json').write_text(json.dumps(h))
server='''import http.server,socket,threading,time
class Server(http.server.ThreadingHTTPServer):
 address_family=socket.AF_INET6
 def server_bind(self):
  self.socket.setsockopt(socket.IPPROTO_IPV6,socket.IPV6_V6ONLY,0)
  super().server_bind()
class Handler(http.server.BaseHTTPRequestHandler):
 def do_GET(self):
  self.send_response(200);self.end_headers();self.wfile.write(b'ok')
 def log_message(self,*args):pass
for port in (8080,8081):
 s=Server(('::',port),Handler);threading.Thread(target=s.serve_forever,daemon=True).start()
time.sleep(7200)
'''
for name in ['network-client','network-isolated','network-target','network-neighbor']:
 isclient=name in ['network-client','network-isolated'];labels={'app':name}
 if name=='network-isolated':labels['hermes.wbe7.github.io/installation-uid']=h['metadata']['uid']
 c={'name':'fixture','image':'python@sha256:c4634f578a412db396771b61b064c6e546c9d6414c7fb5b1b05d5871f1885f7b','command':['python','-I','-u','-c','import time;time.sleep(7200)' if isclient else server],'securityContext':{'allowPrivilegeEscalation':False,'readOnlyRootFilesystem':True,'capabilities':{'drop':['ALL']}},'resources':{'requests':{'cpu':'10m','memory':'32Mi'},'limits':{'cpu':'100m','memory':'128Mi'}}}
 if not isclient:c['readinessProbe']={'tcpSocket':{'port':8080},'periodSeconds':2}
 spec={'automountServiceAccountToken':False,'restartPolicy':'Never','securityContext':{'runAsNonRoot':True,'runAsUser':10000,'runAsGroup':10000,'seccompProfile':{'type':'RuntimeDefault'}},'nodeSelector':{'kubernetes.io/hostname':'hermes-operator-e2e-'+('control-plane' if isclient else 'worker')},'tolerations':[{'key':'node-role.kubernetes.io/control-plane','operator':'Exists','effect':'NoSchedule'}],'containers':[c]}
 apply({'apiVersion':'v1','kind':'Pod','metadata':{'name':name,'namespace':ns,'labels':labels},'spec':spec})
subprocess.run(k+['apply','-f',str(pathlib.Path(__file__).with_name('network-targets.yaml'))],check=True)
subprocess.run(k+['-n','kube-system','patch','service','kube-dns','--type=merge','-p',json.dumps({'spec':{'ipFamilyPolicy':'RequireDualStack'}})],check=True)
