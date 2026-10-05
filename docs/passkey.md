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
of the certificate that signed the **installed** APK. There are two Android
packages and each needs its own entry: `ru.tinyops.turboist` is the WebView app,
`ru.tinyops.turboist.native` the native client, which carries a package id of its
own so both can be installed side by side. An app that is not named here gets the
passkey sheet and then a failure, however correct its fingerprint is.

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
  },
  {
    "relation": [
      "delegate_permission/common.handle_all_urls",
      "delegate_permission/common.get_login_creds"
    ],
    "target": {
      "namespace": "android_app",
      "package_name": "ru.tinyops.turboist.native",
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

The value comes from the signing certificate, not from the package name, so two
apps signed by the same key report the same origin and need one entry between
them — which is what a machine's debug keystore gives both Android builds. A
build signed by a different key (Play App Signing, another developer's machine)
reports a different origin and needs its own entry.

Without it the ceremony runs to completion on the device and then fails at
`/auth/passkey/login/finish` with `401`. iOS 17.4+ reports the HTTPS origin
configured in step 1 and needs nothing extra.

### The native Android client

`android-native/` is a second Android app — Compose rather than a WebView — and
it signs in with passkeys through Credential Manager. It needs steps 2 and 3, not
step 1: there is no domain to bake in, because the server is typed in by the user
at first launch and the relying party follows whatever instance the app is
connected to. So the association file has to be served **by that instance**, and
its `WEBAUTHN_ORIGINS` has to allow the origin of the certificate that signed the
installed APK.

Its package id is `ru.tinyops.turboist.native`, which is why `assetlinks.json`
lists two targets. Everything else is the same file and the same origin value.

Everything on the server side of that can be checked without a phone:

```sh
just android-native-passkey-preflight
```

It builds the server, starts an instance configured as a relying party with its
own database and `deploy/well-known/` published under `/.well-known/`, and then
asserts what the app depends on: `GET /api/config` reports passkeys as enabled,
the association file is served **as `application/json`** and names both packages
with a fingerprint each, enrolment asks for a discoverable credential bound to
the configured RP ID, login names no account and no credential, and both
ceremonies still have the shape of the recordings the client's tests are pinned
to. Run it after changing `deploy/well-known/`, the WebAuthn configuration, or
anything about the ceremony payloads.

It starts an instance of its own because an ordinary development instance is
plain HTTP on `localhost`: no relying party comes up there at all, and
`/.well-known/assetlinks.json` is answered by the SPA fallback with `200` and
`index.html` — the exact trap above, and the reason a status-code check proves
nothing.

What it cannot cover is the platform prompt: Credential Manager fetches the
association file itself, over HTTPS, from the relying party's real domain, so
registering and signing in with an actual credential needs an HTTPS instance on
a real domain and a device signed into a passkey provider. That step is a device
check, and everything on either side of it is covered here.

In the app itself the two halves sit where the web client puts them: the sign-in
screen offers a passkey button — only when the instance reports both
`passkeys.enabled` and `passkeys.available`, since a button that can only end in
an explanation is worse than no button — and **Settings → Passkeys** lists the
account's credentials and enrols a new one. The list is never cached: a stale
answer to "which devices can sign in to this account" is worse than no answer,
so an unreachable server is stated as such.

What the user sees when a prerequisite is missing is deliberately specific, and
worth reading as a diagnosis:

| On screen | What it means |
|---|---|
| No passkey button on the sign-in screen | `GET /api/config` said `passkeys.enabled` or `passkeys.available` is false: WebAuthn is not configured on that instance, or nobody has enrolled a credential yet. |
| *"This device could not verify the server for passkeys"* | The device fetched `assetlinks.json` and it did not vouch for this app: the file is missing, answered by the SPA fallback, names only the other package, or lists a fingerprint that is not the installed APK's. |
| *"This device has no passkey for this account"* | The account has passkeys, but none of them lives on (or syncs to) this phone. Sign in with the password once and add one from settings. |
| *"The server did not accept that passkey"* | The ceremony finished on the device and the server refused it — most often the `android:apk-key-hash:` origin missing from `WEBAUTHN_ORIGINS`. |

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
