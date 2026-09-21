"""Exact metadata-address fixture inside an owned disposable kind node only."""
from contextlib import contextmanager
import ipaddress
import json
import os
from pathlib import Path
import subprocess


def command(args):
    return subprocess.check_output(args, text=True, stderr=subprocess.PIPE, timeout=30)


def validate_node(node):
    if node.get('Name') != '/hermes-operator-e2e-control-plane':
        raise ValueError('unexpected node container name')
    labels = node.get('Config', {}).get('Labels', {})
    if labels.get('io.x-k8s.kind.cluster') != 'hermes-operator-e2e' or labels.get('io.x-k8s.kind.role') != 'control-plane':
        raise ValueError('not the owned kind control-plane')
    if node.get('HostConfig', {}).get('NetworkMode') != 'kind':
        raise ValueError('unexpected network namespace; host networking is forbidden')
    identifier = node.get('Id', '')
    if len(identifier) != 64 or any(c not in '0123456789abcdef' for c in identifier):
        raise ValueError('invalid exact container ID')
    return identifier


@contextmanager
def metadata_route(get, evidence):
    if os.environ.get('E2E_CONTEXT') != 'kind-hermes-operator-e2e':
        raise ValueError('metadata fixture requires owned local kind context')
    node = json.loads(command(['docker','inspect','hermes-operator-e2e-control-plane']))[0]
    identifier = validate_node(node)
    target = get('pod','network-neighbor','hermes-operator-test')
    backend = next(x['ip'] for x in target['status']['podIPs'] if ipaddress.ip_address(x['ip']).version == 4)
    installed = []
    details = {'containerID':identifier, 'destination':'169.254.169.254:18080',
               'backend':backend+':8081','sources':[],'installed':False,
               'scope':'owned kind node PREROUTING DNAT, not host or cloud metadata'}
    base = ['docker','exec',identifier,'iptables','-w','-t','nat']
    try:
        for name in ('network-client','network-isolated'):
            pod = get('pod',name,'hermes-operator-test')
            if pod['spec']['nodeName'] != 'hermes-operator-e2e-control-plane':
                raise ValueError('fixture client is not on the owned node')
            source = next(x['ip'] for x in pod['status']['podIPs'] if ipaddress.ip_address(x['ip']).version == 4)
            rule = ['-s',source+'/32','-d','169.254.169.254/32','-p','tcp','--dport','18080',
                    '-m','comment','--comment','hermes-e2e-metadata','-j','DNAT','--to-destination',backend+':8081']
            command(base+['-I','PREROUTING','1']+rule)
            installed.append(rule)
            command(base+['-C','PREROUTING']+rule)
            details['sources'].append(source)
        details['installed'] = True
        evidence('metadata-fixture',details)
        yield
    finally:
        failures=[]
        for rule in reversed(installed):
            try: command(base+['-D','PREROUTING']+rule)
            except subprocess.SubprocessError: failures.append('exact rule removal failed')
        details['installed'] = False
        details['cleanup'] = 'failed' if failures else 'passed'
        evidence('metadata-fixture',details)
        if failures: raise RuntimeError('metadata fixture cleanup failed; cluster deletion is required')
