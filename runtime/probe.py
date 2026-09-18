"""Invoke with the image Python in isolated mode (-I); emit no runtime data."""
from pathlib import Path
import sys
sys.path.insert(0, str(Path(__file__).resolve().parent))
sys.path.insert(1, '/opt/hermes')
from adapters.v20260914 import live, ready
if __name__ == '__main__':
    if len(sys.argv) != 2 or sys.argv[1] not in ('live', 'ready'):
        raise SystemExit(2)
    raise SystemExit(0 if {'live': live, 'ready': ready}[sys.argv[1]](Path('/opt/data')) else 1)
