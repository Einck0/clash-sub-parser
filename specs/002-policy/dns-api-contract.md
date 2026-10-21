# DNS Settings API and Export Contract

Change: `002-policy`  
Run ID: `pi_run_20261020_204735_990606`  
Date: 2026-10-20  

---

## 1. HTTP API Specifications

All endpoints are registered under `/api/v1` and protected by standard administrative session/token authentication.

### 1.1 GET `/api/v1/settings/dns`

Retrieve current DNS settings and effective parsed configuration map.

- **Request Headers**: `Authorization: Bearer <token>` or session cookie
- **Status Codes**:
  - `200 OK`: Settings returned successfully
  - `503 Service Unavailable`: Repository not configured (`service_unavailable`)
  - `401 Unauthorized`: Authentication missing or invalid

#### Response Format (`200 OK`)
```json
{
  "data": {
    "enabled": true,
    "raw_yaml": "enable: true\nnameserver:\n  - 1.1.1.1\n  - 8.8.8.8\n",
    "revision": 0,
    "updated_at": null,
    "source": "default",
    "config": {
      "enable": true,
      "nameserver": ["1.1.1.1", "8.8.8.8"]
    }
  }
}
```

*Notes*:
- When no saved record exists in `dns_config`, `revision` is `0`, `source` is `"default"`, and `updated_at` is `null`.
- When persisted in database, `source` is `"saved"`, `revision` is `>= 1`, and `updated_at` is RFC3339 timestamp string.

---

### 1.2 PUT `/api/v1/settings/dns`

Atomically update DNS settings with optimistic concurrency revision check.

- **Request Body Limit**: Max 128 KiB
- **Status Codes**:
  - `200 OK`: Updated successfully, returns updated settings and parsed config
  - `400 Bad Request`: Malformed JSON payload
  - `409 Conflict`: Revision mismatch (`dns_revision_conflict`), returns `current_revision`
  - `422 Unprocessable Entity`: Invalid YAML, nesting > 64, alias cycle, duplicate key, missing `enabled`, or negative `expected_revision`
  - `503 Service Unavailable`: Repository not configured (`service_unavailable`)

#### Request Payload
```json
{
  "enabled": true,
  "raw_yaml": "enable: true\nnameserver:\n  - 1.1.1.1\n  - 8.8.4.4\nenhanced-mode: fake-ip\n",
  "expected_revision": 0
}
```

#### Conflict Response (`409 Conflict`)
```json
{
  "code": "dns_revision_conflict",
  "message": "DNS settings revision mismatch",
  "current_revision": 1,
  "details": {
    "current_revision": 1
  }
}
```

#### Success Response (`200 OK`)
```json
{
  "data": {
    "enabled": true,
    "raw_yaml": "enable: true\nnameserver:\n  - 1.1.1.1\n  - 8.8.4.4\nenhanced-mode: fake-ip\n",
    "revision": 1,
    "updated_at": "2026-10-20T17:30:00Z",
    "source": "saved",
    "config": {
      "enable": true,
      "nameserver": ["1.1.1.1", "8.8.4.4"],
      "enhanced-mode": "fake-ip"
    }
  }
}
```

---

### 1.3 POST `/api/v1/settings/dns/validate`

Pure validation and normalization endpoint without database side-effects.

- **Request Headers**: Admin authentication
- **Request Body**:
```json
{
  "enabled": true,
  "raw_yaml": "enable: true\nnameserver:\n  - 1.1.1.1\n"
}
```
- **Status Codes**:
  - `200 OK`: YAML is valid, returns normalized YAML representation and parsed map
  - `422 Unprocessable Entity`: YAML syntax or schema error

#### Response Format (`200 OK`)
```json
{
  "data": {
    "raw_yaml": "enable: true\nnameserver:\n  - 1.1.1.1\n",
    "config": {
      "enable": true,
      "nameserver": ["1.1.1.1"]
    }
  }
}
```

---

## 2. Downstream Publication and Compiler Integration

1. **Publication Service**:
   - `AssembleResolveInput`: Fallback to `domain.DefaultDNSSettings()` only when `dnsRepo` is unconfigured (`nil`). If `dnsRepo` is present and returns an error, the call fails immediately with domain error `dns_settings_unavailable` (fail-closed, never masking errors with default).
   - `BuildCurrentArtifact`: Consumes `facts.DNSSettings` loaded transactionally by `ConsistentFactsLoader`. Replaces hardcoded defaults.
   - Converts `DNSSettings` to `resolver.DNSConfig{ Enabled: s.Enabled, Config: parsedMap }`.

2. **Mihomo Compiler**:
   - `mihomoConfig` includes `DNS map[string]any `yaml:"dns,omitempty"``.
   - `renderMihomoDNS` preserves all known and unknown keys from `snapshot.DNS.Config` (e.g. `enhanced-mode`, `fake-ip-filter`, `use-hosts`, `nameserver-policy`).
   - If `snapshot.DNS.Enabled` is `false`, renders `enable: false` in YAML without fabricating `enable: true`.
   - Zero-value `snapshot.DNS` (legacy test fixtures) omits `dns:` block cleanly, maintaining golden fixture regression safety.

3. **Digest Stability**:
   - `computeInputDigest` and `computeSnapshotDigest` include `DNSConfig` in canonical serialization.
   - Any modification to `DNSConfig.Enabled`, `Nameservers`, or custom fields in `Config` directly alters the SHA-256 snapshot digest.

---

## 3. Verification Artifacts

- **Runner**: Docker container `golang:1.27.1-alpine`
- **Cache**: `/tmp/csp-808995/go-build`
- **Test Log**: `/tmp/csp-808995/test-dns-execution.log`
- **Log SHA-256**: `fd40dfd26f14336747d891fb2f0bd2e1b837204252f4043f27f25374db9cbcf5`
- **Exit Code**: `0`
