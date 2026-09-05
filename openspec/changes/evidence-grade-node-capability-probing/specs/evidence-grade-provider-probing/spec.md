## Purpose

Defines a credential-free and auditable probe catalogue for streaming, AI, exit-identity, and CDN-routing checks so an operator can distinguish verified availability from a generic reachable page, provider challenge, or obsolete provider contract.

## ADDED Requirements

### Requirement: Evidence-grade provider result contract
The system SHALL return one normalized result for every selected provider with `status`, `verdict`, `checked_at`, `evidence_version`, and a bounded `evidence` object. `status` MUST be one of `verified`, `partial`, `restricted`, `ip_blocked`, `challenged`, `rate_limited`, `timeout`, `transport_error`, `inconclusive`, or `disabled`. `evidence` MAY expose only non-secret request and response observations: HTTP status, final hostname and path, redirect classification, declared region, response signal identifiers, elapsed milliseconds, and a sanitized error code. It MUST NOT expose authorization headers, cookies, request body credentials, proxy credentials, full response bodies, or a source node's private parameters.

#### Scenario: Provider response proves availability
- **WHEN** a selected provider returns every positive signal defined by its active catalogue contract through the candidate node
- **THEN** the result has `status: verified`, a provider-specific availability `verdict`, the observed region when available, and only the bounded observations that support that conclusion

#### Scenario: Generic successful HTTP response is insufficient
- **WHEN** a provider returns HTTP 200 but none of the positive or negative contract signals match
- **THEN** the result has `status: inconclusive` and SHALL NOT be represented as unlocked or verified

### Requirement: Credential-free and pinned egress probing
Every identity, streaming, AI, and CDN-routing request SHALL use the candidate node's isolated loopback proxy and SHALL disable inherited environment proxy configuration. The active provider catalogue SHALL not require customer accounts, private tokens, static session cookies, copied browser credentials, or opaque request bodies containing secrets.

#### Scenario: Host proxy is configured
- **WHEN** HTTP proxy environment variables are set on the probe host while a candidate node is tested
- **THEN** the recorded provider and identity observations originate only from the candidate node's isolated proxy path and no direct host-egress fallback result is emitted

#### Scenario: A provider would require a secret
- **WHEN** a proposed platform probe cannot produce a reliable classification without a credential or secret-bearing session artifact
- **THEN** that provider is disabled or classified as unsupported rather than adding the secret to configuration, code, logs, diagnostics, or frontend state

### Requirement: Provider-specific classification and contract drift quarantine
The system SHALL classify positive availability, partial catalogue access, region restriction, IP reputation block, challenge, rate limit, timeout, and transport failure separately. Each active provider contract SHALL carry a version and deterministic fixture coverage for its positive, restricted, challenge/rate-limit, and unrecognized-response paths. An unrecognized or inconsistent response MUST result in `inconclusive` and SHALL NOT silently become a verified unlock.

#### Scenario: Netflix catalogue classification
- **WHEN** the Netflix provider observes both its configured non-original catalogue signal and its original-catalogue signal
- **THEN** the result has `status: verified` with `verdict: full`

#### Scenario: Netflix originals-only classification
- **WHEN** the Netflix provider observes its configured original-catalogue signal but not its non-original catalogue signal
- **THEN** the result has `status: partial` with `verdict: originals_only` and SHALL NOT be treated as a full unlock

#### Scenario: Provider contract changes
- **WHEN** a provider returns a response that matches neither the versioned positive nor negative fixture signals
- **THEN** the result has `status: inconclusive`, carries the catalogue version and sanitized signal summary, and the UI marks it as requiring a catalogue review rather than as blocked or unlocked

### Requirement: Identity consensus and CDN-routing are distinct observations
The system SHALL collect exit identity only when at least two independent identity providers agree on the outbound IP and ISO country code through the tested node. Agreement, disagreement, and unavailable-provider outcomes SHALL be retained in bounded identity evidence. CDN-routing observations SHALL be reported as CDN routing hints and MUST NOT overwrite or contradict the verified egress identity.

#### Scenario: Identity providers agree
- **WHEN** two independent identity providers report the same outbound IP and country through the candidate node
- **THEN** the probe records the IP, country, and agreement evidence as verified identity

#### Scenario: Identity providers disagree
- **WHEN** successful identity providers return inconsistent outbound IP or country observations
- **THEN** the identity confidence is `conflicted`, the node result SHALL NOT claim a verified country or ASN consensus, and media results remain independently visible

#### Scenario: YouTube CDN differs from exit country
- **WHEN** a YouTube mapping probe reports a CDN routing location different from the verified exit country
- **THEN** the UI presents the mapping as a routing hint and retains the verified exit country as the node's geographic identity

### Requirement: Independent deadlines and bounded retry discipline
Each provider SHALL obey the existing configured service deadline as a total deadline across its request and permitted fallback probes. A provider MAY make at most one bounded retry only for transient transport failures before the deadline expires; it MUST NOT retry a definitive restriction, IP block, challenge, rate limit, or contract-drift result. A provider timeout or failure SHALL NOT alter the liveness verdict of a node whose transport handshake succeeded.

#### Scenario: One provider times out
- **WHEN** one selected provider exceeds its service deadline after node transport succeeds
- **THEN** only that provider reports `status: timeout`, its resources are released, and all completed sibling provider and node observations remain available

#### Scenario: Definitive restriction is returned
- **WHEN** a provider returns a configured regional-restriction signal
- **THEN** the system records `status: restricted` without retrying the same provider request and without marking the node transport as failed
