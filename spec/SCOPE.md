# Authorized scope and safety

**Status:** authoritative · **Derived from:** engagement authorization requirements · **Updated:** 2026-11-21

Sentinel Hunter actively connects to targets. Run it only where the operator has explicit authorization and a documented exclusions list. The included fixtures are the default development scope.

A hunt record must contain:

1. Authorization owner and evidence reference.
2. Exact CIDRs/domains, ports, exclusions, maximum request rate, and time window.
3. Cursor key identifier (not the secret key), starting checkpoint, and rollback owner.
4. Approval requirements for broad actions and maximum TTL, never above 24 hours.

Do not scan third parties, collect credentials, execute discovered content, or convert a heuristic score into attribution. The fake phishing panel rejects all POSTs and the fake C2 contains no command execution. Broad mitigation remains stopped until a named operator signs the exact target, TTL, and expiry.

For false positives or uncertain ownership, choose observation or early revocation. Reversible defense takes precedence over campaign coverage.
