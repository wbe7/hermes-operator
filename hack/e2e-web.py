#!/usr/bin/env python3
"""Opt-in HTTPS/auth/assets/WebSocket check for a dedicated Hermes test instance.
Reads credentials directly into memory. Never prints cookies, credentials or config.
Does not alter the CR, PVC or DNS. A WebSocket ticket opens /api/events only;
this proves transport/auth, not a conversational model turn.
"""
import base64
import hashlib
import http.cookiejar
import json
import os
import re
import secrets
import socket
import ssl
import subprocess
import urllib.error
import urllib.parse
import urllib.request


def validate_url(url):
    p = urllib.parse.urlsplit(url)
    if p.scheme != 'https' or not p.hostname or p.username or p.password or p.query or p.fragment or p.port not in (None, 443):
        raise ValueError('expected HTTPS dashboard URL without credentials/query/fragment')
    return url.rstrip('/')


def websocket_upgrade(url, ticket):
    p = urllib.parse.urlsplit(url)
    key = base64.b64encode(secrets.token_bytes(16)).decode()
    path = p.path.rstrip('/') + '/api/events?ticket=' + urllib.parse.quote(ticket)
    with socket.create_connection((p.hostname, 443), timeout=20) as raw:
        with ssl.create_default_context().wrap_socket(raw, server_hostname=p.hostname) as conn:
            request = f'GET {path} HTTP/1.1\r\nHost: {p.hostname}\r\nOrigin: https://{p.hostname}\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: {key}\r\nSec-WebSocket-Version: 13\r\n\r\n'
            conn.sendall(request.encode())
            headers = b''
            while b'\r\n\r\n' not in headers and len(headers) < 32768:
                part = conn.recv(4096)
                if not part: break
                headers += part
            lines = headers.split(b'\r\n\r\n', 1)[0].decode().split('\r\n')
            if not lines[0].startswith('HTTP/1.1 101 '):
                return False
            expected = base64.b64encode(hashlib.sha1((key+'258EAFA5-E914-47DA-95CA-C5AB0DC85B11').encode()).digest()).decode()
            fields = {k.lower(): v.strip() for k,v in (line.split(':',1) for line in lines[1:] if ':' in line)}
            if fields.get('sec-websocket-accept') != expected:
                raise AssertionError('invalid WebSocket upgrade')
            # Client close frame, masked as required by RFC6455.
            conn.sendall(b'\x88\x80' + secrets.token_bytes(4))
            return True


def main():
    ns = os.environ['E2E_WEB_NAMESPACE']
    if not ns.startswith('hermes-operator-test') or os.environ.get('E2E_WEB_DEDICATED') != 'yes':
        raise ValueError('dedicated test namespace assertion required')
    name = os.environ['E2E_WEB_HERMES']
    k = ['kubectl', '--kubeconfig', os.environ['E2E_WEB_KUBECONFIG'], '--context', os.environ['E2E_WEB_CONTEXT'], '-n', ns]
    def get(kind, name):
        return json.loads(subprocess.check_output(k+['get',kind,name,'-o','json'], stderr=subprocess.PIPE))
    h = get('hermes', name)
    url = validate_url(h['status']['web']['url'])
    secret = get('secret', h['spec'].get('credentials',{}).get('secretName') or name+'-hermes-secret')
    password = base64.b64decode(secret['data']['WEB_PASSWORD']).decode()
    username = h['spec']['web'].get('auth',{}).get('username') or 'admin'
    jar = http.cookiejar.CookieJar()
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
    def request(path, data=None, headers=None):
        req = urllib.request.Request(url+path, data=None if data is None else json.dumps(data).encode(), headers={'Content-Type':'application/json', **(headers or {})})
        try:
            with opener.open(req, timeout=30) as response:
                return response.status, response.read(4*1024*1024), response.headers
        except urllib.error.HTTPError as error:
            return error.code, error.read(), error.headers
    status, body, _ = request('/api/status')
    data = json.loads(body)
    assert status == 200 and data.get('auth_required') is True and 'basic' in data.get('auth_providers',[]), 'auth guard missing'
    assert request('/api/config')[0] == 401, 'anonymous config permitted'
    assert not websocket_upgrade(url, 'invalid-smoke-ticket'), 'anonymous WS permitted'
    assert request('/auth/password-login', {'provider':'basic','username':username,'password':secrets.token_urlsafe(32)})[0] == 401, 'bad password accepted'
    assert request('/auth/password-login', {'provider':'basic','username':username,'password':password})[0] == 200, 'login failed'
    prefix = urllib.parse.urlsplit(url).path or '/'
    assert list(jar) and all(c.secure and c.path.rstrip('/') == prefix.rstrip('/') for c in jar), 'cookie HTTPS/path mismatch'
    assert request('/api/config')[0] == 200, 'authenticated API failed'
    status, body, _ = request('/')
    assert status == 200, 'dashboard HTML failed'
    assets = re.findall(r'(?:src|href)="([^"]*assets/[^\"]+)"', body.decode())
    assert assets, 'built frontend assets missing'
    for asset in assets[:4]:
        path = urllib.parse.urlsplit(asset).path
        assert path.startswith((prefix.rstrip('/') or '')+'/assets/'), 'asset prefix mismatch'
        with opener.open(urllib.parse.urljoin(url+'/', asset), timeout=30) as response:
            assert response.status == 200 and 'text/html' not in response.headers.get('Content-Type',''), 'asset route failed'
    status, body, _ = request('/api/auth/ws-ticket', {})
    assert status == 200, 'WS ticket failed'
    ticket = json.loads(body)['ticket']
    assert websocket_upgrade(url, ticket), 'authenticated WS failed'
    assert not websocket_upgrade(url, ticket), 'single-use WS ticket reused'
    print(json.dumps({'result':'passed','url':url,'checks':['TLS','native guard','anonymous rejection','wrong password','login','Secure/path cookies','authenticated API','frontend assets','WebSocket upgrade','ticket replay rejection']}))


if __name__ == '__main__':
    main()
