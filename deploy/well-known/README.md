# `/.well-known/` association files

Served by the backend when `WELL_KNOWN_PATH` points at this directory, or by a
reverse proxy. Full setup guide: `docs/passkey.md`. Both files exist for **native
passkeys**: the OS refuses to hand the app a credential for a domain until the
domain vouches for the app.

## `assetlinks.json` (Android)

Android Credential Manager fetches `https://<rp-id>/.well-known/assetlinks.json`
and looks for a `delegate_permission/common.get_login_creds` entry naming this
package and the certificate that signed the **installed** APK. Without it the
passkey sheet fails with *"RP ID not found"*.

Both relations in the array are deliberate. Only `get_login_creds` is about
passkeys, but Google's FIDO2 documentation still asks for
`delegate_permission/common.handle_all_urls` beside it until its own migration
completes, and a file carrying `get_login_creds` alone is refused by the device
with *"RP ID cannot be validated"* — while Google's Digital Asset Links API
answers `linked: true` for the same file, which is what makes the failure look
like a fingerprint problem. `handle_all_urls` changes nothing about link
handling here: this app claims no verified web links.

The Android package is `ru.tinyops.turboist.native`. The entry is keyed by package
name, so an app missing from this file gets the passkey sheet and then a failure,
however correct its fingerprint is. A deployment that predates the native client
may still name `ru.tinyops.turboist` — the Capacitor app that no longer exists —
and a file naming only that one vouches for nothing: the device refuses every
attempt with a `DataError` ("RP ID cannot be validated") before the sheet is
drawn, which the client reports as *"This device could not verify the server for
passkeys"*. `just android-native-passkey-preflight` checks the name in this
directory, but only in this directory; an installed instance serves its own copy.

`sha256_cert_fingerprints` must list every signing certificate in use:

- `just android-native-deploy` builds the debug APK, so the fingerprint is your local
  debug keystore's:

  ```sh
  keytool -list -v -alias androiddebugkey -keystore ~/.android/debug.keystore \
      -storepass android | grep SHA256
  ```

  A debug keystore is per-machine — building on a second machine means adding
  that machine's fingerprint here too.
- A Play Store build is re-signed by Play App Signing: take the fingerprint from
  Play Console → *App integrity* → *App signing key certificate*.

The same fingerprint also has to reach the server as an allowed WebAuthn origin,
because Android reports `android:apk-key-hash:<base64url-sha256>` in
`clientDataJSON` instead of an HTTPS origin. That value is derived from the signing
certificate, so a build signed by a different key (a Play-signed release, another
developer's machine) needs its own entry.

```sh
# hex fingerprint -> the WEBAUTHN_ORIGINS value
python3 -c 'import base64,sys; print("android:apk-key-hash:"+base64.urlsafe_b64encode(bytes.fromhex(sys.argv[1].replace(":",""))).decode().rstrip("="))' \
    91:99:F3:...
```

## `apple-app-site-association` (iOS)

Same idea for iOS: a `webcredentials` entry naming `<TEAM_ID>.ru.tinyops.turboist`,
served as JSON **without** a file extension. Copy
`apple-app-site-association.example` to `apple-app-site-association` and fill in
the team id from the Apple Developer account.

## Checking this directory

`just android-native-passkey-preflight` starts a server with this directory
published under `/.well-known/` and asserts that `assetlinks.json` comes back as
`application/json`, names the Android package, and gives it a fingerprint.
Worth running after every edit here: a web app answers any unknown path with
`200` and its `index.html`, so a file that is not served at all still looks fine
to anything that only reads the status code.

## Caching

Both platforms cache the result. After changing a file, reinstall the app (or
clear Google Play services storage) rather than assuming the change did not take.
