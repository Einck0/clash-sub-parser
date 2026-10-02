## 1. Complete preparation feature

- [x] 1.1 Preserve current baseline; read-only managed credential discovery, current node/source/audit classification, backup/rollback verification; prepare redacted acceptance and exact recovery plan without production writes.
- [x] 1.2 Implement necessary semantic acceptance/recovery tooling or minimal fixes with deterministic temporary DB/isolated fixtures; run go build ./..., relevant full Go tests and required frontend gate if changed; record exit0.

## 2. Production precondition gate

- [x] 2.1 Fresh reviewer independently verifies final code, safety, causal classification and bounded production plan before writes.

## 3. Authorized live closure

- [x] 3.1 After reviewer PASS, immediate hot backup and execute legally authenticated bounded live response-body audit, existing subscriptions refresh and representative real node handshake; preserve old inventory on failure.
- [x] 3.2 Restore only evidence-qualified reviewed allowlist; report actual count and all non-restored categories without hiding ambiguity. If evidence unavailable, retain unchecked external dependency and explicit user decision.
- [x] 3.3 Independent reviewer validates live evidence, safety and final remaining conditions; necessary Git commit and deployment only for reviewed actual changes.
