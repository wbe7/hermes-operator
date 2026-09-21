"""Adapter for official v2026.9.14, revision 345cd2b057a452236de401d3534b8502a7465e8d."""
import json
from pathlib import Path
import sqlite3
from contextlib import closing


def reset_model_overrides(home: Path) -> None:
    """Fail closed on primary failure; remove active routing fields, never history."""
    from bootstrap import atomic_write
    try:
        mirror_path = home / 'sessions/sessions.json'
        mirror = json.loads(mirror_path.read_text()) if mirror_path.exists() else {}
        for key, entry in mirror.items():
            if not key.startswith('_'):
                entry.pop('model_override', None)
        database = home / 'state.db'
        if database.exists():
            with closing(sqlite3.connect(f'file:{database}?mode=rw', uri=True, timeout=5)) as db, db:
                if db.execute('PRAGMA quick_check').fetchone()[0] != 'ok':
                    raise ValueError()
                exists = db.execute("SELECT 1 FROM sqlite_master WHERE type='table' AND name='gateway_routing'").fetchone()
                if exists:
                    scope = str((home / 'sessions').resolve())
                    for key, raw in db.execute('SELECT session_key,entry_json FROM gateway_routing WHERE scope=?', (scope,)).fetchall():
                        entry = json.loads(raw)
                        entry.pop('model_override', None)
                        db.execute('UPDATE gateway_routing SET entry_json=? WHERE scope=? AND session_key=?', (json.dumps(entry), scope, key))
                    for (raw,) in db.execute('SELECT entry_json FROM gateway_routing WHERE scope=?', (scope,)):
                        if json.loads(raw).get('model_override'):
                            raise ValueError()
        if mirror_path.exists():
            atomic_write(mirror_path, json.dumps(mirror))
    except Exception:
        raise RuntimeError('session restore failed') from None


def _runtime_status(home):
    from gateway.status import read_runtime_status, get_runtime_status_running_pid
    record = read_runtime_status(home / 'gateway_state.json')
    pid = get_runtime_status_running_pid(record, expected_home=home)
    return record, (pid, record['start_time']) if pid is not None else None


def _tick(home, pid):
    import socket
    with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as client:
        client.settimeout(2)
        client.connect(str(home / 'state' / f'gateway.loop-tick.{pid}.sock'))
        return client.recv(1) == b'1'


def _created_at(pid):
    import psutil
    return psutil.Process(pid).create_time()


def _health(home, readiness):
    import time
    try:
        record, identity = _runtime_status(home)
        if not identity or not record:
            return False
        pid, start = identity
        if record.get('pid') != pid or record.get('start_time') != start:
            return False
        beat = json.loads((home/'state/gateway.heartbeat').read_text())
        age = time.monotonic() - float(beat['monotonic'])
        if (beat.get('pid') != pid or not _created_at(pid) <= float(beat['start_time']) <= time.time() or
                not 0 <= age <= 90 or not beat.get('loop_tick_socket') or not _tick(home, pid)):
            return False
        if not readiness:
            return True
        telegram = record.get('platforms', {}).get('telegram', {})
        return (record.get('gateway_state') == 'running' and
                record.get('session_store', {}).get('status') == 'ok' and
                telegram.get('state') == 'connected' and
                telegram.get('writer_pid') == pid and telegram.get('writer_start_time') == start)
    except Exception:
        return False


def ready(home: Path) -> bool:
    return _health(home, True)


def live(home: Path) -> bool:
    return _health(home, False)


