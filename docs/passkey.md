# Passkeys (WebAuthn)

A passkey signs you in with the device you already unlock — Face ID, Touch ID,
Windows Hello, an Android screen lock, or a hardware security key — instead of
typing a password.

In Turboist a passkey is an **additional** login method, never a replacement:

- The password stays as the recovery path. Deleting your last passkey is allowed
  and cannot lock you out.
- A passkey login satisfies **both factors at once** — the authenticator holds
  the key *and* verifies you — so it skips the TOTP step even on an account with
  2FA enabled.
- Credentials are **discoverable**: the login screen has no username field. The
  authenticator reports which account it holds and the server resolves the rest.

The web app works with no configuration beyond HTTPS. The native apps need a
one-time setup, described below, because the operating system will not hand an
app a credential for a domain until that domain vouches for the app.

## Web setup

Nothing to configure. On start-up the server derives its WebAuthn Relying Party
from `BASE_URL` and logs the result:

```
INFO passkeys enabled rp_id=todo.example.com origins=[https://todo.example.com]
```

Two requirements:

- **A secure context.** WebAuthn runs only over HTTPS, or on `localhost` for
  development. A plain-HTTP LAN address (`http://192.168.1.10:8080`) cannot use
  passkeys in any browser.
- **A stable Relying Party ID.** Credentials are bound to it. Changing the RP ID
  — including moving the app to a different hostname — invalidates every
  registered passkey, and users have to enrol again with their password.

If the Relying Party cannot be configured, the feature disables itself (logged at
WARN) and `/auth/passkey/*` plus `/api/v1/passkeys/*` return `404`. Password
login is unaffected — a bad passkey configuration never takes the server down.

### Optional environment variables

| Variable | Purpose |
|---|---|
| `WEBAUTHN_RP_ID` | Overrides the RP ID. Set it only to bind credentials to a parent domain — e.g. `example.com` while the app runs on `todo.example.com`, so the same passkeys keep working if you later move the app to another subdomain. It must be the app's domain or a parent of it. |
| `WEBAUTHN_ORIGINS` | Extra allowed ceremony origins, comma-separated. The origin from `BASE_URL` is always allowed. Needed for the Android app (see below). |
| `WELL_KNOWN_PATH` | Directory served under `/.well-known/`. Holds the association files the native apps need. Leave empty if a reverse proxy already serves those paths. |

See [configuration.md](configuration.md) for the full variable list.

## Using passkeys

**Add one** in Settings → Security → Passkeys, while signed in. The platform
prompts, and the passkey is stored on that device (or in its password manager).
Name it after the device — the name is only a label and can be edited later.

**Sign in** with the *Sign in with a passkey* button on the login screen. It
appears only when the browser supports WebAuthn *and* the instance has at least
one passkey registered, so a fresh install shows nothing until you enrol.

Up to **20 passkeys** per account.

### One passkey is not automatically on every device

Where a passkey lives depends on where it was created:

| Created with | Available on |
|---|---|
| iCloud Keychain (Safari / iOS / macOS) | Every Apple device on the same Apple ID |
| Google Password Manager (Chrome / Android) | Every device signed into that Google account |
| A platform authenticator with no sync (some Windows Hello setups) | That one device |
| A hardware security key | Wherever you plug it in |

A passkey created with Touch ID on a Mac will **not** appear on an Android phone.
On the new device either sign in with the password once and add a second passkey
there, or use the platform's cross-device flow (scan a QR code with the phone
that holds the credential).

## Native app setup (iOS / Android)

Unlike everything else on native, this is **build-time** configuration: iOS bakes
the associated-domains entitlement in at sign time, and Android needs
asset-statements metadata in the manifest. A native build therefore targets one
instance domain. Skip this section entirely and the apps sign in with a password
as before.

### 1. Point the build at your domain

In `frontend/capacitor.config.ts`:

```ts
CapacitorPasskey: {
    origin: 'https://todo.example.com',
    domains: ['todo.example.com'],
    autoShim: true
}
```

Then `just mobile`. The plugin's sync hook writes the iOS entitlement and the
Android asset-statements metadata into the generated host projects. Confirm it
ran — `npx cap sync android` must list `@capgo/capacitor-passkey` among the
plugins, and `frontend/android/app/src/main/res/values/capacitor-passkey.xml`
must point at your domain.

### 2. Serve the association files

Both platforms fetch a file from the Relying Party's domain to confirm the app
may use its credentials. Put them in `deploy/well-known/` and set
`WELL_KNOWN_PATH=/app/well-known`; `docker-compose.yml` already mounts that
directory into the container. A reverse proxy serving the same two paths works
just as well.

**`/.well-known/assetlinks.json`** (Android) — needs the
`delegate_permission/common.get_login_creds` relation and the SHA-256 fingerprint
of the certificate that signed the **installed** APK:

