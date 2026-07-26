# Deploying the iOS app with AltServer + AltStore

This is the **no-cable, no-Xcode** way to get Turboist onto your iPhone: build an unsigned `.ipa` on the Mac with `just build-ipa`, then let [AltStore](https://altstore.io/) install and sign it on the device with your own Apple ID.

For the rest of the mobile picture — how the app finds your server, native auth, prerequisites, the widget, offline behaviour — see [docs/mobile.md](mobile.md).

## When to use which recipe

| Recipe | Needs | Signing | Good for |
| --- | --- | --- | --- |
| `just ios-run` | Xcode, simulator or device | Xcode handles it | Fast iteration during development |
| `just deploy-ios` | Cable, `IOS_DEV_TEAM_ID` | Signed **Debug** build via `xcodebuild` + `devicectl` | The tightest loop when the phone is plugged in |
| **`just build-ipa`** | Nothing beyond Xcode's toolchain | **Unsigned** — AltStore re-signs on device | Installing a **Release** build wirelessly; "daily driver" installs |

`build-ipa` is the only one of the three that produces a distributable artifact you can hand to a device over AirDrop.

## Why the .ipa is unsigned

`build-ipa` builds with `CODE_SIGNING_ALLOWED=NO`, so `codesign -dv` on the product reports `code object is not signed at all`. That is deliberate, not a shortcut:

- AltStore **re-signs the entire bundle** — the app, the `TurboistWidget` extension, and the embedded `Capacitor`/`Cordova` frameworks — with a certificate it provisions from your Apple ID at install time.
- Any signature baked in on the Mac would simply be stripped and replaced.

Consequences: the recipe needs **no** `IOS_DEV_TEAM_ID`, no provisioning profile, and no connected device. The `.env` setup described for `deploy-ios` is irrelevant here.

## One-time setup

You only do this once per Mac / per phone.

1. **AltServer on the Mac** — install from [altstore.io](https://altstore.io/) and leave it running in the menu bar. AltServer is what talks to Apple to provision certificates and what refreshes apps before they expire.
2. **AltStore on the iPhone** — installed *by* AltServer over USB the first time (follow AltStore's own instructions for the current macOS/iOS combination). After that the cable is no longer needed.
3. **Same Wi-Fi network** — AltStore's background refresh only works when the phone can reach AltServer. Keep both on the same LAN.
4. **Apple ID** — AltStore asks for a real Apple ID and a 2FA code (not an app-specific password) to provision the signing certificate. A free Apple ID works; see [Free Apple ID limits](#free-apple-id-limits).

## Build the .ipa

```sh
just build-ipa
```

What it does, in order:

1. Runs `cap-sync-versioned` — stamps `frontend/package.json`'s `version` with `VERSION` + the short git commit hash (e.g. `1.14.0+61b9737`), runs `yarn build`, syncs the bundle into `frontend/ios/`, then restores `package.json` so the stamp never shows up as a working-tree diff. That value is what the app shows in **Settings → Version**, so you can always tell which commit is installed.
2. Builds the `App` scheme, `Release`, `-destination 'generic/platform=iOS'`, code signing off, into `frontend/ios/DerivedData/`.
3. Wraps `App.app` into the `Payload/` directory layout an `.ipa` expects and zips it.

Output:

```
dist/Turboist-1.14.0+61b9737.ipa      # ~1.3 MB
```

`dist/` is gitignored. The archive contains `Payload/App.app/` with `PlugIns/TurboistWidget.appex`, `Frameworks/{Capacitor,Cordova}.framework`, and the SPA under `public/`.

## Install on the iPhone

1. **Transfer the file** — AirDrop is easiest; Files, iCloud Drive, or any share sheet works. AirDropped files land in *Files → Downloads*.
2. **AltStore → My Apps → “+”** (top-left) → pick `Turboist-<version>.ipa`.
3. Enter your Apple ID credentials if prompted. AltStore registers the App IDs, provisions a certificate, re-signs, and installs. This takes a few seconds to a minute.
4. **Trust the developer once**: **Settings → General → VPN & Device Management → Developer App → Trust**. iOS blocks the first launch until you do. This is per-certificate, not per-install, so you rarely repeat it.
5. Launch the app. You get the **Connect** screen — enter your Turboist server URL (`https://…`, validated against `GET /api/config`). Then setup or login as usual.

## Updating to a new build

Run `just build-ipa` again and install the new `.ipa` the same way. AltStore replaces the existing app **in place** as long as the bundle id (`ru.tinyops.turboist`) is unchanged — your session, server URL, and offline cache survive.

There is no auto-update channel: Turboist is not in an AltStore source, so every update is a manual build + install.

## Keeping it alive (expiry & refresh)

A sideloaded app is only valid as long as its provisioning profile.

- **Free Apple ID → 7 days.** After that the app refuses to launch until refreshed.
- **Paid developer account → 1 year.**

Refresh options, in order of least effort:

- **Background refresh** — AltStore refreshes automatically when it can reach AltServer. Requirements: AltServer running on the Mac, both devices on the same Wi-Fi, and AltStore permitted to run in the background. This is the whole point of the AltServer half of the setup.
- **Manual refresh** — AltStore → *My Apps* → **Refresh All**, with AltServer reachable.
- Refresh **AltStore itself** too; it is sideloaded and expires on the same clock.

If the app has already expired, a refresh usually revives it without reinstalling. If it does not, install the `.ipa` again — an in-place install keeps your data.

## Free Apple ID limits

Worth knowing before you hit them:

- **3 sideloaded apps** installed at once.
- **10 App IDs per 7 days.** Turboist consumes **two**: `ru.tinyops.turboist` and the widget's `ru.tinyops.turboist.TurboistWidget`. Registered App IDs stay burnt for the full 7 days even if you uninstall.
- The 7-day expiry above.

Turboist has **no entitlements file** and does not use App Groups, iCloud, or push — none of the capabilities a free Apple ID cannot provision. Nothing in the app requires a paid account.

## Deployment targets

- App: **iOS 15.0+**
- `TurboistWidget` lock-screen widget: **iOS 16.0+** (the widget target simply does not install on iOS 15)

## Troubleshooting

- **`just build-ipa` fails inside `cap-sync-versioned`** — the failure is in the web build, not the iOS build. A common cause is unresolved merge-conflict markers in `frontend/src/`, which surface as `RolldownError: Unexpected token`. Fix the sources and re-run.
- **`error: $APP not found after build`** — `xcodebuild` reported success but produced no `Release-iphoneos/App.app`. Re-run after `rm -rf frontend/ios/DerivedData`.
- **AltStore: "Could not find AltServer"** — AltServer is not running, or the phone and Mac are on different networks / a network with client isolation (many guest and corporate Wi-Fis). Plug in over USB once to complete the operation.
- **AltStore fails to install with a certificate or App ID error** — usually the free-account quotas above: too many sideloaded apps, or the 10-App-IDs-per-7-days limit. Remove other sideloaded apps and retry.
- **"Untrusted Developer" on launch** — the trust step was skipped: **Settings → General → VPN & Device Management**.
- **App launches, then quits immediately after ~7 days** — expired profile; refresh in AltStore.
- **Blank screen or API errors after the Connect screen** — server URL or transport, not sideloading. See *Troubleshooting* in [docs/mobile.md](mobile.md#troubleshooting).
- **Widget missing from the lock-screen gallery** — the device is on iOS 15, or the widget target was not embedded in the build (`just ios-widget` re-wires it after a project regeneration).
