# Verification

1. Master exact-scope commit/push dev; use existing .github/workflows/ci.yml. Never run CSP tests/build on host/local Docker.
2. Verify same-SHA checks and runtime build, image digest and OCI revision. Release skipped on dev is expected.
3. Pull prebuilt image; fresh synthetic DB on internal Docker network, loopback port, disabled subscription/probe schedulers, no production mounts or external fetch/probe. Record exact resources and cleanup owner.
4. Use synthetic mixed SS/VLESS/target-invalid VMess fixture. Verify direct aliases, compatible exclusions and nonblank output; strict preview errors; all-excluded rejection; non-Mihomo active chains rejection; Mihomo full rules/DNS/plugins preserved. Verify preview real ID -> publish -> immutable download, live/check agreement, auth/ETag and userinfo.
5. Independent reviewer reads source/cloud evidence and reproduces isolated endpoint checks. Surge/QX grammar validation is not proprietary-app import proof. Clean only owned resources/secrets; verify absence explicitly.
6. Record final evidence/aggregate and notify user; no production deployment authorized.
