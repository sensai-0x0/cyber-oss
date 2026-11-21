# Container environment

Local, single-host container plane for Sentinel Hunter. It builds and runs the
Go services, the Python orchestrator, NATS, and the FalkorDB graph backend on
two Docker networks.

- `sentinel-app` — application traffic (`hunterd`, orchestrator, NATS).
- `sentinel-graph` — graph traffic only. **Internal**: containers on it cannot
  reach the internet, and nothing on it is reachable from the host unless the
  debug overlay is used.

The base stack publishes **no host ports**. The graph database is reachable
inside the stack as `falkordb:6379`.

## Requirements

- Docker with Compose v2 (Docker Desktop is fine).
- `openssl` on the host to generate local credentials.
- `python3` on the host if you use `verify.sh`.

## Quick start

```sh
./deploy/container/init-env.sh          # writes ignored .env with random secrets
COMPOSE_BAKE=false ./deploy/container/up.sh build
./deploy/container/up.sh up -d
./deploy/container/verify.sh
```

If your Docker installation fails Compose builds with an HTTP 500, keep
`COMPOSE_BAKE=false` in front of build commands.

With loopback-only debug ports:

```sh
./deploy/container/up.sh --debug up -d
./deploy/container/verify.sh --debug
```

Stop without deleting graph data:

```sh
./deploy/container/up.sh down
```

Delete everything, including the FalkorDB volume:

```sh
./deploy/container/up.sh down -v
```

## Services

| Service        | Image                                  | Internal port | Purpose                                        |
| -------------- | -------------------------------------- | ------------- | ---------------------------------------------- |
| `certs-init`   | built from `certs.Dockerfile`          | none          | one-shot NATS CA/server certificate generation |
| `falkordb`     | `falkordb/falkordb-server:v4.22.4`     | 6379          | graph database, password protected             |
| `nats`         | `nats:2.12-alpine`                     | 4222, 8222    | TLS-required broker with JetStream             |
| `hunterd`      | built from `go.Dockerfile` (`hunterd`) | 50051         | gRPC ingestion service                         |
| `orchestrator` | built from `python.Dockerfile`         | 8080          | health/readiness runtime for the orchestrator  |

`operator` is a separate build target, not a long-running service:

```sh
docker build -f deploy/container/go.Dockerfile --target operator -t sentinel-hunter-operator:local .
```

Debug overlay (`compose.debug.yaml`) binds every port to `127.0.0.1` only:

| Service        | Host address       | Container port |
| -------------- | ------------------ | -------------- |
| `falkordb`     | `127.0.0.1:16379`  | 6379           |
| `nats`         | `127.0.0.1:24222`  | 4222           |
| `hunterd`      | `127.0.0.1:15051`  | 50051          |
| `orchestrator` | `127.0.0.1:18080`  | 8080           |

## Credentials and configuration

`init-env.sh` writes `deploy/container/.env`, which is git-ignored:

- `FALKORDB_PASSWORD` — required `--requirepass` value.
- `NATS_PASSWORD` — password for the `hunter` NATS user.
- `FALKORDB_DATABASE`, `FALKORDB_THREAD_COUNT`, `FALKORDB_TIMEOUT_MS`,
  `FALKORDB_MAXMEMORY`, `FALKORDB_MEMORY_LIMIT`, `ORCHESTRATOR_MEMORY_LIMIT` —
  optional tuning with defaults.

Compose fails closed when a secret is missing (`${VAR:?}`). To rotate secrets,
delete `.env`, re-run `init-env.sh`, then recreate the stack with
`./deploy/container/up.sh up -d --force-recreate`.

`nats-certs` holds a 30-day CA and `nats` server certificate. `certs-init`
regenerates them when either certificate is within 24 hours of expiry, so the
next `up` repairs an expired stack.

## Health checks

- `falkordb`: authenticated `redis-cli ping` returns `PONG`.
- `nats`: monitoring endpoint on container port 8222.
- `hunterd`: `hunterd healthcheck` calls the gRPC health service.
- `orchestrator`: `/healthz` is process liveness; `/readyz` authenticates to
  FalkorDB and returns 503 when the graph is unreachable.

`verify.sh` checks configuration, port exposure, service health, graph network
isolation, FalkorDB authentication, orchestrator readiness, and the NATS TLS
handshake. It exits non-zero on the first failing category.

## Data

- `sentinel-hunter-container_falkordb-data` — mounted at
  `/var/lib/falkordb/data`; AOF with `appendfsync everysec`.
- `sentinel-hunter-container_nats-data` — JetStream store.
- `sentinel-hunter-container_nats-certs` — generated TLS material.

The FalkorDB container runs with `--maxmemory 512mb --maxmemory-policy
noeviction`, so a full graph fails writes instead of silently evicting data.
Raise `FALKORDB_MAXMEMORY` and `FALKORDB_MEMORY_LIMIT` together.

## Hardening

Every long-running service drops all capabilities, sets
`no-new-privileges`, uses an init process, bounds log size, and sets memory and
PID limits. `hunterd` and `orchestrator` have read-only root filesystems with a
small `/tmp`; `nats` is read-only apart from its volumes.

## Related

- `db/CONTAINER_CONFIG.md` — the connection contract for graph database code.
- `db/compose.yaml` — the database agent's standalone, unauthenticated
  loopback-only FalkorDB. It is a separate instance from this stack.
