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

`sha256_cert_fingerprints` must list every signing certificate in use:

- `just deploy-android` builds `assembleDebug`, so the fingerprint is your local
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
`clientDataJSON` instead of an HTTPS origin:

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

## Caching

Both platforms cache the result. After changing a file, reinstall the app (or
clear Google Play services storage) rather than assuming the change did not take.
