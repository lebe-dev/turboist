# Mobile apps (iOS & Android)

Turboist ships native iOS and Android apps built with [Capacitor](https://capacitorjs.com/). The same SvelteKit bundle serves three targets:

- the **web** app (embedded in the Go binary),
- the **iOS** app (WebView shell + native bridge),
- the **Android** app (WebView shell + native bridge).

There is **one** build. The app detects its platform at runtime (`Capacitor.isNativePlatform()`); it does not need separate web/native bundles. The apps are **online-only** — like the web app they read through the REST API and stay fresh over SSE. There is no offline store.

The Capacitor project lives under `frontend/` (that is where `package.json` is): `frontend/capacitor.config.ts`, `frontend/ios/`, `frontend/android/`.

## How it talks to your server

Because the app is a bundled shell, it needs to know **which server** to talk to. On first launch it shows a **Connect** screen where you enter your Turboist server URL. It is validated against the public `GET /api/config` endpoint and persisted on device (`@capacitor/preferences`). All API paths are then resolved against that base URL.

Networking specifics:

- **REST** goes through `CapacitorHttp` (`window.fetch`/`XHR` are routed to the native HTTP stack), which **bypasses CORS**.
- **SSE** (`EventSource`, `/api/v1/events`) is *not* patched by CapacitorHttp, so it is a genuine cross-origin request from the WebView origin (`capacitor://localhost` on iOS, `https://localhost` on Android). The backend allows those origins with a CORS middleware scoped to that one route (`internal/httpapi/handlers/events.go`).

> **Use HTTPS.** The server URL is entered at runtime, so transport rules cannot be domain-scoped at build time. A plain-HTTP server requires extra opt-in (iOS `NSAppTransportSecurity`, Android cleartext/network-security-config) — treat that as a separate task.

## Auth on native

- `clientKind` is `ios` or `android` (web is `web`).
- The rotating **refresh token** is stored in the iOS Keychain / Android Keystore (`@aparajita/capacitor-secure-storage`) and sent in the `POST /auth/refresh` **body** (the backend reads body-first). There is no HttpOnly cookie on native — the backend sets that only for `web`.
- Logout clears the stored refresh token; the server URL is kept (it is configuration, not a credential).

## Prerequisites

Common: **Node 22+**, **yarn 1**.

### iOS

- **Xcode 26+**. Capacitor 8 scaffolds iOS via **Swift Package Manager — CocoaPods is not required.**
  ```sh
  xcode-select --install        # if the command-line tools are missing
  sudo xcodebuild -license accept
  # open Xcode once so it installs an iOS platform + simulator runtime
  ```
- CocoaPods is only a fallback for the SPM plugin-exposure bug ([capacitor#8325](https://github.com/ionic-team/capacitor/issues/8325)); if you hit it, `brew install cocoapods` and `yarn cap add ios --packagemanager CocoaPods`.

### Android

Scaffolding works without the SDK; **building** needs it.

```sh
brew install --cask android-studio   # launch once; install SDK Platform 36 + Build-Tools + an emulator
# persist SDK env (zsh):
export ANDROID_HOME="$HOME/Library/Android/sdk"
export PATH="$ANDROID_HOME/platform-tools:$ANDROID_HOME/cmdline-tools/latest/bin:$PATH"
sdkmanager "platform-tools" "platforms;android-36" "build-tools;36.0.0"
```

Android Studio must be **Otter (2025.2.1)+** for Capacitor 8.

For a **headless** setup (no Android Studio) — enough to build and deploy with `just deploy-android`:

```sh
brew install --cask android-commandlinetools android-platform-tools
export ANDROID_SDK_ROOT="/opt/homebrew/share/android-commandlinetools"   # brew SDK root
yes | sdkmanager --sdk_root="$ANDROID_SDK_ROOT" --licenses
sdkmanager --sdk_root="$ANDROID_SDK_ROOT" "platform-tools" "platforms;android-36" "build-tools;36.0.0"
```

The first Gradle build may auto-pull an extra Build-Tools revision a plugin pins; it accepts the license from the `licenses/` dir populated above.

## Build & run

All recipes rebuild the web bundle and sync it into the native projects first.

```sh
just cap-sync        # yarn build + cap sync (both platforms)
just ios-run         # build, sync iOS, run on a simulator/device
just android-run     # build, sync Android, run on an emulator/device
just ios-build       # build, sync, open Xcode
just android-build   # build, sync, open Android Studio
```

Re-run `just cap-sync` after **every** web change — the native projects hold a *copy* of `frontend/build`.

### Deploy a signed Debug build to a physical iPhone

`just deploy-ios` rebuilds the web bundle, syncs it, code-signs a Debug build with your Apple Development team, then installs it on the connected device via `devicectl` (auto-detecting the first plugged-in iPhone; override with `IOS_DEVICE_ID=<udid>`).

The signing team is **not** committed into the Xcode project. It comes from `IOS_DEV_TEAM_ID`:

1. Find your team id — the `OU` field of the cert:
   ```sh
   security find-identity -v -p codesigning
   ```
2. Put it in `.env.dev` (`IOS_DEV_TEAM_ID=XXXXXXXXXX`).
3. `just init-dev` merges `.env.example` + `.env.dev` into `.env` (it refuses to overwrite an existing `.env` — edit that by hand, or delete and re-run). `set dotenv-load` then loads `IOS_DEV_TEAM_ID` for the recipe.
4. `just deploy-ios`.

**First launch on a personal (free) profile** is blocked by iOS until you trust the developer once: **Settings → General → VPN & Device Management → Developer App → Trust**. After that the app icon launches normally.

### Deploy a Debug APK to a physical Android device

`just deploy-android` rebuilds the web bundle, syncs it, assembles a **Debug** APK (`./gradlew assembleDebug`, auto-signed with Gradle's debug keystore — no dev team or keystore needed), then installs and launches it over `adb`.

The SDK is located from `ANDROID_SDK_ROOT`/`ANDROID_HOME`, else common install paths (Homebrew `android-commandlinetools`, `~/Library/Android/sdk`). With several devices attached, pick one with `ANDROID_DEVICE_ID=<serial>` (from `adb devices`).

On the phone, **enable USB debugging** first (Settings → About → tap *Build number* ×7, then Developer options → *USB debugging*), set the USB mode to *File transfer*, and accept the **"Allow USB debugging?"** prompt. There is no trust-the-developer dance like iOS — a Debug APK launches straight away.

### App icon

The icon is the same mark as the web app — the orange (`#e2580e`) tile with the white lightning bolt from `frontend/static/icons/icon.svg`.

- **iOS**: a single 1024×1024 `AppIcon-512@2x.png` in the Xcode asset catalog (`frontend/ios/App/App/Assets.xcassets/AppIcon.appiconset/`). It is a **full-bleed square** — no rounded corners and **no alpha channel** (iOS rejects transparency in app icons and masks the corners itself), so it drops the `rx=96` rounding the web SVG uses.
- Regenerate it from the web SVG (needs `rsvg-convert`, from `librsvg`) — start from a square variant of the SVG with the `<rect>` corner radius removed:
  ```sh
  rsvg-convert -w 1024 -h 1024 icon-square.svg \
    -o frontend/ios/App/App/Assets.xcassets/AppIcon.appiconset/AppIcon-512@2x.png
  ```
  Xcode derives every smaller size at build time; rebuild (`just deploy-ios`) to apply.

## Lock-screen widget (iOS)

iOS ships a **lock-screen widget** — a round `+` accessory button. Tapping it opens
the app and pops the **QuickAdd** (new task) dialog straight away. It carries no
live data: it is a pure launcher, so the widget extension makes no API calls and
needs no auth or App Group.

How it is wired:

- **Widget extension** — `frontend/ios/App/TurboistWidget/` (`TurboistWidget.swift`
  + `Info.plist`). A WidgetKit `StaticConfiguration` limited to the
  `.accessoryCircular` family (lock screen). Its view's `widgetURL` is
  `turboist://quick-add`. Minimum iOS 16 (accessory widgets); the required
  `containerBackground` is applied only on iOS 17+ via an availability guard.
- **URL scheme** — `turboist` is registered in the App target's `Info.plist`
  (`CFBundleURLTypes`). Capacitor's `AppDelegate` already forwards `open url:` to
  the bridge, so `@capacitor/app` emits `appUrlOpen`.
- **SPA glue** — `frontend/src/lib/native/deepLink.ts` (`initDeepLinks()`, called
  once from the root `+layout.svelte`) turns a `turboist://quick-add` open into the
  same `turboist:quick-add` window event the top-bar `+` dispatches. It covers both
  a **warm** open (app running → dispatch immediately) and a **cold** launch (app
  started by the widget → a pending flag that `(app)/+layout.svelte` drains once it
  mounts, surviving the login → today redirect on a fresh launch).

To add the widget on the phone: long-press the lock screen → **Customize** → the
widgets row below the clock → add **Turboist**.

### If you regenerate the iOS project

The widget **target** lives in the committed `App.xcodeproj`; `cap sync` does not
touch it. Only a full `yarn cap add ios` (scaffolding from scratch) drops it — then
re-run `just ios-widget` (needs the `xcodeproj` gem: `gem install xcodeproj`) to
re-add the target, embed phase, and build settings. The Swift sources are already
on disk under `App/TurboistWidget/`.

Signing is automatic: `just deploy-ios` passes `DEVELOPMENT_TEAM` to `xcodebuild`
(applied to every target) with `-allowProvisioningUpdates`, so Xcode provisions the
widget's `com.itkey.turboist.TurboistWidget` bundle id alongside the app.

## First launch flow

1. **Connect** screen → enter the server URL (`https://…`). Validated against `GET /api/config`.
2. First-ever server → setup (create the single user). Otherwise → login (+ TOTP if enabled).
3. The app loads tasks/projects over REST and subscribes to SSE for live updates.

## Known native gaps (deferred)

- **Google Calendar OAuth** — the OAuth redirect is a full-page navigation and does not round-trip inside the WebView. Configure Google Calendar from the **web** UI for now.
- **Backup download** — the anchor-based file download is a no-op in a WebView. Restore (file picker) works; export from the web UI.
- The "new version available" toast and stale-chunk reload are **web-only** (native assets are bundled, never stale).

## Troubleshooting

- **Blank screen / API calls fail** — check the entered server URL; ensure HTTPS (or add the cleartext opt-in for HTTP).
- **Live updates don't arrive** — SSE CORS: confirm the server is on this build (the `/api/v1/events` CORS allow-list) and that the URL is reachable.
- **Android build can't find the SDK** — `ANDROID_HOME`/`ANDROID_SDK_ROOT` is unset; see Prerequisites.
- **`adb devices` is empty** — USB debugging is off or unauthorized: enable it, set the USB mode to *File transfer* (not charge-only), and accept the on-device "Allow USB debugging?" prompt. A charge-only cable also shows nothing — try another cable/port.
- **`deploy-android` fails with `mapfile: command not found`** — shouldn't happen (the recipe is bash-3.2 safe); if you forked it, avoid `mapfile` on macOS's system bash.
- **iOS plugin not found after adding one** — re-run `yarn cap sync`; if it persists, use the CocoaPods fallback (`#8325`).
- **iOS signing fails with "requires a development team"** — `IOS_DEV_TEAM_ID` is not in `.env`; set it in `.env.dev` and run `just init-dev` (see *Deploy a signed Debug build*).
- **Old app icon still shows after redeploy** — SpringBoard caches icons; delete the app and reinstall, or reboot the device.
