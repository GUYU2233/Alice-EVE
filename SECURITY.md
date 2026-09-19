# Security and public-repository boundary

## Scope

Alice-EVE is local-first. The public repository contains source code and **redacted deployment templates only**. It must not contain credentials, personal access tokens, EVE SSO tokens, device tokens, database dumps, private keys, certificates, hostnames, public IP addresses, DNS/Cloudflare identifiers, or operator filesystem paths.

Run the audit before every public push:

```powershell
pwsh -File .\scripts\security-audit.ps1
# Include ignored/untracked files when reviewing a worktree before release:
pwsh -File .\scripts\security-audit.ps1 -IncludeIgnored
```

The audit is a release gate, not a secret scanner replacement. Review Git history and rotate any credential that was ever committed. Use repository secret scanning (for example GitHub secret scanning/push protection) and an external scanner such as Gitleaks in CI where available.

Before staging a public push, also review `git status --short` and exclude local tool state such as `.dsh-*`, `.superdesign/`, IDE state, build artifacts, databases and runtime logs. Inspect the staged diff—not only the working tree—before pushing.

## Production configuration

Start from [`relay-server/config/production.env.example`](relay-server/config/production.env.example), then provide the real values through a protected secret manager or a root-readable, untracked environment file. Never put secrets in a systemd unit, Nginx config committed to Git, shell history, issue, log, or chat transcript.

Recommended controls:

- Bind the Server to loopback/private networking and terminate TLS at a managed reverse proxy. Do not expose PostgreSQL or the Server listener directly to the Internet.
- Use a dedicated database and least-privileged role. Require PostgreSQL TLS certificate verification, restrict network access, and rotate credentials through the secret manager.
- Give Cloudflare/API automation narrowly scoped, short-lived credentials; separate read-only audit credentials from DNS/change credentials. Do not commit zone IDs, account IDs, origin IPs, or API tokens.
- Keep device tokens hashed at rest, short-lived/revocable/rotated, and scoped to one device. Never log Authorization headers or token values.
- Apply OWASP API controls: authentication and authorization on every non-health endpoint, object ownership checks, request/body limits, rate limits, strict CORS/Origin policy, generic error responses, and security event auditing without sensitive payloads.
- Run the service as a non-root user with a private writable directory. Use systemd hardening (`NoNewPrivileges`, `PrivateTmp`, `ProtectSystem=strict`, `ProtectHome`, `ReadWritePaths`) and explicit outbound/network permissions.
- Keep backups encrypted, access-controlled, tested for restore, and outside the source tree. Publish checksums/signatures only; exclude binaries and build output from commits.
- Desktop/mobile clients store Alice session credentials in OS secure storage and never receive EVE access/refresh tokens. After explicit authorization, the Server may encrypt an EVE refresh grant with the deployment keyring for background read-only ESI sync; EVE access tokens remain memory-only, and no EVE token may enter frontend/mobile/model/telemetry/log output.

## Incident response

If a secret or infrastructure identifier reaches Git history: stop distribution, revoke/rotate it immediately, remove it from all branches/tags/releases, inspect access logs, and document the incident. Removing a line from the latest commit is not sufficient because Git history is immutable for anyone who already cloned it.
