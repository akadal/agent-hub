#!/usr/bin/env bash
# Run Agent Hub API on the host OS so Tailscale (100.x) SSH works.
# Pair with: docker compose -f docker-compose.yml -f docker-compose.host-api.yml up -d web ssh-target
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

DATA_DIR="${DATA_DIR:-$ROOT/data}"
mkdir -p "$DATA_DIR"

# One-time: pull store from the compose volume if local data is empty
if [[ ! -f "$DATA_DIR/store.json" ]]; then
  if docker volume inspect agent-hub_api-data >/dev/null 2>&1; then
    echo "copying store from docker volume agent-hub_api-data → $DATA_DIR"
    docker run --rm \
      -v agent-hub_api-data:/data \
      -v "$DATA_DIR:/out" \
      alpine sh -c 'cp -a /data/. /out/ 2>/dev/null || true'
  fi
fi

export HTTP_ADDR="${HTTP_ADDR:-:27341}"
export DATA_DIR
export JWT_SECRET="${JWT_SECRET:-dev-only-change-me-agent-hub}"
export JWT_ACCESS_TTL="${JWT_ACCESS_TTL:-forever}"
export BOOTSTRAP_ADMIN_USERNAME="${BOOTSTRAP_ADMIN_USERNAME:-admin}"
export BOOTSTRAP_ADMIN_PASSWORD="${BOOTSTRAP_ADMIN_PASSWORD:-123456}"
export ACCESS_DEFAULT_TAILSCALE_ONLY="${ACCESS_DEFAULT_TAILSCALE_ONLY:-false}"

# Free host port if an old container still binds it
if command -v docker >/dev/null 2>&1; then
  docker rm -f agent-hub-api 2>/dev/null || true
fi

echo "API on host $HTTP_ADDR  DATA_DIR=$DATA_DIR  (Tailscale-capable)"
cd "$ROOT/backend"
exec go run ./cmd/agent-hub