def reset_provider_credentials(home, config):
    """Remove only stored pool credentials that could supersede the selected provider."""
    path = home / 'auth.json'
    if not path.exists() and not config.get('providers') and not config.get('custom_providers'):
        return
    from bootstrap import atomic_write
    from agent.credential_pool import _iter_custom_providers, _pool_keys_for_custom_entry
    from hermes_cli.runtime_provider_custom import _entry_url
    auth = json.loads(path.read_text()) if path.exists() else {}
    model = config.get('model', {})
    provider = model.get('provider')
    keys = {provider} if provider and provider != 'custom' else set()
    endpoint = str(model.get('base_url') or '').strip().rstrip('/')
    if provider == 'custom':
        # The pinned pool normalizer prefers base_url, unlike the named runtime
        # resolver. Give it a read-only view with the resolver's effective URL
        # so conflicting aliases cannot retire another provider's pool.
        pool_config = {**config, 'providers': {
            name: {**entry, 'base_url': _entry_url(entry)} if isinstance(entry, dict) else entry
            for name, entry in config.get('providers', {}).items()
        }}
        for name, entry in _iter_custom_providers(pool_config):
            if str(entry.get('base_url') or '').strip().rstrip('/') == endpoint:
                keys.update(_pool_keys_for_custom_entry(name, entry))
    if provider == 'custom':
        # New keyed providers use api > url > base_url; the legacy list only
        # accepts base_url. Do not match a shadowed URL and overwrite another
        # endpoint's credentials.
        entries = [(entry, _entry_url) for entry in config.get('providers', {}).values()]
        entries += [(entry, lambda value: value.get('base_url')) for entry in config.get('custom_providers', [])]
        for entry, address in entries:
            if isinstance(entry, dict) and str(address(entry) or '').strip().rstrip('/') == endpoint:
                entry['api_key'] = model.get('api_key', '')
                if model.get('api_mode'):
                    entry['api_mode'] = model['api_mode']
                for field in ('key_cmd', 'key_env', 'api_key_env'):
                    entry.pop(field, None)
    pool = auth.get('credential_pool', {})
    for key in keys:
        pool.pop(key, None)
    if path.exists():
        atomic_write(path, json.dumps(auth))


def reset_channel_overrides(config):
    """Per-channel model/provider routing is managed; personal prompts are retained."""
    for section in (config, config.get('gateway', {})):
        platforms = list(section.get('platforms', {}).values())
        if isinstance(section.get('telegram'), dict):
            platforms.append(section['telegram'])
        for platform in platforms:
            for channel in platform.get('channel_overrides', {}).values():
                channel.pop('model', None)
                channel.pop('provider', None)


def normalize_reasoning_map(current, desired):
    """The sole map-valued base field has exact replacement semantics.

    Clear only this versioned administrative map before generic ownership checks;
    never extend this exception to arbitrary requested extra paths or ancestors.
    """
    import copy
    desired_agent = desired.get('agent', {})
    if isinstance(desired_agent, dict) and isinstance(desired_agent.get('reasoning_overrides'), dict):
        current = copy.deepcopy(current)
        agent = current.get('agent')
        if isinstance(agent, dict):
            agent.pop('reasoning_overrides', None)
    return current


def normalize_model_config(current, desired):
    """Accept the pinned resolver's scalar model shorthand before owned-leaf merge."""
    import copy
    model = current.get('model')
    if isinstance(desired.get('model'), dict) and isinstance(model, str) and model.strip():
        current = copy.deepcopy(current)
        current['model'] = {'default': model.strip()}
    return current


def reset_telegram_pairing(home, desired):
    """CR allowlist supersedes persisted Telegram grants, including split layouts.

    Native PairingStore imports alternate-layout grants on construction and its
    public revoke mutates ambient dotenv. Filter the pinned JSON schema in both
    locations instead; preserve other platforms, rate limits and allowed users.
    Fail closed on malformed data rather than invoking upstream's forgiving read.
    """
    from bootstrap import atomic_write
    allowed = desired.get('gateway', {}).get('platforms', {}).get('telegram', {}).get('extra', {}).get('allow_from')
    if not isinstance(allowed, list) or not allowed:
        return
    for directory in (home / 'pairing', home / 'platforms' / 'pairing'):
        for suffix in ('approved', 'pending'):
            path = directory / ('telegram-' + suffix + '.json')
            if path.exists():
                record = json.loads(path.read_text())
                if not isinstance(record, dict):
                    raise ValueError('invalid Telegram pairing state')
                retained = {key: value for key, value in record.items() if key in allowed} if suffix == 'approved' else {}
                atomic_write(path, json.dumps(retained))
