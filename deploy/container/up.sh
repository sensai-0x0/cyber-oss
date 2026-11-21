#!/bin/sh
set -eu

dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
debug="false"

if [ "${1:-}" = "--debug" ]; then
  debug="true"
  shift
fi

if [ ! -f "$dir/.env" ]; then
  "$dir/init-env.sh"
fi

if [ "$debug" = "true" ]; then
  exec docker compose \
    --project-directory "$dir" \
    --env-file "$dir/.env" \
    -f "$dir/compose.yaml" \
    -f "$dir/compose.debug.yaml" \
    "$@"
fi

exec docker compose \
  --project-directory "$dir" \
  --env-file "$dir/.env" \
  -f "$dir/compose.yaml" \
  "$@"
