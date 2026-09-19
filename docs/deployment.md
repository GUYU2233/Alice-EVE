# Deployment boundary (operator-owned)

The repository intentionally documents only a **generic** deployment shape. Real domains, IPs, Cloudflare zone/account identifiers, SSH aliases, Nginx vhosts, systemd paths, database DSNs, and backup locations belong in the operator's private runbook and secret manager—not in GitHub.

```text
managed HTTPS reverse proxy / CDN
          ↓
private Server listener (loopback or private subnet)
          ↓
managed PostgreSQL (private network, TLS verification)
```

## Safe setup

1. Copy `relay-server/config/production.env.example` to an untracked, root-readable environment file or inject variables from a secret manager.
2. Replace every placeholder with environment-specific values. Use an operator-owned HTTPS origin; do not commit it.
3. Choose a migration mode: the default `MIGRATIONS_MODE=apply` acquires an advisory lock, verifies checksums and applies embedded migrations using `DATABASE_URL` before the listener opens. If a temporary migration role runs the same runner separately, start the runtime role with `MIGRATIONS_MODE=off`.
4. Terminate TLS at the reverse proxy, enforce HTTPS, strict CORS/Origin policy, request limits, rate limits, and proxy header sanitization.
5. Bind the Server to loopback/private networking. Permit database access only from the Server identity/security group.
6. Run `scripts/security-audit.ps1 -IncludeIgnored` (and repository history secret scanning) before creating a public commit or release.
7. Review `git status --short` and the staged diff. Do not stage `.dsh-*`, `.superdesign/`, build artifacts, databases, logs, local environment files or private runbooks.

The checked-in `relay-server/deploy/alice-relay.service` is a generic Server example; its legacy filename is retained for compatibility. Its executable and environment-file paths are deployment placeholders; keep real paths and credentials in private host configuration. Never use this repository as a source of truth for server state, and do not apply deployment changes automatically to a real server.
