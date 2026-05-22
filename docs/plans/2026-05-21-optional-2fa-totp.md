# Optional TOTP 2FA

## Overview

Internal TOTP (RFC 6238) two-factor authentication, per-user opt-in. After password check passes, if TOTP is enabled the API returns a short-lived `awaiting_otp` ticket; the frontend prompts for a 6-digit code; verification issues the JWT/refresh pair. Setup shows a QR code plus 8 single-use recovery codes. Secrets are stored encrypted at rest with a dedicated `TOTP_SECRET_KEY`.

## Context

- Files involved:
  - Backend config: `internal/config/*.go`, `.env.example`
  - Crypto: `internal/service/calendar/crypto.go` → moved to `internal/crypto/cipher.go`; callers in `internal/service/calendar/*` and `internal/repo/calendars.go`
  - Migrations: `internal/db/migrations/019_totp.sql` (new)
  - Repo: `internal/repo/users.go`, new `internal/repo/totp.go`
  - Service: new `internal/service/totp/` (setup, verify, recovery codes)
  - HTTP API: `internal/httpapi/handlers/auth.go` (login flow), new `internal/httpapi/handlers/totp.go` (`/auth/totp/*`), `internal/httpapi/dto/auth.go`, `internal/httpapi/server.go`
  - Frontend: `frontend/src/lib/api/endpoints/auth.ts`, new `frontend/src/lib/api/endpoints/totp.ts`, new `frontend/src/lib/components/settings/TwoFactorSection.svelte`, `frontend/src/routes/(app)/settings/+page.svelte`, `frontend/src/routes/(auth)/login/+page.svelte`, `frontend/src/lib/auth/store.svelte.ts`
  - i18n: `frontend/src/lib/i18n.ts` (en + ru strings)
  - Docs: `README.md`, `files/files/API.md`, `docs/architecture/backend.md`
- Related patterns: AES-256-GCM `TokenCipher` (`enc:v1:` prefix); `repo.UserRepo` style; `auth.IPLimiter` for rate limiting; Fiber v3 handler style in `auth.go`
- Dependencies: `github.com/pquerna/otp` (TOTP + QR code), already-used Argon2id helpers for hashing recovery codes

## Development Approach

- Testing approach: Regular (code first, then tests). Backend gets unit + handler tests in the existing style (`*_test.go` siblings).
- Complete each task fully before moving to the next.
- **CRITICAL: every task MUST include new/updated tests.**
- **CRITICAL: all tests must pass (`just test-backend` / `just test-frontend`) before starting next task.**
- Use `just lint` before finishing.

## Implementation Steps

### Task 1: Extract shared crypto package and add TOTP_SECRET_KEY config

**Files:**
- Create: `internal/crypto/cipher.go` (move `TokenCipher`, `EncryptedTokenPrefix`, `NewTokenCipher`)
- Create: `internal/crypto/cipher_test.go` (port existing tests)
- Modify: `internal/service/calendar/crypto.go` (delete or thin re-export)
- Modify: all calendar callers to import `internal/crypto`
- Modify: `internal/config/config.go`, `internal/config/config_test.go`, `.env.example`

- [x] move `TokenCipher` to `internal/crypto`, keep `enc:v1:` wire format unchanged
- [x] update calendar imports/usages; remove the old file
- [x] add `TOTPSecretKey` to config loader, validate ≥32 bytes (new env `TOTP_SECRET_KEY`; accept empty and treat as "feature disabled" if empty; document in `.env.example`)
- [x] write/port tests for cipher and config validation
- [x] run `just test-backend` and `just lint-backend`

### Task 2: Database migration and user model

**Files:**
- Create: `internal/db/migrations/019_totp.sql`
- Modify: `internal/model/user.go` (or wherever `User` lives), `internal/repo/users.go`, `internal/repo/users_test.go`

- [x] migration adds to `users`: `totp_secret TEXT NOT NULL DEFAULT ''`, `totp_enabled INTEGER NOT NULL DEFAULT 0`, `totp_enabled_at TEXT`
- [x] migration creates `totp_recovery_codes(id, user_id, code_hash, used_at, created_at)` with FK + index on `(user_id, used_at)`
- [x] extend `model.User` and `scanUser`/insert/update with new columns
- [x] repo methods: `SetTOTPSecret`, `EnableTOTP`, `DisableTOTP`, `GetTOTPState`
- [x] tests cover migration roundtrip and repo methods
- [x] run `just test-backend`

### Task 3: TOTP service and recovery codes

**Files:**
- Create: `internal/service/totp/totp.go`, `internal/service/totp/totp_test.go`
- Create: `internal/repo/totp_recovery.go`, `internal/repo/totp_recovery_test.go`
- Modify: `go.mod`, `go.sum` (add `github.com/pquerna/otp`)

