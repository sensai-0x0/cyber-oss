#!/bin/sh
set -eu

out="${1:-/certs}"
mkdir -p "$out"
chmod 0700 "$out"

# Reuse existing certificates only while they remain valid for another day.
if [ -s "$out/ca.crt" ] && [ -s "$out/nats-server.crt" ] && [ -s "$out/nats-server.key" ] \
  && openssl x509 -noout -checkend 86400 -in "$out/ca.crt" >/dev/null 2>&1 \
  && openssl x509 -noout -checkend 86400 -in "$out/nats-server.crt" >/dev/null 2>&1; then
  exit 0
fi

rm -f "$out"/*.crt "$out"/*.key "$out"/*.csr "$out"/*.srl

openssl req -x509 -newkey rsa:3072 -nodes -days 30 \
  -subj '/CN=Sentinel Hunter Container CA' \
  -keyout "$out/ca.key" -out "$out/ca.crt" >/dev/null 2>&1

openssl req -newkey rsa:3072 -nodes -subj '/CN=nats' \
  -addext 'subjectAltName=DNS:nats,DNS:localhost,IP:127.0.0.1' \
  -keyout "$out/nats-server.key" -out "$out/nats-server.csr" >/dev/null 2>&1

openssl x509 -req -days 30 -in "$out/nats-server.csr" \
  -CA "$out/ca.crt" -CAkey "$out/ca.key" -CAcreateserial \
  -copy_extensions copy -out "$out/nats-server.crt" >/dev/null 2>&1

chmod 0644 "$out"/*.crt
chmod 0600 "$out"/*.key
rm -f "$out"/*.csr
