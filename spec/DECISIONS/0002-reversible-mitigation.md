# ADR 0002: Signed actions with mandatory expiry

- Status: accepted
- Date: 2026-11-21

Use Ed25519 signatures and a maximum 24-hour TTL. Broad actions require a named operator. Revocation does not require a new approval because it restores the prior state and reduces impact. A failed revocation stays in the tracked set for retry; it is never treated as successful expiry.

The lab uses fake executors for automated tests. Real nftables and DNS changes require explicit runtime wiring and are outside ordinary CI.
