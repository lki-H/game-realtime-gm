# Security policy

This is a local learning and portfolio backend, with a separately isolated RPC/message experiment. It is not a public hosted game service or a production deployment.

Use GitHub private vulnerability reporting when available. Otherwise contact the repository owner privately; do not publish credentials, player data or exploit payloads in an issue.

Reports should identify the commit, affected mode, minimal reproduction and impact. Relevant areas include authentication, authorization, reward/ledger integrity, stale connections, worker retries, RPC/message scopes, resource limits, deployment tooling and secret handling.

New installations use the loopback-only isolated launcher and external random credentials. Existing storage is never a test fixture. The launcher must not rotate existing passwords, reset databases or delete volumes to repair a failed check. Legacy operation is an explicit historical mode, not an Internet compatibility guarantee.

CI and tests run with synthetic data and dedicated resources. Public logs must not contain passwords, JWTs, private configuration, full credential-bearing URLs or real message contents. Diagnostics and retries must remain bounded.

See [current security design](docs/security-design.md), [maintenance scope](docs/maintenance/README.md) and [R5 boundaries](experiments/r5-m6/README.md). Findings are investigated and fixed with regression tests; a clean test result is not a proof of absolute security or production readiness.
