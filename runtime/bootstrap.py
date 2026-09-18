"""Restore declared leaves before every gateway exec; never replace the user's home."""
import copy
import json
import os
from pathlib import Path
import tempfile


def leaves(value, prefix=()):
    if isinstance(value, dict) and value:
        for key, child in value.items():
            if not isinstance(key, str):
                raise ValueError('invalid configuration key')
            yield from leaves(child, prefix + (key,))
    elif prefix:
        yield prefix, value


def merge_config(current: dict, desired: dict, previous_paths: set[tuple[str, ...]]) -> dict:
    if not isinstance(current, dict) or not isinstance(desired, dict):
        raise ValueError('configuration must be an object')
    result = copy.deepcopy(current)
    desired_leaves = dict(leaves(desired))
    def remove(path):
        node = result
        parents = []
        for key in path[:-1]:
            if not isinstance(node.get(key), dict):
                return
            parents.append((node, key))
            node = node[key]
        node.pop(path[-1], None)
        for parent, key in reversed(parents):
            if not parent[key]:
                del parent[key]
    for path in sorted(previous_paths, key=len, reverse=True):
        if path not in desired_leaves:
            remove(path)
    for path, value in desired_leaves.items():
        node = result
        for index, key in enumerate(path[:-1]):
            if key in node and not isinstance(node[key], dict):
                if path[:index + 1] not in previous_paths:
                    raise ValueError('configuration ownership collision')
                node[key] = {}
            node = node.setdefault(key, {})
        old = node.get(path[-1])
        if isinstance(old, dict) and old and old != value and path not in previous_paths:
            raise ValueError('configuration ownership collision')
        if value is None:
            remove(path)
        else:
            node[path[-1]] = copy.deepcopy(value)
    return result


