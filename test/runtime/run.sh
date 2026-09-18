#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
image='nousresearch/hermes-agent@sha256:99641e57ec762c59e54cb44aa6746b7fc68c18b3c5ddb088af54234c613d9294'
docker run --rm --pull=never --user 10000:10000 --read-only --cap-drop=ALL --security-opt=no-new-privileges --tmpfs /tmp:rw,nosuid,nodev --entrypoint /opt/hermes/.venv/bin/python -v "$root/runtime:/runtime:ro" -e PYTHONDONTWRITEBYTECODE=1 -e PYTHONPATH=/runtime:/opt/hermes -e HERMES_HOME=/tmp/hermes-test -e HOME=/tmp/hermes-test "$image" -m unittest discover -s /runtime/tests -v
