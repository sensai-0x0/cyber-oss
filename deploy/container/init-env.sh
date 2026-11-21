#!/bin/sh
set -eu

dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
target="$dir/.env"

if [ -e "$target" ]; then
  printf '%s already exists; refusing to overwrite\n' "$target" >&2
  exit 1
fi

if ! command -v openssl >/dev/null 2>&1; then
  printf 'openssl is required to generate local credentials\n' >&2
  exit 1
fi

umask 077
falkordb_password="$(openssl rand -hex 32)"
nats_password="$(openssl rand -hex 32)"

cat >"$target" <<EOF
FALKORDB_PASSWORD=${falkordb_password}
NATS_PASSWORD=${nats_password}

FALKORDB_DATABASE=sentinel_lite
FALKORDB_THREAD_COUNT=2
FALKORDB_TIMEOUT_MS=5000
FALKORDB_MAXMEMORY=512mb
FALKORDB_MEMORY_LIMIT=768m
ORCHESTRATOR_MEMORY_LIMIT=512m
EOF

printf 'Generated local credentials in %s\n' "$target"
