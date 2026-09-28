"""Supervise native Hermes processes; never create a second messaging gateway."""
import json
import os
from pathlib import Path
import signal
import subprocess
import time

PROCESS_FILE = Path('/tmp/hermes-operator-processes.json')


def channels(bundle):
    value = bundle.get('channels', {'telegram': True, 'web': False})
    if not isinstance(value, dict) or set(value) != {'telegram', 'web'} or any(type(v) is not bool for v in value.values()):
        raise ValueError('invalid declared channels')
    return value


def supervise(commands, marker=PROCESS_FILE, startup_fd=None, grace=45):
    import psutil
    children = {}
    stopping = False
    handlers = {}

    def stop(signum, frame):
        nonlocal stopping
        stopping = True

    try:
        for sig in (signal.SIGTERM, signal.SIGINT):
            handlers[sig] = signal.signal(sig, stop)
        for name, command in commands.items():
            if stopping:
                return 0
            fds = (startup_fd,) if startup_fd is not None and name == 'gateway' else ()
            children[name] = subprocess.Popen(command, start_new_session=True, pass_fds=fds)
        data = {name: {'pid': p.pid, 'started': psutil.Process(p.pid).create_time()} for name, p in children.items()}
        tmp = marker.with_suffix('.tmp')
        tmp.write_text(json.dumps(data));os.chmod(tmp,0o600);tmp.replace(marker)
        while not stopping:
            if any(p.poll() is not None for p in children.values()):
                return 1
            time.sleep(.2)
        return 0
    except Exception:
        # Commands/environments may carry credentials: never interpolate them.
        return 1
    finally:
        marker.unlink(missing_ok=True)
        descendants = []
        for p in children.values():
            try:
                descendants.extend(psutil.Process(p.pid).children(recursive=True))
            except psutil.Error:
                pass
        for p in children.values():
            try: os.killpg(p.pid,signal.SIGTERM)
            except ProcessLookupError: pass
        for p in descendants:
            try: p.terminate()
            except psutil.Error: pass
        deadline=time.monotonic()+grace
        while any(p.poll() is None for p in children.values()) and time.monotonic()<deadline:time.sleep(.1)
        for p in children.values():
            try: os.killpg(p.pid,signal.SIGKILL)
            except ProcessLookupError: pass
            p.wait()
        for p in descendants:
            try:
                if p.is_running():p.kill()
            except psutil.Error:pass
        for sig, handler in handlers.items():signal.signal(sig,handler)


def dashboard_healthy(marker=PROCESS_FILE):
    import psutil
    import urllib.request
    try:
        identity=json.loads(marker.read_text())['dashboard']
        process=psutil.Process(identity['pid'])
        if process.create_time()!=identity['started'] or process.status()==psutil.STATUS_ZOMBIE:return False
        request=urllib.request.Request('http://127.0.0.1:9119/api/status')
        # Never route a localhost probe through user HTTP_PROXY settings.
        opener=urllib.request.build_opener(urllib.request.ProxyHandler({}))
        with opener.open(request,timeout=2) as response:
            data=json.loads(response.read(1024*1024))
        return data.get('auth_required') is True and 'basic' in data.get('auth_providers',[])
    except Exception:
        return False
