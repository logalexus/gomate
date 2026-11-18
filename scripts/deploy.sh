#!/usr/bin/env sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"

if [ "$(uname -s)" != Linux ]; then
  echo "Gomate packet capture requires a Linux host." >&2
  exit 1
fi
if ! command -v docker >/dev/null 2>&1; then
  echo "Install Docker Engine and Compose v2, then run this script again." >&2
  exit 1
fi
if ! docker compose version >/dev/null 2>&1; then
  echo "Docker Compose v2 is required (docker compose)." >&2
  exit 1
fi

if [ ! -f .env ]; then
  if ! command -v openssl >/dev/null 2>&1; then
    echo "openssl is required to generate local credentials." >&2
    exit 1
  fi
  umask 077
  DB_PASSWORD=$(openssl rand -hex 32)
  WEB_PASSWORD=$(openssl rand -hex 32)
  DOCKER_DISCOVERY_TOKEN=$(openssl rand -hex 32)
  {
    printf 'DB_PASSWORD=%s\n' "$DB_PASSWORD"
    printf 'WEB_LOGIN=packmate\n'
    printf 'WEB_PASSWORD=%s\n' "$WEB_PASSWORD"
    printf 'DB_PORT=65432\n'
    printf 'LISTEN=127.0.0.1:65000\n'
    printf 'INTERFACE=\nLOCAL_IPS=\nRETENTION=1h\n'
    printf 'DOCKER_DISCOVERY_TOKEN=%s\n' "$DOCKER_DISCOVERY_TOKEN"
  } > .env
  echo "Created .env with random credentials (file permissions are private)."
else
  echo "Using existing .env. Verify its passwords and discovery token before deployment."
fi

docker compose --profile tools build runner-image
docker compose up --build -d --wait
echo "Gomate containers started. UI address and login are set by LISTEN and WEB_LOGIN in .env."
echo "View the generated UI password with: sed -n 's/^WEB_PASSWORD=//p' .env"
