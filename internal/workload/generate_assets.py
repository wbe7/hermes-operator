"""Copy canonical runtime files, or verify generated copies without writing."""
import hashlib
from pathlib import Path
import sys

root = Path(__file__).resolve().parents[2]
target = Path(__file__).resolve().parent / 'assets'
paths = ['bootstrap.py', 'probe.py', 'adapters/__init__.py', 'adapters/v20260914.py']
expected = {name: (root / 'runtime' / name).read_bytes() for name in paths}
expected['SHA256SUMS'] = ''.join(f'{hashlib.sha256(data).hexdigest()}  {name}\n' for name, data in sorted(expected.items())).encode()
if '--check' in sys.argv:
    actual = {str(p.relative_to(target)): p.read_bytes() for p in target.rglob('*') if p.is_file()}
    if actual != expected:
        sys.exit('runtime assets are stale; run make generate-runtime-assets')
else:
    for name, data in expected.items():
        path = target / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data)
