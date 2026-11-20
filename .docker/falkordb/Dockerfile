# FalkorDB for the graph store (plans/graph-store, Phase 1).
#
# Thin wrapper over the upstream image, pinned by tag AND digest. The digest is
# the multi-arch index for v4.22.4 as published 2026-11-12. Move it only through
# the graph-store ADR, never to "latest".
#
# The container joins the internal `graphnet` Compose network and publishes no
# host port. EXPOSE below is documentation only and does not publish anything.
#
# Secrets are NOT set here. Compose passes the password at run time, e.g.
#   REDIS_ARGS: "--requirepass ${FALKORDB_PASSWORD:?set it}"
# so the image carries no credential.
FROM falkordb/falkordb:v4.22.4@sha256:4dfe286fe175f3d1ef0166f8910b867f36bca4b530b295abb2f6852f6230f1bd

LABEL org.opencontainers.image.title="sentinel-hunter-falkordb" \
      org.opencontainers.image.description="FalkorDB graph store for Sentinel Hunter (lab network only)" \
      org.opencontainers.image.licenses="see upstream falkordb/falkordb"

# Persisted data lives on a named volume mounted here by Compose.
ENV FALKORDB_DATA_PATH=/var/lib/falkordb/data
VOLUME ["/var/lib/falkordb/data"]

EXPOSE 6379

# Authenticated PING, so the check fails if the server is up but auth or the
# data path is broken (plans/graph-store/Phase-1.md, GF-1.5). REDISCLI_AUTH is
# read by redis-cli, which keeps the password out of the process arguments.
HEALTHCHECK --interval=10s --timeout=3s --start-period=20s --retries=5 \
  CMD sh -c 'REDISCLI_AUTH="$FALKORDB_PASSWORD" redis-cli ping | grep -q PONG'
