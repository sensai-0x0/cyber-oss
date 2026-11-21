#!/bin/sh
set -eu

dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
debug="false"

if [ "${1:-}" = "--debug" ]; then
  debug="true"
  shift
fi

compose() {
  if [ "$debug" = "true" ]; then
    docker compose \
      --project-directory "$dir" \
      --env-file "$dir/.env" \
      -f "$dir/compose.yaml" \
      -f "$dir/compose.debug.yaml" \
      "$@"
    return
  fi
  docker compose \
    --project-directory "$dir" \
    --env-file "$dir/.env" \
    -f "$dir/compose.yaml" \
    "$@"
}

failures=0
fail() {
  printf 'FAIL  %s\n' "$1" >&2
  failures=$((failures + 1))
}
pass() {
  printf 'ok    %s\n' "$1"
}

if [ ! -f "$dir/.env" ]; then
  printf 'Missing %s/.env; run %s/init-env.sh first\n' "$dir" "$dir" >&2
  exit 1
fi

printf '== compose configuration ==\n'
if compose config --quiet; then
  pass "compose configuration is valid"
else
  fail "compose configuration is invalid"
  exit 1
fi

published="$(compose config --format json | python3 -c '
import json, sys
config = json.load(sys.stdin)
targets = {"6379": "falkordb", "4222": "nats", "50051": "hunterd", "8080": "orchestrator"}
exposed = []
for name, service in config.get("services", {}).items():
    for port in service.get("ports", []) or []:
        if str(port.get("target")) in targets and port.get("published"):
            exposed.append("%s:%s" % (name, port.get("published")))
print(",".join(exposed))
')"

if [ "$debug" = "true" ]; then
  expected="falkordb:16379,nats:24222,hunterd:15051,orchestrator:18080"
  published_sorted="$(printf '%s\n' "$published" | tr ',' '\n' | LC_ALL=C sort | paste -sd, -)"
  expected_sorted="$(printf '%s\n' "$expected" | tr ',' '\n' | LC_ALL=C sort | paste -sd, -)"
  if [ "$published_sorted" = "$expected_sorted" ]; then
    pass "debug ports are loopback-only: $published"
  else
    fail "debug ports differ from expectation (got '$published')"
  fi
  if compose config --format json | grep -q '0\.0\.0\.0:'; then
    fail "a debug port binds 0.0.0.0"
  fi
else
  if [ -z "$published" ]; then
    pass "no service ports are published"
  else
    fail "base stack publishes ports: $published"
  fi
fi

printf '\n== running services ==\n'
compose up -d >/dev/null

attempt=0
while [ "$attempt" -lt 30 ]; do
  ready_services=0
  statuses="$(compose ps --format '{{.Service}} {{.State}} {{.Health}}')"
  for service in falkordb nats hunterd orchestrator; do
    line="$(printf '%s\n' "$statuses" | awk -v s="$service" '$1 == s')"
    state="$(printf '%s' "$line" | awk '{print $2}')"
    health="$(printf '%s' "$line" | awk '{print $3}')"
    if [ "$state" = "running" ] && [ "$health" = "healthy" ]; then
      ready_services=$((ready_services + 1))
    fi
  done
  [ "$ready_services" -eq 4 ] && break
  attempt=$((attempt + 1))
  sleep 2
done

statuses="$(compose ps --format '{{.Service}} {{.State}} {{.Health}}')"
for service in falkordb nats hunterd orchestrator; do
  line="$(printf '%s\n' "$statuses" | awk -v s="$service" '$1 == s')"
  state="$(printf '%s' "$line" | awk '{print $2}')"
  health="$(printf '%s' "$line" | awk '{print $3}')"
  if [ "$state" != "running" ]; then
    fail "$service is not running (state '$state')"
  elif [ "$health" != "healthy" ]; then
    fail "$service is not healthy (health '$health')"
  else
    pass "$service is running and healthy"
  fi
done

printf '\n== graph network isolation ==\n'
internal="$(docker network inspect sentinel-graph --format '{{.Internal}}' 2>/dev/null || true)"
if [ "$internal" = "true" ]; then
  pass "sentinel-graph is internal"
else
  fail "sentinel-graph is not internal (got '$internal')"
fi

project="$(compose config --format json | python3 -c 'import json, sys; print(json.load(sys.stdin)["name"])')"
egress_image="${project}-certs-init:latest"
if ! docker image inspect "$egress_image" >/dev/null 2>&1; then
  compose build certs-init >/dev/null
fi

if docker run --rm --network sentinel-graph --entrypoint sh \
  "$egress_image" \
  -c 'wget -T 3 -q -O- http://example.com >/dev/null 2>&1' >/dev/null 2>&1; then
  fail "sentinel-graph allows internet egress"
else
  pass "sentinel-graph blocks internet egress"
fi

printf '\n== dependency checks ==\n'
if compose exec -T falkordb sh -c 'redis-cli --no-auth-warning ping' | grep -q PONG; then
  pass "FalkorDB accepts authenticated PING"
else
  fail "FalkorDB rejected authenticated PING"
fi

if compose exec -T falkordb sh -c 'redis-cli --no-auth-warning MODULE LIST' | grep -q falkordb; then
  pass "FalkorDB module is loaded"
else
  fail "FalkorDB module is not loaded"
fi

ready="$(compose exec -T orchestrator python -c \
  "import urllib.request; print(urllib.request.urlopen('http://127.0.0.1:8080/readyz', timeout=3).read().decode())" 2>/dev/null || true)"
case "$ready" in
  *'"graph":"reachable"'*) pass "orchestrator reports the graph reachable" ;;
  *) fail "orchestrator readiness failed: $ready" ;;
esac

if compose exec -T orchestrator python - <<'PY' >/dev/null 2>&1
import socket, ssl
raw = socket.create_connection(("nats", 4222), timeout=3)
raw.settimeout(3)
raw.makefile("rb").readline()
context = ssl.create_default_context()
context.check_hostname = False
context.verify_mode = ssl.CERT_NONE
with context.wrap_socket(raw, server_hostname="nats") as tls:
    assert tls.version() is not None
PY
then
  pass "NATS completes a TLS handshake"
else
  fail "NATS TLS handshake failed"
fi

printf '\n'
if [ "$failures" -ne 0 ]; then
  printf '%d check(s) failed\n' "$failures" >&2
  exit 1
fi
printf 'All container checks passed\n'
