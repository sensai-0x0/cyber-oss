# syntax=docker/dockerfile:1.7

FROM golang:1.27.4-bookworm AS build

WORKDIR /src
ENV CGO_ENABLED=0 \
    GOFLAGS=-buildvcs=false

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY hunterd ./hunterd
COPY enforcerd ./enforcerd
COPY operator ./operator

FROM build AS hunterd-build
RUN --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w -buildid=" -o /out/hunterd ./hunterd/cmd/hunterd

FROM gcr.io/distroless/static-debian12:nonroot AS hunterd
COPY --from=hunterd-build --chown=nonroot:nonroot /out/hunterd /hunterd
USER nonroot:nonroot
EXPOSE 50051
ENTRYPOINT ["/hunterd"]
CMD ["serve", "-grpc-listen", "0.0.0.0:50051"]
HEALTHCHECK --interval=15s --timeout=4s --start-period=10s --retries=3 \
    CMD ["/hunterd", "healthcheck", "-addr", "127.0.0.1:50051"]


FROM build AS operator-build
RUN --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w -buildid=" -o /out/operator ./operator/cmd/operator

FROM gcr.io/distroless/static-debian12:nonroot AS operator
COPY --from=operator-build --chown=nonroot:nonroot /out/operator /operator
USER nonroot:nonroot
ENTRYPOINT ["/operator"]
