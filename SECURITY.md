# Security Policy

## Supported versions

This project is currently pre-1.0 for public open-source use. Security fixes should target the default branch unless maintainers document release branches later.

## Reporting a vulnerability

Please report security issues privately to the project maintainer instead of opening a public issue.

Include:

- A short description of the issue.
- Reproduction steps.
- Affected configuration or endpoint.
- Whether real subscription URLs, tokens, or generated configs are involved.

Do not include real subscription tokens or private node credentials in public reports.

## Sensitive data guidelines

This project may process highly sensitive data:

- subscription URLs and tokens
- proxy node credentials
- generated Clash YAML / Script.js
- local database files
- upstream response headers containing traffic and expiry information

Before publishing logs, screenshots, issues, or examples, redact:

- full subscription URLs
- proxy credentials such as passwords, UUIDs, private keys
- local/private IP addresses if they identify your infrastructure
- generated configs containing real nodes

## Deployment notes

- Do not expose the management UI directly to the public Internet without authentication or a trusted reverse proxy.
- Keep `.env` private.
- Use `CLASH_REQUEST_TRUST_ENV=true` only when you intentionally want subscription fetching to inherit proxy environment variables.
- Treat the SQLite database volume as sensitive runtime data.