- [x] `Service` wraps cipher + repo; methods: `BeginSetup` (generate secret, return `otpauth://` URL and QR PNG bytes), `ConfirmSetup(code)` (verifies, encrypts secret, persists, generates 8 recovery codes, returns plaintext codes once), `Verify(code)`, `ConsumeRecoveryCode(code)`, `Disable(code)`
- [x] recovery codes: generated as 10-char base32 strings, stored as Argon2id hash, single-use (mark `used_at`)
- [x] use issuer "Turboist" + username in `otpauth://`
- [x] unit tests with frozen time and a known secret; cover replay protection (reject same code twice within window), recovery code consumption, disable flow
- [x] run `just test-backend`

### Task 4: HTTP API — TOTP management endpoints

**Files:**
- Create: `internal/httpapi/handlers/totp.go`, `internal/httpapi/handlers/totp_test.go`
- Modify: `internal/httpapi/dto/auth.go`, `internal/httpapi/server.go`, `internal/httpapi/errors.go`, `files/files/API.md`

- [x] routes (JWT-protected): `POST /auth/totp/setup` (returns `secret`, `otpauthUrl`, `qrPngBase64`), `POST /auth/totp/confirm` (`{code}` → `{recoveryCodes:[]}`), `POST /auth/totp/disable` (`{code}` accepts TOTP or recovery)
- [x] add error codes: `totp_invalid_code`, `totp_already_enabled`, `totp_not_enabled`
- [x] reuse `auth.IPLimiter` on confirm/disable
- [x] handler tests covering happy path + each error case
- [x] update `API.md`
- [x] run `just test-backend`

### Task 5: Login flow with awaiting-OTP state

**Files:**
- Modify: `internal/httpapi/handlers/auth.go`, `internal/httpapi/handlers/auth_test.go`, `internal/httpapi/dto/auth.go`, `internal/auth/jwt.go` (or new helper for OTP ticket)

- [x] after password check, if user has TOTP enabled: return `200 {"otpRequired": true, "ticket": "<signed JWT>"}` instead of `AuthResponse`; ticket is short-lived (5 min), single-purpose claim, signed with existing JWT secret
- [x] new endpoint `POST /auth/login/otp` (`{ticket, code}`) — verifies ticket + TOTP/recovery code, then issues normal `AuthResponse`
- [x] IP rate-limit on `/auth/login/otp`; on failure log and apply same backoff as login
- [x] update handler tests: login without 2FA (unchanged), login with 2FA (two-step), wrong OTP, expired ticket, recovery-code login
- [x] update `API.md`
- [x] run `just test-backend` and `just lint-backend`

### Task 6: Frontend — settings UI

**Files:**
- Create: `frontend/src/lib/api/endpoints/totp.ts`, `frontend/src/lib/components/settings/TwoFactorSection.svelte`
- Modify: `frontend/src/routes/(app)/settings/+page.svelte`, `frontend/src/lib/i18n.ts`
- Tests: `frontend/src/lib/components/settings/TwoFactorSection.test.ts` (if test scaffolding exists; otherwise component-level vitest)

- [x] API client wrappers for `setup`, `confirm`, `disable`
- [x] `TwoFactorSection.svelte`: shows status (enabled/disabled). Enable flow: button → QR + secret + code input → confirm → show recovery codes once (download `.txt` + copy buttons). Disable flow: code input → confirm
- [x] add EN + RU strings to `i18n.ts`
- [x] frontend tests for the component states (disabled→setup→confirmed; disable flow)
- [x] run `just test-frontend`

### Task 7: Frontend — login OTP prompt

**Files:**
- Modify: `frontend/src/routes/(auth)/login/+page.svelte`, `frontend/src/lib/api/endpoints/auth.ts`, `frontend/src/lib/auth/store.svelte.ts`, `frontend/src/lib/auth/store.test.ts`, `frontend/src/lib/i18n.ts`

- [x] update `login` API call to handle `otpRequired` response and call new `loginOtp(ticket, code)` endpoint
- [x] login page: if `otpRequired`, swap form to OTP input + "use recovery code" link; show error on invalid/expired
- [x] keep ticket only in memory (not in localStorage)
- [x] tests for both branches of `auth/store`
- [x] run `just test-frontend`

### Task 8: Verify acceptance criteria

- [x] `just test` (full suite — backend + frontend)
- [x] `just lint`
- [x] manual test (skipped - not automatable): `just dev` walk-through of enable 2FA → log out → log in with TOTP → log in with recovery code → disable 2FA

### Task 9: Update documentation

- [x] `README.md`: add 2FA to feature list, document `TOTP_SECRET_KEY`
- [x] `docs/architecture/backend.md`: note new package layout (skipped — file does not exist in repo)
- [x] `files/files/API.md`: already updated in Tasks 4–5, double-check (verified: TOTP routes, error codes, and OTP login flow documented in `API.md`)
- [x] add short note to `CLAUDE.md` if any new conventions emerged (skipped — no project-level `CLAUDE.md`; shared crypto convention is self-documenting via package location)
