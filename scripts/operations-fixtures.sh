#!/usr/bin/env bash
# Isolated fixtures for tests/browser/operations*.cjs on the authorized Kali lab.
set -euo pipefail
case "${1:-}" in
up)
ssh -o BatchMode=yes kali@192.168.56.103 'sh -s' <<'REMOTE'
set -eu
for name in gomate-ui-ops gomate-ui-tty; do
 if docker inspect "$name" >/dev/null 2>&1; then echo "Already exists: $name; inspect or remove the fixture before setup." >&2; exit 1; fi
done
docker run -d --name gomate-ui-ops --label gomate.ui-fixture=true --cpus .25 --memory 64m --read-only --user 65534:65534 --cap-drop ALL --entrypoint /bin/sh postgres:17.7-alpine -c 'trap "exit 0" TERM; i=0; while [ "$i" -lt 2500 ]; do printf "LOG_UI_STDOUT_%04d synthetic line\n" "$i"; if [ $((i % 5)) -eq 0 ]; then printf "LOG_UI_ERROR_%04d synthetic error\n" "$i" >&2; fi; i=$((i+1)); done; printf "<img src=x onerror=window.logXSS=1>\n"; while :; do echo LOG_UI_HEARTBEAT; echo LOG_UI_ERROR_HEARTBEAT >&2; sleep 2; done'
docker run -d -t --name gomate-ui-tty --label gomate.ui-fixture=true --cpus .25 --memory 32m --read-only --user 65534:65534 --cap-drop ALL --entrypoint /bin/sh postgres:17.7-alpine -c 'trap "exit 0" TERM; while :; do echo LOG_UI_TTY; sleep 2; done'
REMOTE
;;
down)
ssh -o BatchMode=yes kali@192.168.56.103 'sh -s' <<'REMOTE'
set -eu
for name in gomate-ui-ops gomate-ui-tty; do
 if ! docker inspect "$name" >/dev/null 2>&1; then continue; fi
 label=$(docker inspect --format '{{ index .Config.Labels "gomate.ui-fixture" }}' "$name")
 if [ "$label" != true ]; then echo "Refusing to remove non-fixture: $name" >&2; exit 1; fi
 if [ "$(docker inspect --format '{{.State.Paused}}' "$name")" = true ]; then docker unpause "$name"; fi
 docker stop -t 5 "$name"
 docker rm "$name"
done
REMOTE
;;
*) echo "Usage: bash scripts/operations-fixtures.sh up|down" >&2; exit 2;;
esac
