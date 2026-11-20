#!/bin/sh
set -eu
out="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/generated"
mkdir -p "$out"
chmod 700 "$out"
openssl req -x509 -newkey rsa:3072 -nodes -days 1 -subj '/CN=Sentinel Hunter Lab CA' \
  -keyout "$out/ca.key" -out "$out/ca.crt" >/dev/null 2>&1
openssl req -newkey rsa:3072 -nodes -subj '/CN=nats' \
  -addext 'subjectAltName=DNS:nats,DNS:localhost,IP:127.0.0.1' \
  -keyout "$out/nats-server.key" -out "$out/nats-server.csr" >/dev/null 2>&1
openssl x509 -req -days 1 -in "$out/nats-server.csr" -CA "$out/ca.crt" -CAkey "$out/ca.key" \
  -CAcreateserial -copy_extensions copy -out "$out/nats-server.crt" >/dev/null 2>&1
chmod 600 "$out"/*.key
printf 'Generated one-day lab certificates in %s\n' "$out"
