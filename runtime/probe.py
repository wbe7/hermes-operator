"""Read declared channels from the immutable bundle; emit no runtime data."""
from pathlib import Path
import json
import sys
sys.path.insert(0, str(Path(__file__).resolve().parent))
sys.path.insert(1, '/opt/hermes')
from adapters.v20260914 import live, ready
from supervisor import channels, dashboard_healthy
if __name__ == '__main__':
    if len(sys.argv) != 2 or sys.argv[1] not in ('live', 'ready'):
        raise SystemExit(2)
    try:
        declared=channels(json.loads(Path('/operator/config/input.json').read_text()))
        home=Path('/opt/data')
        healthy=live(home) if sys.argv[1]=='live' else ready(home,telegram_enabled=declared['telegram'])
        if healthy and declared['web']:
            healthy=dashboard_healthy()
    except Exception:
        healthy=False
    raise SystemExit(0 if healthy else 1)
