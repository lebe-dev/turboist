# Federation

Federation lets two Turboist instances share a project in real time. The owner instance holds the project; the joiner gets a live, editable copy. Changes push within seconds via a signed event stream (Ed25519); a pull loop catches up after a short outage.

## Prerequisites

- Both instances must be publicly reachable by each other over HTTPS (or HTTP for local testing).
- `BASE_URL` must be set to each instance's actual public URL — this is the federation identity.

## Step 1 — Enable federation on both instances

Add `FEDERATION_KEY` to each instance's environment:

```sh
# Generate once per instance — keep secret, back it up
openssl rand -hex 32
```

```env
# .env (or container environment)
FEDERATION_KEY=<64-char hex string>
```

Restart both instances. Federation endpoints are disabled when this variable is unset; all `/federation/*` routes return `403 federation_key_missing` without it.

**Important:** rotating `FEDERATION_KEY` makes the instance's existing Ed25519 keypair unreadable. Peers that trusted the old public key will reject its events until re-paired.

## Step 2 — Enable federation on the project (owner side)

In the owner's UI, open the project settings and enable federation. This marks the project as federated and generates the owner's instance keypair on first use (the keypair is stored encrypted at rest using `FEDERATION_KEY`).

Alternatively, via API:

```http
POST /api/v1/projects/{id}/federation/enable
Authorization: Bearer <jwt>
```

## Step 3 — Create an invite (owner side)

In the owner's UI, go to the federated project → **Sharing** → **Create invite**.

Choose permissions:
- `read` — joiner sees tasks, cannot edit
- `write` — joiner can create and edit tasks
- `admin` — write + can invite other peers

Defaults: single-use, expires in 7 days.

Via API:

```http
POST /api/v1/projects/{id}/invites
Authorization: Bearer <jwt>
Content-Type: application/json

{
  "permissions": "write",
  "max_uses": 1
}
```

Response includes a **one-time** `link` in the form:

```
https://owner.example.com/federation/join#invite=<id>.<secret>
```

The secret is in the URL fragment — it is never sent to the server in the HTTP request line and never logged. **Copy it once; it cannot be retrieved again.**

## Step 4 — Join (joiner side)

Open the link in the joiner's browser, or paste it into the joiner's UI under **Federation → Join**.

The joiner's instance calls the owner server-to-server (the secret never travels browser → owner). On success, the federated project appears in the joiner's project list and an initial snapshot is downloaded.

Via API (joiner instance):

```http
POST /api/v1/federation/join
Authorization: Bearer <jwt>
Content-Type: application/json

{
  "owner_instance_url": "https://owner.example.com",
  "invite_id": "<id>",
  "secret": "<secret>"
}
```

## Sync behaviour

After joining, changes flow automatically:

- **Push** — the owner publishes a signed event within ~1 s of each mutation; the joiner applies it via LWW (last-writer-wins per field).
- **Pull backstop** — a recovery loop re-pulls from the last received HLC cursor every 60 s (configurable: `pull-interval-seconds`). A peer that was offline for less than 30 days auto-catches-up on reconnect.
- **Offline editing (write/admin joiners)** — edits queue locally in the outbox and flush + LWW-resolve when the owner becomes reachable again. The UI shows a *"pending — owner offline"* badge after 30 days of owner silence (configurable: `owner-timeout-days`).

## Monitoring

### Sync status badge

`GET /api/v1/federation/status` (JWT) — one entry per federated project:

| status | meaning |
|---|---|
| `synced` | all peers up to date |
| `pending` | events queued, not yet delivered |
| `unreachable` | at least one peer is offline |
| `key_mismatch` | peer rotated its key — see [Key mismatch](#key-mismatch) |

### Liveness probe (no auth)

```
GET https://your-instance.example.com/federation/health
```

Returns `status: ok | degraded | peers_stale`, `uptime_s`, and `outbox_depth`.

### Detailed health (JWT)

```
GET /api/v1/federation/health
```

Same as the public probe plus per-peer `last_contact_at` and status.

### Federation overview

```
GET /api/v1/federation/overview
```

Lists every federated project with this instance's role (`owner` / `peer` / `read-only`) and the named peer audience.

## Peer management

All routes require a valid JWT.

| Action | Endpoint |
|---|---|
| List peers | `GET /api/v1/projects/{id}/federation/peers` |
| Pause a peer | `POST /api/v1/projects/{id}/federation/peers/pause` |
| Resume a peer | `POST /api/v1/projects/{id}/federation/peers/resume` |
| Revoke a peer | `DELETE /api/v1/projects/{id}/federation/peers` |
| Leave (joiner) | `POST /api/v1/projects/{id}/federation/leave` |

Pause/resume/revoke take `{"instance_url": "https://peer.example.com"}` in the body. Revoke is **irreversible** — the peer loses access permanently and receives a signed `federation_revoke` control event.

## Key mismatch

If a peer rotated its `FEDERATION_KEY` (and therefore its Ed25519 keypair), this instance will reject its events with `401` and record a sticky `key_mismatch` incident. To re-trust the peer after verifying the rotation is genuine:

```http
POST /api/v1/projects/{id}/federation/peers/trust-key
Authorization: Bearer <jwt>
Content-Type: application/json

{"instance_url": "https://peer.example.com"}
```

The server fetches the peer's current `GET /federation/.well-known/instance`, overwrites the pinned key, clears the incident, and writes an audit entry.

## Audit log

```
GET /api/v1/federation/audit?peer=https://peer.example.com&limit=50
```

Security-relevant events (handshake, revoke, signature failures, replay attempts, clock-skew rejections) newest-first. Also surfaces `alerts` when signature failures from one peer exceed the threshold (default: 10 failures in 60 min).

## Backup

A federation-aware physical backup includes the keypair, so a restored instance keeps its identity without re-pairing:

```
GET /api/v1/federation/backup
```

Downloads a full `VACUUM INTO` SQLite snapshot as a `.db` file.

## Advanced configuration

All tuning lives in `config.yml` under the `federation:` block (see `config.example.yml`). Defaults are safe and need no changes for a standard two-instance setup. Notable knobs:

| Key | Default | Purpose |
|---|---|---|
| `publish-interval-seconds` | 60 | Catch-up backstop tick (push is near-instant) |
| `pull-interval-seconds` | 60 | Recovery pull loop interval |
| `owner-timeout-days` | 30 | Days of owner silence before "owner offline" badge |
| `inbound-rate-per-minute` | 600 | Per-peer event rate limit before 429 |
| `tombstone-retention-days` | 90 | Deleted entity GC horizon (peer offline > this → re-snapshots) |
| `outbox-retention-days` | 30 | Outbox replay buffer (hard-capped at 30 d) |
| `audit-retention-days` | 365 | Audit log retention |
