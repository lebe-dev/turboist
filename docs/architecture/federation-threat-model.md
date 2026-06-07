# Federation Threat Model & Security Hardening

This document is the consolidated security reference for turboist federation
(Federation v1). It records the trust model, the two distinct signature planes,
the full set of Must-grade checks, and the documented v1 gaps. It is the
companion to the security regression suite in
`internal/httpapi/handlers/federation_security_suite_test.go`, which pins the
behaviours described here so they cannot silently change.

The core checks were implemented across earlier phases (transport in F0.3,
per-event payload validation in F3.2a); this document is the F6.2 tie-off and the
single place the v1 security posture — including its deliberate limitations — is
written down.

## Trust model

Federation is a **peer-to-peer overlay** between instances that have completed a
signed handshake. There is no central server and no shared account. Trust is:

- **Per-instance.** Each instance has one stable Ed25519 keypair (the seed is
  encrypted at rest via the existing `TokenCipher`, keyed by `FEDERATION_KEY`).
  An instance is identified by its `instance_url`; its public key is published at
  `GET /federation/.well-known/instance` and pinned on first contact.
- **Per-project.** A peer is granted access to an individual project with a
  permission grade (`read` / `write` / `admin`). Labels and contexts are NOT
  federated (name-matched on import); only Project + Task (+ sections, comments,
  checklists) cross instances.
- **Owner-hub (W-7).** All fan-out routes through the project owner in v1. A peer
  pushes its changes to the owner; the owner relays them to the other peers. A
  peer never talks directly to another peer.

The owner sees project plaintext (W-4: no end-to-end project encryption in v1).
TLS is a reverse-proxy concern, not enforced in-process.

## Two distinct signature planes

Every inbound federation request crosses **two independent signature checks**.
They are intentionally kept separate, because in owner-hub relay the request
signer (the owner) is **not** the author of the events it forwards.

### 1. Transport request signature (F0.3)

Authenticates the HTTP request itself — *who is talking to this instance right
now*. Implemented in `internal/httpapi/federation_signature_middleware.go` over
the helpers in `internal/federation/transport`.

The signature is Ed25519 over a single **pinned 7-line canonical string** (no
second signing scheme exists; the handshake reuses it):

```
METHOD
Request().URI().Path()        ← concrete path, NOT the Fiber route template (R4)
instance_url
timestamp
nonce
protocol_version              ← binds X-Federation-Protocol-Version (anti-downgrade)
SHA256(body)                  ← base64-std; empty body = SHA256("")
```

The signed header set is `{X-Federation-Instance, X-Federation-Timestamp,
X-Federation-Nonce, X-Federation-Protocol-Version, X-Federation-Digest}` plus the
request body. The middleware runs the Must-grade checks in this **fixed order**:

1. **Headers present** — any missing `X-Federation-*` header → `401`
   `federation_signature_invalid` (generic; the precise reason is logged
   server-side, never disclosed to the caller).
2. **Body digest** — recompute `SHA256(body)` and **constant-time compare**
   (`crypto/subtle`) against `X-Federation-Digest`; mismatch → `400`
   `federation_digest_mismatch` (US-7.2 AC2 transport leg). Checked before
   signature verification so a swapped body is rejected cheaply.
3. **±5 min timestamp window** — checked **before** the nonce (US-7.3 AC2) so a
   stale request is rejected without polluting the replay cache; stale/malformed
   → `401` `federation_timestamp_stale`.
4. **Nonce anti-replay** — a previously-seen nonce → `401` `federation_replay`
   (US-7.3 AC1). The nonce cache is a clone of `events.TicketStore`
   (`internal/federation/nonce`).
5. **Signature verify** — resolve the peer's published key via the peer-key
   cache (`.well-known` fetch-once; a *pinned* peer never auto-refetches, R5),
   then `ed25519.Verify` over the pinned canonical string. Because
   `protocol_version` is line 6, rewriting the version header in transit
   invalidates the signature (anti-downgrade, US-9.1).

A request that fails any transport check **never reaches the handler** — the
events endpoint is mounted *behind* this middleware, so there is no multi-phase
exposure window (R22). The guard is asserted by
`TestSecuritySuite_TransportSignatureRequired`.

