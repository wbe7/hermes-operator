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
        if value is None:
            remove(path)
            continue
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


def restore(home: Path, bundle: dict, credentials: dict[str, str]) -> None:
    import yaml
    from adapters.v20260914 import reset_model_overrides, reset_provider_credentials, reset_channel_overrides
    try:
        if bundle['schema'] != 1 or bundle['release'] != 'v2026.9.14':
            raise ValueError()
        desired = _resolve(bundle['config'], credentials)
        env = _resolve(bundle['env'], credentials)
        owned = {tuple(path) for path in bundle['ownedPaths']}
        if set(dict(leaves(desired))) != owned or set(env) != set(bundle['ownedEnv']):
            raise ValueError()
        manifests = [_manifest(home / '.operator' / name) for name in ('committed.json', 'pending.json')]
        previous = owned | {tuple(path) for manifest in manifests for path in manifest['ownedPaths']}
        previous_env = set(env) | {key for manifest in manifests for key in manifest['ownedEnv']}
        config_path = home / 'config.yaml'
        current = yaml.safe_load(config_path.read_text()) if config_path.exists() else {}
        merged = merge_config(current or {}, desired, previous)
        from dotenv import dotenv_values
        env_path = home / '.env'
        current_env = dict(dotenv_values(env_path)) if env_path.exists() else {}
        for key in previous_env:
            current_env.pop(key, None)
        current_env.update(env)
        if any(not isinstance(key, str) or not key.replace('_', 'a').isalnum() for key in current_env):
            raise ValueError()
        pending = {'ownedPaths': [list(path) for path in sorted(previous)], 'ownedEnv': sorted(previous_env)}
        atomic_write(home / '.operator/pending.json', json.dumps(pending))
        reset_model_overrides(home)
        reset_provider_credentials(home, merged)
        reset_channel_overrides(merged)
        atomic_write(config_path, yaml.safe_dump(merged, allow_unicode=True))
        def quote(value):
            return "'" + str(value).replace('\\', '\\\\').replace("'", "\\'") + "'"
        atomic_write(env_path, ''.join(key + '=' + quote(value) + '\n' for key, value in sorted(current_env.items()) if value is not None))
        if yaml.safe_load(config_path.read_text()) != merged:
            raise ValueError()
        committed = {'ownedPaths': bundle['ownedPaths'], 'ownedEnv': bundle['ownedEnv']}
        atomic_write(home / '.operator/committed.json', json.dumps(committed))
        (home / '.operator/pending.json').unlink()
    except Exception:
        raise RuntimeError('startup restore failed; gateway was not started') from None


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
        os.environ['HOME'] = str(home)
        os.environ['HERMES_HOME'] = str(home)
        os.environ['GATEWAY_MULTIPLEX_PROFILES'] = 'false'
        os.environ.pop('HERMES_PROFILE', None)
        os.environ.pop('PYTHONPATH', None)
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
        restore(home, bundle, credentials)
        from dotenv import load_dotenv
        load_dotenv(home/'.env', override=True)
        os.environ['GATEWAY_MULTIPLEX_PROFILES'] = 'false'
        (home/'workspace').mkdir(parents=True, exist_ok=True)
        os.chdir(home/'workspace')
        from tools.skills_sync import sync_skills
        sync_skills(quiet=True)
        if args.restore_only:
            return
    except Exception:
        print('startup restore failed; gateway was not started', file=sys.stderr)
        raise SystemExit(1) from None
    os.execv('/opt/hermes/.venv/bin/python', ['/opt/hermes/.venv/bin/python', '-I', '-c', 'import sys; sys.path.insert(0,"/opt/hermes"); from hermes_cli.main import main; main()', 'gateway', 'run', '--no-supervise'])


if __name__ == '__main__':
    main()
