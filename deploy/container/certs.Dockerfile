# syntax=docker/dockerfile:1.7

FROM alpine:3.23.8

RUN apk add --no-cache openssl

COPY deploy/container/generate-certs.sh /usr/local/bin/generate-certs
RUN chmod 0555 /usr/local/bin/generate-certs \
    && mkdir -p /certs \
    && chmod 0700 /certs

ENTRYPOINT ["/usr/local/bin/generate-certs"]