### 2. Per-event payload validation (F3.2a)

Authenticates each **event** end-to-end across the relay — *who actually authored
this change, is the clock sane, and may this peer write here*. Implemented in
`internal/federation/inbox/validate.go`. Every check runs **before** any
`federation_inbox` or domain write, so a rejected event leaves **zero rows**
(US-7.2 AC1). The order is signature → author/origin → clock-skew → membership:

1. **Per-event Ed25519** over the canonical event-minus-signature, verified
   against the **author's** key (not the transport caller's — the owner relays a
   peer's event keeping the original author + signature). A forged/tampered/
   unsigned event, or a verified-and-rejected mismatch (genuine key rotation) →
   `401` `federation_signature_invalid`. A transient *author-key resolution*
   failure (a `.well-known` blip) is the **distinct** `ErrEventKeyUnresolved` →
   retryable `503` `federation_key_unresolved`, and must NOT be misread as a key
   rotation (it never stamps the sticky `key_mismatch` marker — F4.3 review fix).
2. **author == origin_instance** — a relay may carry the event but must never
   rewrite its author/origin; both must be present and equal, else `400`
   `federation_author_mismatch` (US-7.2 AC3).
3. **HLC clock-skew** over **every** field's HLC (not just the lexically-greatest
   one — a malformed HLC sorting below the max would otherwise poison per-field
   LWW). Bounds are **asymmetric** (US-7.2 AC4): future `> 10min` is suspicious;
   past `> 1h` is the wider window (a peer briefly offline legitimately replays
   older events). Out-of-bounds or unparseable → `400` `federation_clock_skew`.
4. **Membership + permission** — the sending peer must be a non-revoked member of
   the targeted project, direction-aware (owner→read-peer fan-out is accepted;
   read-peer→owner writes are rejected). Revoked → `403` `federation_revoked`
   (terminal); paused → `403` `federation_paused` (reversible); not-a-member /
   read-only → `403` `federation_untrusted`.

The two planes are provably independent: a transport-valid request carrying a
tampered or foreign-key-signed payload is still rejected with zero rows
(`TestSecuritySuite_TamperedPayloadNotApplied`,
`TestSecuritySuite_TransportAndPerEventSignaturesAreDistinct`).

## Invite & handshake

- Invite secrets are 256-bit, generated with `crypto/rand`, and verified with a
  **constant-time** compare (NFR-3). A wrong secret collapses to a generic `401`
  (no oracle distinguishing "no such invite" from "wrong secret", US-2.2 AC4).
- The handshake reuses the transport signing string; the owner additionally
  enforces `body.joiner_public_key == the key the request verified under` (US-2.2
  AC1 defence-in-depth) so a joiner cannot present a body key it did not sign with.
- Protocol versions are negotiated as the max intersection before the invite is
  consumed; no overlap → `400` `federation_version_unsupported`, nothing consumed
  (atomicity, US-9.1 AC2).
- A handshake from an `instance_url` already known with a different key → `409`
  `federation_key_mismatch` + WARN (US-2.2 AC5); the pinned key only changes via
  an explicit operator Trust action (R5).

## Logging & secret hygiene

- Rejections log a **generic** reason to the caller and the precise reason
  server-side at WARN. Stale/replay/digest/signature failures never disclose
  which check failed.
- Secrets are never logged: invite secrets, the encrypted key seed, JWTs, and
  Ed25519 private material are excluded from all log fields. Tokens, where logged
  at all, are masked.

## Documented v1 gaps

These are deliberate, accepted limitations. Each is recorded here (and pinned by
a regression test where applicable) so it cannot regress silently or be mistaken
for a bug.

### In-memory anti-replay resets on restart (R18 / US-7.3 AC3)

The nonce anti-replay cache and the per-peer rate-limit/incident state are
**in-memory**. On a process restart they are wiped. Consequently a request whose
nonce was rejected as a replay before a restart is **accepted once again**
immediately after the restart — a single in-window (≤ 5 min) replay is possible
per restart.

