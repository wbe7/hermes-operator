#!/bin/sh
set -eu
image=${HERMES_TOOLS_IMAGE:-wbe7/hermes:v2026.9.14}
docker run --rm --pull=never --user 10000:10000 --read-only \
  --cap-drop=ALL --security-opt=no-new-privileges --network=none \
  --tmpfs /tmp:rw,nosuid,nodev,size=2g \
  --entrypoint /opt/hermes/.venv/bin/python \
  -e HOME=/tmp/hermes-tools-home -e HERMES_HOME=/tmp/hermes-tools-home \
  "$image" -I /opt/hermes-tools/smoke.py