```json
[
  {
    "relation": [
      "delegate_permission/common.handle_all_urls",
      "delegate_permission/common.get_login_creds"
    ],
    "target": {
      "namespace": "android_app",
      "package_name": "ru.tinyops.turboist",
      "sha256_cert_fingerprints": ["91:99:F3:…"]
    }
  }
]
```

Where the fingerprint comes from:

```sh
# just deploy-android builds assembleDebug -> the local debug keystore signs it
keytool -list -v -alias androiddebugkey -keystore ~/.android/debug.keystore \
    -storepass android | grep SHA256
```

A debug keystore is per-machine, so building on a second machine means adding
that fingerprint too. For a Play Store build take the fingerprint from Play
Console → *App integrity* → *App signing key certificate*, since Play re-signs
the upload.

**`/.well-known/apple-app-site-association`** (iOS) — served as JSON with **no**
file extension:

```json
{ "webcredentials": { "apps": ["TEAMID.ru.tinyops.turboist"] } }
```

Verify both are actually served, and look at the **body**: a SPA answers any
unknown path with `200` and `index.html`, which passes a status-code-only check.

```sh
curl https://todo.example.com/.well-known/assetlinks.json
```

The server also reports what it found at start-up:

```
INFO well-known files served path=/app/well-known files=[assetlinks.json apple-app-site-association]
```

A `WARN` there (directory missing, empty, or unreadable) is the answer to a `404`
on those paths. The container runs rootless as uid 10001, so the files must be
world-readable (`chmod 644`).

### 3. Allow the Android app origin

Android Credential Manager is not a browser: it reports
`android:apk-key-hash:<base64url-sha256>` in `clientDataJSON` rather than your
HTTPS origin. Convert the fingerprint and add it to the server:

```sh
python3 -c 'import base64,sys; print("android:apk-key-hash:"+base64.urlsafe_b64encode(bytes.fromhex(sys.argv[1].replace(":",""))).decode().rstrip("="))' \
    91:99:F3:…
```

```sh
WEBAUTHN_ORIGINS=android:apk-key-hash:kZnz5woESO8kA7iFlIjUCXVvfA_Nw1LbKOfUfX0w0PA
```

Without it the ceremony runs to completion on the device and then fails at
`/auth/passkey/login/finish` with `401`. iOS 17.4+ reports the HTTPS origin
configured in step 1 and needs nothing extra.

## Troubleshooting

| Symptom | Cause |
|---|---|
| No passkey button on the login screen | No passkey registered yet, or the browser has no WebAuthn support (check `GET /api/config` → `passkeys.available`). |
| Registration fails immediately in the browser | Not a secure context — plain HTTP over a LAN address or IP. |
| Android: *"RP ID not found"* | The device could not fetch a valid `assetlinks.json` from the RP ID's domain: not served, served as HTML by the SPA fallback, or unreadable by the container user. |
| Android: *"RP ID cannot be validated"* | The file was fetched but does not vouch for this app: wrong package name, or a fingerprint that is not the one which signed the installed APK. |
| Native: `Cannot read properties of undefined (reading 'id')` | The passkey plugin is missing from the build, or the call reached it shaped as the wrong ceremony. Both are covered by `frontend/src/lib/webauthn/` and its tests — see [mobile.md](mobile.md). |
| `401` right after the device prompt succeeds | The reported origin is not allowed: add the Android `apk-key-hash` origin to `WEBAUTHN_ORIGINS`, or check that the iOS build sends the configured HTTPS origin. |
| Everything is fixed but the device still fails | Both platforms cache the association result. Reinstall the app (Android: or clear Google Play services storage) rather than assuming the fix did not land. |
| All passkeys stopped working after a move | The RP ID changed. Sign in with the password and enrol again. |

## How it works

The wire protocol, error codes and endpoint list live in
[API.md](../API.md#passkeys-webauthn); the server-side design — discoverable
credentials, the single-use in-memory challenge store, signature-counter
write-back — is in
[architecture/backend.md](architecture/backend.md#passkeys-webauthn).

Ceremony code on the client is in `frontend/src/lib/webauthn/`: the web path goes
through `navigator.credentials`, the native path through platform APIs, and both
speak the same JSON shape to the server.

## Security notes

- **Passkeys are phishing-resistant.** The credential is bound to the RP ID, so a
  look-alike domain cannot use it, and there is no shared secret to leak.
- **`/api/v1/passkeys/*` requires a JWT session.** An API token — even one with
  `["*"]` — is rejected there: enrolling a passkey mints a password-equivalent
  credential, and a long-lived token must not be able to do that.
- **Cloned-authenticator detection.** Every assertion writes the authenticator's
  signature counter back, so a copied credential can be spotted. Synced passkeys
  legitimately report `0` and are not flagged.
- **Ceremonies expire.** A challenge is valid for 5 minutes and exactly one
  attempt; a replayed response is refused with `passkey_ceremony_invalid`.