This is accepted for v1: the timestamp window still bounds the exposure to the
±5 min skew window, and a durable nonce store (or a shared cache across replicas)
is deferred. The behaviour is pinned by
`TestSecuritySuite_NonceCacheResetsOnRestart` — if the nonce store is ever made
durable, that assertion flips and this section must be updated.

Operational mitigation: minimise restarts during active federation; front the
instance with a reverse proxy that already rejects obviously stale requests where
possible.

**Nonce consumed before signature verification (deliberate).** The middleware
consumes the nonce (step 4) *before* resolving the peer key and verifying the
signature (step 5). A valid-timestamp/valid-digest request carrying a **garbage
signature** therefore burns its nonce-cache slot before the signature is checked.
This ordering is intentional, not a defect: checking the nonce first lets a replay
be rejected **cheaply**, before the step-5 key resolution — which may trigger a
`.well-known` fetch. Consuming the nonce only *after* a successful verify would let
a replayed request reach that fetch on **every** replay, turning replay into a
key-resolution amplification vector. The accepted cost — a fresh-nonce
garbage-signature probe occupies one cache slot — is bounded by the ±5 min
timestamp window (the slot expires with it) and gains the attacker nothing (the
request still fails verification). Documented at the consume site in
`internal/httpapi/federation_signature_middleware.go`.

### NFC canonical-JSON normalization (R17)

The canonical JSON used for signing (`internal/crypto/canonical.go`,
`internal/federation/events`) sorts keys and disables HTML escaping
(`SetEscapeHTML(false)`) but does **not** Unicode-NFC-normalize string values. A
strict RFC-8785 peer that normalizes differently could in principle produce
non-matching canonical bytes for visually-identical strings. v1 pins the **same**
canonicalizer on both the sign and verify side within the turboist codebase
(W-8: federation is between same-codebase instances only), so this does not
affect interop in practice. NFC normalization is a documented v1 gap.

### Other accepted scope cuts

- **Key rotation** is manual and requires a restart (the env-derived
  `FEDERATION_KEY` cannot rotate live, W-6). The `instance_url`/key pinning plus
  the `409 federation_key_mismatch` path detect an unexpected key change.
- **No realtime co-editing, no shared server, no cross-instance search**
  (W-1/W-2/W-3). Conflict resolution is per-field LWW ordered by HLC, not CRDT.
- **Owner-death "fork to local"** is v2; v1 offers a read-only fallback + queued
  edits (US-6.5 AC4 deferred).

## Test surface

| Concern | Test |
|---|---|
| Valid request through both planes | `TestSecuritySuite_ValidRequestThroughBothPlanes` |
| Replayed nonce → 401 (US-7.3 AC1) | `TestSecuritySuite_ReplayedNonceRejected` |
| Stale timestamp → 401, ordered before nonce (US-7.3 AC2) | `TestSecuritySuite_StaleTimestampRejectedBeforeNonce` |
| Restart replay window (US-7.3 AC3, R18) | `TestSecuritySuite_NonceCacheResetsOnRestart` |
| Digest mismatch → 400 (US-7.2 AC2) | `TestSecuritySuite_DigestMismatchRejected` |
| Endpoint unreachable without transport plane (R22) | `TestSecuritySuite_TransportSignatureRequired` |
| Event sig fail → 401, inbox count 0 (US-7.2 AC1) | `TestSecuritySuite_EventSignatureFailLeavesZeroRows` |
| Tampered payload not applied (§15.5) | `TestSecuritySuite_TamperedPayloadNotApplied` |
| Author/origin mismatch → 400 (US-7.2 AC3) | `TestSecuritySuite_AuthorOriginMismatchRejected` |
| Clock-skew boundaries (US-7.2 AC4) | `TestSecuritySuite_ClockSkewBoundaries` |
| Two planes distinct | `TestSecuritySuite_TransportAndPerEventSignaturesAreDistinct` |
| Per-event payload checks (unit) | `internal/federation/inbox/validate_test.go` |
| Transport middleware checks (unit) | `internal/httpapi/federation_signature_middleware_test.go` |
| Validate-then-apply zero-rows (DB) | `internal/federation/inbox/validate_apply_test.go` |
