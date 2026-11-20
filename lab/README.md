# Isolated lab

- `fake_c2.py`: harmless staging metadata; no beacon, payload, shell, or command execution.
- `phish_panel.py`: visual fixture with disabled inputs and rejected POSTs; it cannot collect credentials.
- `deploy/dns/`: loopback-only CoreDNS and RPZ zone fixtures.
- `firewall-target`: optional privileged Linux profile for manual nftables adapter checks.

Generate certificates, then run `docker compose -f deploy/compose.yaml up --build nats fake-c2 phish-panel dns`. Host ports are bound to loopback. Do not point lab tooling at external infrastructure.