def atomic_write(path: Path, text: str):
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, temporary = tempfile.mkstemp(prefix='.restore-', dir=path.parent)
    try:
        with os.fdopen(fd, 'w') as stream:
            stream.write(text)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
        directory = os.open(path.parent, os.O_RDONLY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def _manifest(path):
    return json.loads(path.read_text()) if path.exists() else {'ownedPaths': [], 'ownedEnv': []}


def _resolve(value, credentials):
    if isinstance(value, dict):
        if set(value) == {'credential'}:
            return credentials[value['credential']]
        return {key: _resolve(child, credentials) for key, child in value.items()}
    if isinstance(value, list):
        return [_resolve(child, credentials) for child in value]
    return value



def _config_credentials(value, credentials, env, names):
    """Keep credentials behind one-pass YAML env refs; literal ${...} stays literal."""
    import hashlib
    if isinstance(value, dict):
        if set(value) == {'credential'}:
            name = value['credential']
            key = names.get(name) or ('HERMES_OPERATOR_CREDENTIAL_' + hashlib.sha256(name.encode()).hexdigest()[:16].upper())
            if key in env and env[key] != credentials[name]:
                raise ValueError('credential environment collision')
            env[key] = credentials[name]
            return '${' + key + '}'
        return {key: _config_credentials(child, credentials, env, names) for key, child in value.items()}
    if isinstance(value, list):
        return [_config_credentials(child, credentials, env, names) for child in value]
    return value

def _restore(home: Path, bundle: dict, credentials: dict[str, str]) -> None:
    import yaml
    from adapters.v20260914 import reset_model_overrides, reset_provider_credentials, reset_channel_overrides, normalize_reasoning_map, reset_telegram_pairing
    try:
        if bundle['schema'] != 1 or bundle['release'] != 'v2026.9.14':
            raise ValueError()
        env = _resolve(bundle['env'], credentials)
        if set(env) != set(bundle['ownedEnv']):
            raise ValueError()
        names = {value['credential']: key for key, value in bundle['env'].items()
                 if isinstance(value, dict) and set(value) == {'credential'}}
        desired = _config_credentials(bundle['config'], credentials, env, names)
        owned = {tuple(path) for path in bundle['ownedPaths']}
        if set(dict(leaves(desired))) != owned:
            raise ValueError()
        manifests = [_manifest(home / '.operator' / name) for name in ('committed.json', 'pending.json')]
        previous = {tuple(path) for manifest in manifests for path in manifest['ownedPaths']}
        previous_env = set(env) | {key for manifest in manifests for key in manifest['ownedEnv']}
        config_path = home / 'config.yaml'
        current = yaml.safe_load(config_path.read_text()) if config_path.exists() else {}
        current = normalize_reasoning_map(current or {}, desired)
        merged = merge_config(current, desired, previous)
        from dotenv import dotenv_values
        env_path = home / '.env'
        current_env = dict(dotenv_values(env_path, interpolate=False)) if env_path.exists() else {}
        for key in previous_env:
            current_env.pop(key, None)
        current_env.update(env)
        for key in RESERVED_ENV:
            current_env.pop(key, None)
        if any(not isinstance(key, str) or not key.replace('_', 'a').isalnum() for key in current_env):
            raise ValueError()
        pending = {'ownedPaths': [list(path) for path in sorted(previous | owned)], 'ownedEnv': sorted(previous_env)}
        atomic_write(home / '.operator/pending.json', json.dumps(pending))
        reset_model_overrides(home)
        reset_telegram_pairing(home, desired)
        reset_provider_credentials(home, merged)
        reset_channel_overrides(merged)
        legacy_path = home / 'gateway.json'
        if legacy_path.exists():
            legacy = json.loads(legacy_path.read_text())
            reset_channel_overrides(legacy)
            atomic_write(legacy_path, json.dumps(legacy))
        atomic_write(config_path, yaml.safe_dump(merged, allow_unicode=True))
        def quote(value):
            return "'" + str(value).replace('\\', '\\\\').replace("'", "\\'") + "'"
        atomic_write(env_path, ''.join(key + '=' + quote(value) + '\n' for key, value in sorted(current_env.items()) if value is not None))
        if yaml.safe_load(config_path.read_text()) != merged:
            raise ValueError()
        committed = {'ownedPaths': bundle['ownedPaths'], 'ownedEnv': sorted(env)}
        atomic_write(home / '.operator/committed.json', json.dumps(committed))
        (home / '.operator/pending.json').unlink()
    except Exception:
        raise RuntimeError('startup restore failed; gateway was not started') from None



# These determine bootstrap/service identity rather than user provider preferences.
RESERVED_ENV = frozenset({'HOME', 'HERMES_HOME', 'HERMES_PROFILE', 'HERMES_DEFAULT_PROFILE',
    'PYTHONPATH', 'PYTHONHOME', 'PYTHON_DOTENV_DISABLED', 'GATEWAY_MULTIPLEX_PROFILES'})


def fix_service_environment(home):
    for key in RESERVED_ENV:
        os.environ.pop(key, None)
    os.environ.update(HOME=str(home), HERMES_HOME=str(home),
                      GATEWAY_MULTIPLEX_PROFILES='false', PYTHON_DOTENV_DISABLED='1')


def load_runtime_environment(home):
    from dotenv import dotenv_values
    for key, value in dotenv_values(home / '.env', interpolate=False).items():
        if value is not None and key not in RESERVED_ENV:
            os.environ[key] = value
    fix_service_environment(home)


def acquire_startup_lock(home):
    """Nonblocking local lock, inherited by gateway exec; never take over a live owner."""
    import fcntl
    lock_dir = home / '.operator'
    lock_dir.mkdir(parents=True, exist_ok=True)
    fd = os.open(lock_dir / 'startup.lock', os.O_CREAT | os.O_RDWR, 0o600)
    try:
        fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        # Also refuse a live gateway started outside this wrapper.
        if (home / 'gateway.pid').exists() or (home / 'gateway_state.json').exists():
            from gateway.status import live_gateway_pid_for_home
            if live_gateway_pid_for_home(home) is not None:
                raise RuntimeError()
        os.set_inheritable(fd, True)
        return fd
    except Exception:
        os.close(fd)
        raise RuntimeError('startup lock unavailable; gateway was not started') from None


def restore(home: Path, bundle: dict, credentials: dict[str, str]) -> None:
    fd = acquire_startup_lock(home)
    try:
        _restore(home, bundle, credentials)
    finally:
        os.close(fd)

def main():
    import argparse
    import sys
    # python -I removes PYTHONPATH and current-directory imports. Only these trusted,
    # read-only directories are added; /opt/data and workspace never enter sys.path.
    sys.path.insert(0, str(Path(__file__).resolve().parent))
    sys.path.insert(1, '/opt/hermes')
    parser = argparse.ArgumentParser()
    parser.add_argument('--bundle', default='/operator/config/input.json')
    parser.add_argument('--credentials', default='/operator/credentials')
    parser.add_argument('--restore-only', action='store_true')
    args = parser.parse_args()
    try:
        home = Path('/opt/data')
        fix_service_environment(home)
        startup_fd = acquire_startup_lock(home)
        bundle = json.loads(Path(args.bundle).read_text())
        names = set()
        def references(value):
            if isinstance(value, dict):
                if set(value) == {'credential'}:
                    names.add(value['credential'])
                else:
                    for child in value.values(): references(child)
            elif isinstance(value, list):
                for child in value: references(child)
        references(bundle)
        if any(not isinstance(name, str) or not name or Path(name).name != name or name in ('.', '..') for name in names):
            raise ValueError()
        credentials = {name: (Path(args.credentials)/name).read_text() for name in names}
        _restore(home, bundle, credentials)
        load_runtime_environment(home)
        (home/'workspace').mkdir(parents=True, exist_ok=True)
        os.chdir(home/'workspace')
        from tools.skills_sync import sync_skills
        sync_skills(quiet=True)
        if args.restore_only:
            os.close(startup_fd)
            return
    except Exception:
        print('startup restore failed; gateway was not started', file=sys.stderr)
        raise SystemExit(1) from None
    fix_service_environment(home)
    os.execv('/opt/hermes/.venv/bin/python', ['/opt/hermes/.venv/bin/python', '-I', '-c', 'import sys; sys.path.insert(0,"/opt/hermes"); from hermes_cli.main import main; main()', 'gateway', 'run', '--no-supervise'])


if __name__ == '__main__':
    main()
