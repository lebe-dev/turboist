# Mobile apps (iOS & Android)

Turboist ships native iOS and Android apps built with [Capacitor](https://capacitorjs.com/). The same SvelteKit bundle serves three targets:

- the **web** app (embedded in the Go binary),
- the **iOS** app (WebView shell + native bridge),
- the **Android** app (WebView shell + native bridge).

There is **one** build. The app detects its platform at runtime (`Capacitor.isNativePlatform()`); it does not need separate web/native bundles. Like the web app they read through the REST API and stay fresh over SSE, and they share the same **offline** read cache + write outbox — see [docs/offline.md](offline.md).

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

### Passkeys on native

The WebView cannot run a browser WebAuthn ceremony for your server: its origin is
`capacitor://localhost` (iOS) / `https://localhost` (Android), not your domain. So
`frontend/src/lib/webauthn/` routes native ceremonies through the platform passkey
APIs via `@capgo/capacitor-passkey`, while the web build keeps using
`navigator.credentials` (the plugin is imported lazily, so it never enters the web
bundle).

The setup below is summarised here for the native build; the full feature guide,
including the web side and a troubleshooting table, is
[passkey.md](passkey.md).

Unlike everything else on native, this cannot be configured at runtime — iOS bakes
the associated-domains entitlement in at sign time. A native build therefore
targets one instance domain:

1. **Point the app at your domain.** In `frontend/capacitor.config.ts`, set
   `plugins.CapacitorPasskey.origin` (e.g. `https://todo.example.com`) and
   `domains` (`['todo.example.com']`), then run `just mobile`.
2. **Serve the association files** from that same domain — set `WELL_KNOWN_PATH`
   (see [configuration.md](configuration.md); `docker-compose.yml` already mounts
   `deploy/well-known/` at `/app/well-known`) or let your reverse proxy answer:
   - `/.well-known/apple-app-site-association` →
     `{"webcredentials":{"apps":["<TEAM_ID>.ru.tinyops.turboist"]}}`, served as
     JSON with no file extension;
   - `/.well-known/assetlinks.json` → a Digital Asset Links entry for
     `ru.tinyops.turboist` listing every signing certificate fingerprint you use,
     debug builds included.
3. **Allow the Android origin.** Android Credential Manager reports
   `android:apk-key-hash:<sha256>` in `clientDataJSON`, not your HTTPS origin, so
   add it to the server's `WEBAUTHN_ORIGINS`. iOS 17.4+ reports the configured
   HTTPS origin and needs nothing extra.

Skip all of this and the native apps simply sign in with a password as before;
web passkeys are unaffected.

**Symptom → cause.** The platform sheet opening and then failing with *"RP ID not
found"* means Credential Manager could not verify the domain against the app:
either the served `assetlinks.json` is missing/wrong (a SPA install answers `200`
with `index.html`, which looks fine to `curl -o /dev/null` — check the body), the
signing fingerprint in it is not the one that signed the installed APK, or the
native plugin is missing from the build so the call fell back to the WebView,
whose origin is `https://localhost` and never matches the RP ID. Verify the last
one with `npx cap sync android` — `@capgo/capacitor-passkey` must appear in the
plugin list and `android/app/src/main/res/values/capacitor-passkey.xml` must
point at your domain.

**`Cannot read properties of undefined (reading 'id')` on login** was this same
seam: `@capgo/capacitor-passkey` distinguishes an assertion from a registration
by whether the call carries a `mediation` key, and without it runs the create
path, which reads `publicKey.rp.id` — a field only registration options have.
`lib/webauthn/index.ts` therefore always passes `mediation` to `getCredential`
and never to `createCredential`; `native.test.ts` pins both directions.

**A passkey is per-device unless it syncs.** One created with Touch ID on a Mac
lives in iCloud Keychain and will not show up on a Pixel. On the phone either
sign in with the password once and add a passkey from Settings → Security, or use
the platform's cross-device (QR) flow from the desktop credential.

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
just build-ipa       # build, sync, package an unsigned .ipa into dist/ (AltStore)
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

### Sideload an .ipa with AltStore (no cable, no Xcode)

`just build-ipa` produces `dist/Turboist-<VERSION>+<commit>.ipa`: it rebuilds the web bundle through `cap-sync-versioned`, builds the `App` scheme in **Release** for `generic/platform=iOS` with `CODE_SIGNING_ALLOWED=NO`, then wraps `App.app` (including the `TurboistWidget.appex` extension and the embedded `Capacitor`/`Cordova` frameworks) into the `Payload/` layout an `.ipa` expects.

The build is deliberately **unsigned**, so no `IOS_DEV_TEAM_ID` and no connected device are needed — AltStore re-signs the whole bundle with your own Apple ID at install time, and any signature baked in here would just be discarded.

1. `just build-ipa`
2. Get the file onto the phone — AirDrop, Files, iCloud Drive, whatever.
3. On the iPhone: **AltStore → My Apps → “+” → pick the .ipa**, and enter your Apple ID when asked.
4. Trust the developer once: **Settings → General → VPN & Device Management → Developer App → Trust**.

Full walkthrough — AltServer setup, refresh/expiry, free-account quotas, troubleshooting: [docs/deploy-ios.md](deploy-ios.md).

Notes for a **free** Apple ID: the app expires after **7 days** — keep AltServer running on this Mac (and the phone on the same network) so AltStore can refresh it in the background; you get at most 3 sideloaded apps, and the widget extension consumes a second App ID out of the 10-per-7-days quota. A paid developer account raises the expiry to a year.

`deploy-ios` remains the faster loop when the phone is plugged in — it installs a signed Debug build directly, skipping the packaging and the AltStore round-trip.

### Deploy a Debug APK to a physical Android device

`just deploy-android` rebuilds the web bundle, syncs it, assembles a **Debug** APK (`./gradlew assembleDebug`, auto-signed with Gradle's debug keystore — no dev team or keystore needed), then installs and launches it over `adb`.

The SDK is located from `ANDROID_SDK_ROOT`/`ANDROID_HOME`, else common install paths (Homebrew `android-commandlinetools`, `~/Library/Android/sdk`). With several devices attached, pick one with `ANDROID_DEVICE_ID=<serial>` (from `adb devices`).

On the phone, **enable USB debugging** first (Settings → About → tap *Build number* ×7, then Developer options → *USB debugging*), set the USB mode to *File transfer*, and accept the **"Allow USB debugging?"** prompt. There is no trust-the-developer dance like iOS — a Debug APK launches straight away.

### Local build version stamp

`just deploy-ios` and `just deploy-android` build through `cap-sync-versioned` instead of plain `cap-sync`: it stamps `frontend/package.json`'s `version` field with `VERSION` + the short git commit hash (e.g. `1.14.0-dev+a1b2c3d`) before running `yarn build`, then restores the file afterwards (`git checkout -- package.json`), so the stamp never shows up as a working-tree diff. That value is what renders in the app's **Settings → Version** row, so a locally sideloaded Debug build tells you exactly which commit it was built from. `just ios-run`/`just android-run`/`just ios-build`/`just android-build` still use plain `cap-sync` and show the untouched `package.json` version (normally `0.0.0` outside of the Docker build, which does the same substitution without the commit hash — see `Dockerfile`).

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
widget's `ru.tinyops.turboist.TurboistWidget` bundle id alongside the app.

## First launch flow

1. **Connect** screen → enter the server URL (`https://…`). Validated against `GET /api/config`.
2. First-ever server → setup (create the single user). Otherwise → login (+ TOTP if enabled).
3. The app loads tasks/projects over REST and subscribes to SSE for live updates.

## Pull-to-refresh

Native-only: on the current screen, pulling down from the top of the scroll area with an elastic drag re-fetches shell data (contexts/labels/projects/inbox/plan/pinned tasks) and re-broadcasts the same `turboist:invalidate` scopes the SSE reconnect sweep uses, so every open page's `useInvalidation` listener revalidates. Implemented in `frontend/src/lib/components/app/PullToRefresh.svelte`, wired around `<main>` in `routes/(app)/+layout.svelte` and gated by `isNativePlatform()` — the web app has no equivalent gesture and stays fresh via SSE alone.

## Known native gaps (deferred)

- **Google Calendar OAuth** — the OAuth redirect is a full-page navigation and does not round-trip inside the WebView. Configure Google Calendar from the **web** UI for now.
- **Backup download** — the anchor-based file download is a no-op in a WebView. Restore (file picker) works; export from the web UI.
- The "new version available" toast and stale-chunk reload are **web-only** (native assets are bundled, never stale).

## Offline on native

Native shares the web offline layer (read cache + write outbox), with a few native-only
notes — see [docs/offline.md](offline.md) for the full model:

- **`CapacitorHttp` bypasses the service worker.** `fetch`/XHR go through the native HTTP
  stack, so `/api/*` requests never hit the SW. That is fine — the offline cache and outbox
  live in the JS layer (`frontend/src/lib/offline/`), not the SW, so they work on native
  without any service worker. The SW only delivers the web shell.
- **The app shell is always local.** `webDir: 'build'` is bundled into the app, so only
  *data* can ever be missing offline, never the app itself. There is no PWA install step.
- **Secure token survives logout of the offline data.** The refresh token is stored in the
  device secure storage (`frontend/src/lib/native/secureToken.ts`), separate from the
  IndexedDB cache/outbox. Clearing offline data does not by itself drop the session, and an
  expired refresh routes to login without wiping the queued outbox.
- **No Background Sync.** Replay runs only while the app is open/foregrounded (kicks on app
  start, network recovery, and foreground), never in the background.

## Troubleshooting

- **Blank screen / API calls fail** — check the entered server URL; ensure HTTPS (or add the cleartext opt-in for HTTP).
- **Live updates don't arrive** — SSE CORS: confirm the server is on this build (the `/api/v1/events` CORS allow-list) and that the URL is reachable.
- **Android build can't find the SDK** — `ANDROID_HOME`/`ANDROID_SDK_ROOT` is unset; see Prerequisites.
- **`adb devices` is empty** — USB debugging is off or unauthorized: enable it, set the USB mode to *File transfer* (not charge-only), and accept the on-device "Allow USB debugging?" prompt. A charge-only cable also shows nothing — try another cable/port.
- **`deploy-android` fails with `mapfile: command not found`** — shouldn't happen (the recipe is bash-3.2 safe); if you forked it, avoid `mapfile` on macOS's system bash.
- **iOS plugin not found after adding one** — re-run `yarn cap sync`; if it persists, use the CocoaPods fallback (`#8325`).
- **iOS signing fails with "requires a development team"** — `IOS_DEV_TEAM_ID` is not in `.env`; set it in `.env.dev` and run `just init-dev` (see *Deploy a signed Debug build*).
- **Old app icon still shows after redeploy** — SpringBoard caches icons; delete the app and reinstall, or reboot the device.

## The native Android client (`android-native/`)

Alongside the Capacitor shells there is a second, fully native Android client
under `android-native/`. It is a **separate Gradle build** with its own
application id (`ru.tinyops.turboist.native`), so it installs side by side with
the Capacitor app (`ru.tinyops.turboist`) instead of replacing it. Nothing in
`frontend/` is involved: no web bundle, no WebView.

Layout:

```
android-native/
├── settings.gradle.kts, gradle/libs.versions.toml   # one version catalog for every module
├── buildSrc/          # build logic: versionName/versionCode from the repo-root VERSION, string resources from frontend/locales/
├── app/               # Compose UI, DI wiring
├── core/model/        # domain types — a plain Kotlin/JVM module, no Android APIs
├── core/database/     # the on-device replica
├── core/network/      # HTTP client and wire DTOs
└── core/sync/         # the engine that reconciles the two
```

Dependencies run in one direction only: `app` → every `core/*`; `core/sync` →
`core/database` + `core/network`; those two → `core/model`. `core/model` is a
Kotlin/JVM (not Android) module, which is what keeps the Android platform out of
the domain types.

### Domain types (`core/model`)

`core/model` mirrors the Go domain (`internal/model/`) in Kotlin: the entities,
the enums, and one conversion helper. Three conventions hold everywhere in it:

- **Wire strings are the contract.** Every enum constant carries its server
  spelling (`Priority.NONE` is `no-priority`), and decoding is tolerant — a value
  this build has never heard of becomes the enum's `UNKNOWN` sentinel instead of
  throwing, so a server that grows an enum first cannot make an installed client
  refuse the whole payload. The sentinel is the one constant with an empty wire
  value; it is never offered in a picker and is never written back.
- **Two identities per row.** Every replicated entity has a device-assigned
  `localId` that never changes and a nullable `serverId` filled in once the row
  exists on the server, because a task created offline cannot wait for an id.
  References between replicated rows are local ids and are named with a
  `LocalId` suffix; the few ids that stay server-side (the singleton inbox, the
  label and project ids embedded in the settings blobs, which travel back to the
  server unchanged) are named plainly and documented where they appear.
- **Timestamps are epoch milliseconds.** The wire form is UTC with exactly three
  fractional digits (`2024-03-09T12:34:56.789Z`); `WireTime` is the only place
  that converts between it and the `Long` the replica stores. Writing is strict,
  reading accepts any ISO-8601 instant and truncates below the millisecond.

The version is not maintained in Gradle: `app/build.gradle.kts` reads the
repository-root `VERSION` file, so the client can never claim a version the
backend does not know about. `versionName` is that file with any pre-release or
build suffix removed (`1.17.0-dev` → `1.17.0`), and `versionCode` packs the three
components into one ascending integer (`major·1000000 + minor·1000 + patch`,
each component capped at 999). Those rules are covered by unit tests that
`just android-native-test` runs first.

### The replica (`core/database`)

`core/database` is the Room database every screen reads from. Nothing renders
from a response: a request updates the replica and the replica updates the
screen, which is what makes a screen work with no network and turns "loading"
into a property of data being absent rather than of a request being in flight.

It holds three kinds of table, and they are not the same thing:

- **the workspace** — contexts, labels, projects and their sections, tasks, the
  task↔label and project↔label edges, the task relation graph, task templates
  with their subtasks, and the three single-row documents (the user's settings,
  the installation's rules, the interface state that follows the user between
  devices). Table names are the server's, so the replica and the migrations under
  `internal/db/migrations/` read side by side; column names are the field names
  the API's payloads use, so a mapper reads as a straight assignment.
- **the full-text indexes** over task, project, label and context text. They are
  *content-backed*: they store no copy of the text and are maintained by triggers
  created with the schema, so a row cannot be written, edited or deleted without
  its index entry following — including through a cascade nobody called directly.
  A hand-maintained index would drift the first time a write path forgot it.
- **the bookkeeping**, which exists only on the device and is never uploaded:
  how far the copy has caught up, what this device has changed and not yet sent,
  and what the server refused.

Four rules hold across all of it:

- **Two identities per row.** The primary key is a `localId` the device assigns
  on the first write — including for a row created with no network in sight —
  and never changes. `serverId` is nullable, unique where present, and is what an
  incoming record is matched against: known server id updates in place, unknown
  one inserts. Keeping the local id stable is what keeps an open screen pointing
  at the same row after the server answers. **Every reference between replica
  rows is a local id**; the two exceptions are the singleton inbox and the ids
  embedded in the settings documents, which travel back to the server unchanged.
- **Deletes cascade exactly as far as the server's do.** There are no tombstone
  columns on either side, so the replica mirrors the server's own semantics: a
  context takes its projects and their tasks, a task takes its subtasks and the
  relation edges it is an endpoint of, deleting a board column leaves its tasks
  in the project, and a recurrence snapshot outlives the recurring task it was
  cut from. References are checked statement by statement, not at the end of a
  transaction, so a batch has to write the rows a task points at before the task
  — and an unresolvable reference is left `null` rather than written as a
  dangling id.
- **Enumerated columns store the server's spelling**, never an ordinal (a
  constant inserted into the middle of an enum would renumber every stored row)
  and never the Kotlin name. Reading is tolerant in the same way the payload
  decoders are: a value this build has never heard of costs one attribute of one
  row instead of the whole replica.
- **The settings documents are stored whole.** The server keeps each of them as
  one document rather than as a table and echoes writes back through the same
  document, so spreading one into columns would silently drop every key this
  build has not been taught about — and a write would then hand the server back a
  document with those keys missing.

**Unsent writes.** A user action is one transaction: the optimistic change to
the replica plus a row in the outbox naming the request to send. Each queued op
carries the UUID it will be sent under, so a retry after a lost answer is
recognised as the same write rather than performed twice, and an explicit queue
position, because a bulk action queues several ops within the same millisecond
and their order among themselves is what must not be lost. A row with a queued
write is *dirty*: an incoming record must not overwrite it, or the user's edit
would vanish and then reappear. A write the server *refuses* is a different
thing from one that failed to arrive — it leaves the queue for the quarantine,
with the server's own error code and the payload intact, so the queue behind it
moves on and the user can be told what did not happen.

The schema is versioned independently of the server's and exported to
`core/database/schemas/`, which is checked in: a change to the replica shows up
in review as a schema diff rather than as a crash on a device. That file is not
documentation — it is the starting point the next version's migration is written
and tested against — so a test reads it back and holds it to the database the
engine actually builds, comparing the identity hash the way the runtime does
when it opens an existing file on a device. Its tests run a real SQLite engine
on the JVM and ask it the same questions the app will — a schema is only as
correct as the engine says it is.

#### Lists are queries, and they are under contract

Every list the app shows — today, tomorrow, overdue, the week, the backlog, the
pinned tasks, the completion history, a project, a board column, a label, a
context, the daily plan and its board, the subtasks of a task — is a query
against the replica returning a `Flow`, so a screen redraws whenever the rows
behind it change and no screen has to be told to refresh itself. Alongside them
sit the counters the drawer paints as badges, which are the same predicates
asked for a number.

They re-implement the server's own list queries (`internal/repo/views.go`), and
two implementations of one set of rules drift apart on their own. So they are
pinned: one dataset under `testdata/sync-contract/` is rendered by both sides and
must produce the orderings stored beside it. The Kotlin half of that contract is
`ViewContractTest` in `core/database`, which writes the shared dataset into a
real SQLite database and compares every list against the same golden files the Go
test asserts. **A list that stops matching is either a wrong query or a rule that
genuinely changed — and a rule that changed is changed in the shared dataset
first, on both sides at once.**

Three rules hold across every list, and they are the ones easiest to get wrong:

- **Windows are half-open**, `[from, until)`. The last millisecond of a day is in
  that day; midnight belongs to the next one. A *day* is a fixed 24 hours,
  because an all-day task sits exactly on midnight; a *week* is seven calendar
  days from Monday, because it has to end on a Monday midnight even across a
  daylight-saving change.
- **Closed work is out.** A view of work to do matches open tasks only, whatever
  a completed or cancelled task's dates and plan state still say.
- **Membership is the predicate and nothing else.** A task in an archived project
  is due today like any other, the private flag hides a task from the shared
  read-only view and from nothing else, and a task that cannot be completed
  because something blocks it is still listed everywhere its dates put it —
  being blocked changes the checkbox, never the membership or the order.

The shared sort is: pinned first, then priority, then how recently it was pinned,
then how recently it was created. Priority is compared *before* pin time — a pin
lifts a task above the unpinned ones, it does not reorder the pinned ones among
themselves. The completion history is the one list that departs from it and sorts
by when work finished, because it is read as days.

The week list is the one place where the list and its badge answer different
questions. The list holds what the user planned for the week, what falls due
inside it, and the whole open subtree beneath each of those, so a parent is never
shown without the work it is made of. The badge counts only the tasks the user
chose: planning a task cascades down its open subtasks, and counting those would
let one decision eat several slots of the weekly limit.

Every list is required to be answered through an index rather than by reading
every task on the device, and the obligation is not left to a review:
`ViewContractTest` asks the engine how it *would* answer each shipped statement
and holds it to the index that list was designed around. Which index is picked
matters more than which indexes exist, because the engine has no statistics about
the rows on a device and costs an equality test as narrow — on this schema that
is backwards. Nearly every task is open, while a day, a project or the inbox
holds a handful, so a single index leading with the status column looks cheapest
for every list that mentions it and quietly pulls several of them onto a read of
every open task. There is therefore no index on the status column by itself: the
dated lists and the completion history use a two-column index that leads with the
date and carries the status behind it, which answers the range from the index and
settles "still open" without reading a row — and for the history supplies the
order too, so a page of it is read straight off the index instead of being cut
from a sorted copy of the whole history. Every other list keeps the selective
column it was always answered through.

The inbox is the one list that names its index in the statement, because there
the engine's guess cannot be corrected by choosing better indexes: the list asks
for tasks that carry an inbox id and are open, and only the second is an equality.
Room checks the name against the schema when it builds, so removing the index
breaks the build rather than the plan. The one thing not answered by a search is
the set of cancelled work the daily-plan board subtracts: it is walked off a
narrow two-column index without touching a table row, because the only index that
would search it directly is one leading with the status column, which costs every
other list far more than this saves.

Cutting a rendered list into the sections a screen draws — phases of the day,
calendar days, board columns, plan slots — is a separate, pure step in
`core/model` (`view/`). It never re-sorts and never produces a heading's text: a
group carries the value it was cut on and the screen turns that into words in the
reader's language.

### HTTP layer (`core/network`)

One Retrofit/OkHttp stack, built once at start-up, serves every endpoint group
(`auth`, `sync`, `tasks`, `projects`, `contexts`, `labels`, `templates`,
`settings`) from a single `OkHttpClient` — so the connection pool and, more
importantly, the token refresh are shared. Four properties are worth knowing
before touching it:

- **The server address is runtime state, not a build constant.** The product is
  self-hosted, so the address is typed in by the user and lives in a mutable
  `ServerUrl` holder that an interceptor reads on every request. Retrofit is
  built against a placeholder host that is never contacted. A bare host is read
  as `https://` — plain HTTP has to be asked for explicitly — and an address
  pointing into a sub-path keeps its prefix, so a reverse proxy mounting the app
  under `/turboist` works.
- **Every failure is one typed family.** Callers never see a status code: the
  `{error: {code, message, details}}` envelope, and a request that never reached
  a server, both become an `ApiException` whose subclass already says what to do
  — `Network` (retry later), `Auth` (refresh or sign in), `Business` (the server
  refused it; retrying the same request will fail identically), `Server` (worth a
  bounded retry), plus `SetupRequired`, `RateLimited` and the two "reseed from a
  snapshot" answers. `code` and `details` survive intact, so a refused completion
  still names the tasks that block it.
- **A 401 that says the access token aged out is repaired once and invisibly.**
  The request is retried with a fresh token; the refresh itself is serialized, so
  five requests rejected together cause one refresh, not five — the server treats
  a re-used rotated refresh token as theft. The token logic itself is not here:
  this module defines the two seams (`AccessTokenSource`, `AccessTokenRefresher`)
  and the session layer fills them in.
- **Mutations are replayable.** Every write to `/api/v1` goes out with an
  `Idempotency-Key`; a caller replaying a stored write passes its own key and it
  is left alone, and a request retried after a token refresh keeps the key it
  already had, so it can never execute twice. Writes are declared as
  `Response<T>` so the caller can read the marker the server sets on a response
  it replayed rather than produced.

Wire DTOs are a faithful transcript of the payload — server ids, wire strings,
timestamps as text — and the `mapping/` functions turn them into domain types.
That is where timestamps become epoch milliseconds, enums decode tolerantly, and
server ids are resolved to the replica's local ids through the `ReplicaIds` seam.
PATCH bodies rely on the three states the API reads out of a body: an absent key
leaves a field unchanged, an explicit `null` clears it, a value sets it — which is
why clearable fields are typed `Clearable<T>?` rather than plain nullable.

### The write path (`core/sync/write`)

Every user action is **one transaction**: the change the user expects is applied
to the replica and the request that will report it is queued alongside, in the
same commit. Split into two steps the pair would sometimes be half-done — a
screen showing an edit nobody will ever hear about, or a request for an edit the
screen never made — and neither half is recoverable afterwards, because nothing
would know it had happened. `OutboxWriter` is the only place that pairing is
made, and the per-domain write repositories (`TaskWriteRepo`,
`ProjectWriteRepo`, `ContextWriteRepo`, `LabelWriteRepo`, `TemplateWriteRepo`,
`SettingsWriteRepo`, `TroikiWriteRepo`) are the only callers.

`OutboxOp.kt` is the catalog: one sealed type per mutation the API offers, with
the endpoint mapping — verb and path — written both as a table in its own
documentation and as an `endpoint()` function the compiler proves is total. Two
properties of a queued write matter more than the rest:

- **The identity is minted when the write is queued, not when it is sent.** It
  travels with the request as its `Idempotency-Key`, so an answer lost to a
  dropped connection — or to the app being killed between sending and hearing
  back — is safe to ask for again. A key minted at send time would differ on the
  second attempt and the write would happen twice.
- **Payloads are wire-shaped except for references, which are local.** Values are
  already in the form the server reads; references to other rows are the device's
  own ids, because a row created without a network has no server id for hours
  and a reference recorded as "nothing yet" could never be repaired. The sender
  translates them at the moment of sending (`ReplicaServerIds`), by which time
  the rows they name have been created — the queue drains strictly in order, so a
  subtask is never sent before the task it hangs from. The exception is the ids
  inside the preference and rule documents, which are server ids in both
  directions by contract.

An edit carries only the fields it changed, which is what lets two devices edit
different fields of the same task and both keep their edit with no merge rule
anywhere; a request that resent every field would instead overwrite whatever
moved on the server while it sat in the queue.

The server owns the rules, and a write that gets one wrong is corrected by the
next page of changes. `ReplicaRules` repeats only the few whose violation the
user would otherwise discover much later, when the queue finally drained and the
change they made came back undone:

- **A task something still blocks cannot be completed.** Blockers are inherited
  down the subtask tree — a task whose parent is blocked is blocked too — except
  for one that lives inside the task's own subtree, which is the work itself.
  Completed *and* cancelled blockers release their dependents. With the whole
  relation graph on the device, finishing a blocker offline genuinely releases
  what it was holding up. The rule itself is written once, over the edges and the
  parent links, and both readings of it go through that one implementation: the
  padlock a list draws and the refusal this guard makes can therefore never
  disagree.
- **Two tasks cannot be linked twice, and a wait cannot close a loop.** The store
  keeps one row per pair and kind, and a loop of waiting would leave every task in
  it permanently unfinishable. Both are permanent refusals, so a link the device
  accepted would be drawn, queued, and thrown out whenever the queue next
  drained — by which time there is nothing on screen left to explain it. Refusing
  at the moment of asking means a link the device turns down never reaches the
  queue at all, and so can never end up as something the user has to clear by
  hand.
- **Planning carries downwards.** Parking a task parks its open subtasks at any
  depth and clears the day they were scheduled for; committing one to the week
  pulls them in and keeps their days. Finished subtasks and ones still in the
  inbox are left alone.
- **New tasks pick up the installation's automatic labels**, matched against the
  replicated rules. Labels are resolved, never invented: a name this device does
  not know is left in the request for the server to answer. Taking one off again
  has to be *stated*, not merely left out of the set — the rules run again on both
  sides and would put it straight back — so an edit that hands over the whole set
  is read for exactly that: a rule-attached label the task carried and the new set
  leaves out is one the user unticked, and travels with the write as such. Only
  what the task already carries counts; a rule that starts matching because the
  same edit changed the title attaches a label the user has never seen.
- **The pinned shelf and the daily plan's slots have room limits.** The shelf's
  size is the user's own setting; a slot's is the constant it starts at, because
  the room a slot earns as work is finished is a counter the server keeps and the
  replica does not carry — so the check is never stricter than the server's and
  the plainly hopeless attempt is answered immediately.
- **Moving a board column renumbers the whole board.** The server lifts the moved
  column out, drops it back in at the requested slot and renumbers every column
  to `0..n-1`, clamping a target past either end to that end; `boardAfterMove`
  restates that rule so the device can apply it before the request is sent.
  Writing only the moved column would leave two of them claiming one position
  until the next pull, and the board would visibly settle a second time under the
  user's hand. The two implementations are held together by a shared set of cases
  in `testdata/sync-contract/section-reorder.json`, replayed by
  `TestSectionReorderContract` on the server and `BoardOrderContractTest` here.

#### A whole selection changed at once

The API answers a whole selection in one request for three of the actions —
completing, moving and setting a priority — and `TaskWriteRepo.bulkComplete`,
`bulkMove` and `bulkPriority` each change every row and queue **one** op for it.
That matters beyond tidiness: twenty separate requests each carry their own
chance of a lost answer and their own replay, while one request is one key and
one outcome. Planning and deleting have no bulk endpoint, so a selection of
those is honestly one request per task rather than a batch op the server could
never answer.

Completing a selection is the one that decides something. A task an unfinished
task still stands in front of is left out of the batch entirely rather than sent
and refused: the device applies the same rule the server would apply item by
item, so the count reported on screen is the real one, and the request never
names a task that was going to come back refused. What was left out travels back
with the write, so the screen can say how many are still waiting rather than
reporting a smaller number without explanation.

Setting a priority leaves tasks out for the other reason. A project standing in
the daily plan decides the priority of everything open inside it and the server
turns a direct edit down outright, so such a task is dropped from the batch
before it is sent and counted separately — nothing stood in its way, the change
was simply never the user's to make, and the sentence the screen shows says so.

Grouping (`TaskWriteRepo.group`) creates the parent and adopts the children in
the same transaction, and rewrites each child to match its new parent in three
ways — where it lives, what it is tagged with, and how urgent it is. That is
what grouping means, and doing it here is what stops the screen showing the old
labels until the next catch-up. A group has to live in a context, a project or
one of its columns: the inbox holds raw capture and no structure, and a group
hung under another task would be a subtask, so both are refused before anything
is queued.

#### A repeating task ticked off with no connection

A repeating task is not finished when it is ticked off: it moves to its next
occurrence, and the run just done is written down as a separate completed row
that points back at it. Leaving both to the server would mean a tap that
visibly did nothing until the queue drained, so the device does them itself —
which only helps if it arrives at the same date the server would, because a date
that appears and is quietly replaced an hour later reads as a bug in the user's
plan rather than in the app.

The arithmetic is three rules, and `RecurrenceAdvancer` (`core/sync/write`, over
`org.dmfs:lib-recur`) is where they live:

1. The **anchor** is the occurrence the task still sits on when that is ahead,
   and otherwise the moment it was ticked off — so a task finished three weeks
   late resumes from today instead of marching through every run it missed.
2. The anchor is also where the series **starts**. A task carries a rule, not a
   start date, so every rule is read relative to the occurrence in hand.
3. The **zone** is part of the question. A rule repeats a wall clock, so a daily
   task at nine stays at nine across a daylight-saving change even though the
   elapsed hours are not twenty-four, and a task due on a day rather than at a
   time stays on local midnight.

A rule this build cannot read is answered with "I do not know", and the task is
left exactly as it was for the server to settle — a short honest delay instead of
a confident wrong date. Ticking the same repeating task off twice in one day is
one run, not two: the second tap changes nothing at all, which is the answer the
server gives as well, so nothing has to be taken back afterwards.

The two implementations are held together by a shared list of worked examples in
`testdata/recurrence-contract/fixture.json` — `(rule, where the task sits, when it
was ticked off, zone)` against the anchor and the next occurrence, covering
daylight-saving changes in both directions, month ends, leap days and series that
run out. The file is written by the server's own implementation
(`go test ./internal/service -run TestRecurrenceContract -update`) and answered by
`RecurrenceContractTest` here, so a rule that genuinely changes is changed there
first, on both sides at once.

When the completion is finally sent, the server records the same run — the same
task at the same moment, because the moment travels with the request — and the
catch-up recognises the device's row as that one and gives it the server's name
rather than adding a second entry to the history.

A row created here has no `serverId` until the queue drains. That absence is the
signal a screen uses to hide what needs a server-side address, a shareable link
most obviously.

### The read path (`core/sync/pull`)

The other half of the same engine: bringing the replica to the server's state.
There are only two ways to do that. A device that has never synced asks for a
complete copy (`GET /api/v1/sync/snapshot`); one that has asks for the changes
since the position it stopped at (`GET /api/v1/sync/changes`), a page at a time
until the server says there are no more. `ReplicaApplier` writes either kind —
a complete copy and a page of changes decode to the same batch, so there is one
applier and not two chances to disagree about what a task is.

Four rules govern the writing, and every one of them is about *when*:

- **A batch and the position it moves to commit together.** The cursor is stored
  in the same transaction as the records it accounts for, so a process killed
  mid-catch-up resumes from the last page that actually landed. Replaying an
  overlapping page costs nothing: every record is its own current state, so
  writing it twice leaves the same row.
- **Records are written in the order the schema needs**, not the order they
  arrived in — a project after the context it names, a subtask after its parent.
  References are checked per statement, and where a batch carries a child ahead
  of its parent the leftovers are passed over again until nothing more can be
  placed. A page that still names a row the device does not hold is abandoned
  whole, cursor included, and the device asks for a complete copy instead, which
  is consistent by construction.
- **A row with an unsent write against it is left alone.** The user is looking at
  a change the server has not been told about; overwriting it would make their
  edit vanish and come back. The write's own send, and the read after it,
  converge the row.
- **A deletion beats that.** No queued write against a deleted record can ever
  land, so the row goes and its queued writes move to the quarantine as
  `target_gone` — the answer the server would give, arriving sooner.

A complete copy also takes out what it does not mention: it names every record
the server holds, so anything else is either gone or older than the window the
copy was cut at. Two things survive that sweep — a row created on this device,
which the server has never heard of, and a row with an unsent write against it,
whose fate its own send will settle.

Two refusals are not errors. `sync_epoch_mismatch` means the history was
replaced (a backup was restored) and `sync_cursor_expired` means the part still
needed has been pruned; both mean the stored position is meaningless, and both
are answered by taking a complete copy. **The unsent queue and the quarantine
survive all of it** — they are the user's own changes, not a copy of the
server's data. Losing the network is not a refusal at all: `SyncPuller.pull()`
answers with a value (`Applied` / `Offline` / `Refused`) and never throws, the
replica is left untouched, and every screen goes on reading it.

`SyncMutex` is the one lock the cycle has, and both halves take it. A cycle is
always **send first, read second**: sending first keeps the server from
answering with state that predates the device's own queued writes, and reading
after picks up everything those writes cascaded into. Run the two together and a
read can land between a write leaving and its answer arriving, writing stale
rows over the ones the user just changed. Requests to catch up arrive in bursts —
a screen refreshing, a background job waking, an event from the server — and a
catch-up may answer a request only if it began after that request was made.
Everything already waiting when one starts shares its single answer, so a burst
costs one round trip; a request made once a catch-up is already on the wire gets
its own, because that catch-up read the server before the question existed.

### Sending the queue (`core/sync/drain`)

`OutboxDrainer` empties the queue against the ordinary mutation endpoints —
there is no batch-push endpoint, so every invariant stays enforced exactly once,
in the Go service layer. **One write at a time, in the order they were made.**
A task moved into a project and then completed is a different outcome from a
task completed and then moved, so a write that cannot be sent stops the queue
rather than being stepped over.

Each request goes out under the write's own id as its `Idempotency-Key`. The key
was minted when the write was queued, so an attempt whose answer was lost — a
dropped connection, or the process killed between sending and hearing back — is
safe to repeat: the server replays what it already decided (`X-Idempotent-Replay`)
instead of doing the work twice. This is why a write is marked in flight before
it is sent and removed only once an answer arrives; the worst case is one extra
request, never one extra task.

`OpSender` turns a queued write into its request, and is the only place local
ids become server ids. A create's answer names the row it brought into
existence, and that id is written onto the local row **in the same transaction
that takes the write out of the queue** — so the writes queued behind it resolve
when their turn comes, without any payload ever being rewritten. Nothing else in
the answer is read: a cascade, an advanced recurrence, an automatically applied
label are all picked up by the catch-up that follows.

A delete is the one write that cannot be resolved that way, because its subject is
gone by the time it is sent: the row leaves the replica in the same transaction
that queues the write. So the id the server knows the row by is written into the
queued write while it is still knowable, and the request is built from that. A row
the server has never heard of carries none, and the create ahead of it in the
queue supplies it. Without this every deletion of an already-replicated task,
project, column, context, label or template would be set aside as unsendable and
the thing the user threw away would live on the server for good.

How a failure is answered is the whole of the design:

| What happened | The queue | The write |
|---|---|---|
| Nothing reached the server | stalls, with a doubling wait capped at five minutes | keeps its place |
| Too many requests (`429`) | stalls the same way | keeps its place |
| The server broke (`5xx`) | stalls | retried on later cycles, a bounded number of times, then set aside |
| The server refused (`4xx`) | carries on | set aside at once |
| The credentials were refused | stops, intact | keeps its place |

A write is never sent again under a fresh key and never quietly dropped. "Set
aside" means the quarantine: the payload, the mapped reason (a cap that was
reached, `target_gone` for a row the server no longer has, `task_blocked` for a
completion something still stands in the way of) and the moment, kept for the
user to read and discard — `UnsentChanges` is that surface. A blocked completion
also keeps the ids of the tasks that were in the way, because the refusal's
wording is the same sentence every time and the ids are the only part that says
which tasks to go and look at — they are in that one answer and nowhere else
afterwards. Waiting for a network never counts against a write; only the server
actually breaking does, because a week somewhere with no signal is not a broken
write.

Discarding one of those usually closes a note and nothing more: the change never
happened on the server, and the row it was made against has already been corrected
by a catch-up. A refused *creation* is the exception — its row exists on this
device and nowhere else, and with the write thrown away nothing will ever bring it
back — so the row goes with the refusal, in the same transaction, and only while
it is still nameless.

**Nothing is rolled back by hand.** The optimistic change a refused write made is
corrected by the next catch-up, which carries the server's own version of those
rows. A second, hand-written answer to the same question would eventually
disagree with the first.

`SyncCycle.sending` is the cycle every trigger runs: drain, then pull — and the
pull happens **only when the queue emptied**. That is a correctness rule, not an
economy: a row created here and not yet sent has no id the server would
recognise, so a copy taken while its creation is still queued would come back as
something new and the device would hold the same thing twice. A queue that
stopped therefore ends the turn, reported as offline or as refused depending on
what stopped it, and the next turn tries again.

### When it syncs (`core/sync/trigger`)

Five reasons, one door. `SyncScheduler` is the only way anything asks for a
cycle, and what reaches it is a *request*, not a run: a request opens a 500 ms
window, everything arriving inside it is folded into the same cycle, and the
cycle happens when the window closes. Bursts are the normal case — one edit on
another device fans out into several events, and a phone waking up gets a
network, a foreground and a reopened stream inside the same second — so
without the window the app would spend several radio wake-ups learning the same
thing once. The window is never extended by later arrivals, so a steady stream
of events syncs once per window rather than being starved by the next one, and a
request made while a cycle is already on the wire opens the next window instead
of being answered by a cycle that read the server before the question existed.

**While the app is in front of the user**, the server's existing event stream
(`POST /api/v1/events/ticket`, then `GET /api/v1/events`) is held open with
`okhttp-sse` and every event asks for a catch-up. Which area the event names is
never read: the reaction to all of them is the same delta pull, which covers
areas the event did not mention, so echoes of this device's own writes need no
suppression — applying a record the replica already holds writes the identical
row. A ticket is single-use, so every reconnect handshakes again and the
transport's own retry is never used; a reconnect after a gap asks for a catch-up
of its own, because whatever changed during the gap arrived as silence. Silence
is also how a dead socket looks — a phone whose radio slept, a proxy that
dropped an idle connection — so the stream's read timeout sits above the
server's 25-second heartbeat and below anything a user would call stale, and
reconnect delays grow to 30 seconds while a stream that lasted ten seconds
resets them.

**While it is not**, there is no stream at all: a persistent connection in the
background drains the battery to learn about tasks nobody is looking at, and the
platform suspends it anyway. Background freshness rides on WorkManager instead —
one periodic job every 15 minutes and one expedited job for the moments that
change the odds a sync is worth doing, both requiring a network. Each is
enqueued under a unique name, which is what turns "ask for a sync" into "make
sure a sync is pending": without it every foreground and every network flicker
would leave another job behind. The periodic job is kept rather than replaced,
or an app opened every ten minutes would restart the interval forever and never
reach the end of one. Workers are built from the dependency graph at the moment
they run (`SyncWorkerFactory`, with WorkManager's automatic start-up entry
removed from the manifest so the graph exists first), because the process that
scheduled a job is often gone by the time it runs.

The two remaining reasons are the ones a device is best placed to act on. The
platform's default-network callback fires when the device gets a usable network
back — the radio is awake and the user is still standing where they made their
changes — and asks for the expedited job; a handover from one network to another
is not a return and is not acted on. Coming to the front asks for the same job,
through the schedule rather than straight at the engine, so that an app sent
away again immediately still gets its catch-up. And the user can always ask:
pulling a list down runs a cycle directly and holds the spinner until it is over,
since a list is a query over the replica and there is nothing else the gesture
could mean.

Both jobs wait for a network, and which network counts is the device's own
choice: a phone told in settings not to sync on mobile data asks the platform for
an unmetered connection instead of any connection, because the system rather than
this app knows which connections the user considers expensive. The choice is read
before the triggers are armed, or the first job of the process would be enqueued
under the wrong constraint and keep it — a job's constraints are fixed when it is
handed over.

Nothing runs while signed out — no stream, no jobs — because a device with no
session has nothing to ask the server and no right to ask it, and signing out
drops both jobs. "Signed out" means the stored credentials were read and there
were none, never "they have not been read yet": a process the system started to
run a sync job begins in the second state, and treating it as the first would
make that process cancel, as its first act, the very job that started it —
throwing away the catch-up and rebuilding the schedule from scratch, so a device
woken from a dead process loses the sync it was woken for. A cycle answers with
a value for everything expected of it, and
the one that fails unexpectedly is logged and turned into a refusal rather than
allowed to end the loop that every automatic reason goes through.

### Saying how current the screen is (`app/.../sync`, `app/.../unsent`)

Every screen is a query over the replica, so nothing on one can say whether the
replica is current. That is a property of the app, and it is said once, above
the whole graph, rather than in each screen.

**What the shell knows.** A status is assembled from what the device already
records, so it cannot disagree with the queue underneath it: how far the replica
has caught up is a row of the replica, what has not been sent is the queue
itself, and what the server refused is the set-aside pile. The one thing that
has to be observed as it happens is whether a cycle is on the wire, so the cycle
is wrapped once where the app assembles it and reports what it is doing. Whether
a cycle is *running* is kept apart from how the last one *ended*: a new attempt
does not undo what the previous one concluded, so a device out of coverage keeps
saying so while it retries instead of flickering once per attempt.

**Two levels of interruption.** A running cycle draws a hairline under the top
bar and nothing else — it happens constantly, it needs no answer, and a banner
for it would be a banner that is always there. A strip is drawn only for the
three states that change what the user should believe about the screen: the
server cannot be reached (with the moment the copy on screen was last level with
it, because a minute old and a week old are not the same thing), a turn the
server did not complete, or work of the user's that has not gone out yet. An app
that is keeping up says nothing at all. Retry is offered on the first two, and
not on the third: a queue that is simply on its way out has nothing to retry, and
the button would do nothing. Retrying runs the same cycle every automatic trigger
runs — what the button buys is the cycle the user is standing there waiting for,
not a different, harder request.

**The pile that needs a person.** Refused writes never expire and are never
cleared by the app, so there is a screen for them, reached from settings and
marked on the drawer entry that leads there whenever it is not empty. Each row
says what the change was, what it was for and why it was refused — the op name
and the payload are the right thing to store and the wrong thing to show, so the
row the change was aimed at is looked up in the replica and its title is carried
alongside. A title that will not resolve stays absent rather than being invented:
the commonest refusal there is comes from a row deleted somewhere else. A blocked
completion also names the work that was in the way, resolved from the ids the
server sent, because the refusal's own wording is the same sentence every time
and the ids are the only part that says where to go and look.

Every write the API offers can end up on that list, so every one of them has a
name a person can read; a test walks the whole catalog, which is what makes a
write added tomorrow fail there instead of turning up as a blank row on a phone.
A write queued by a build this one cannot read is still listed under a name of
its own — a change the app cannot describe is exactly the kind that must not
vanish without the user being told.

Discarding closes the note, not the change: the change never happened on the
server and the row it was made against has already been corrected by a catch-up,
so there is nothing to undo. Clearing the whole pile asks first, because it is
the one action here that cannot be taken back. The queue that is still going out
is listed underneath, read-only — it is shown so that "it has not synced yet" is
something a person can look at rather than something they have to trust, and it
carries no action at all, since throwing away a write that will land is exactly
the silent loss the screen exists to prevent.

### App shell (`app`)

One activity hosts one Compose tree; everything above the screens is navigation,
theme and dependency wiring.

**Navigation** is `navigation-compose` with typed routes: one Kotlin type per
destination (`navigation/Routes.kt`) instead of format strings, so a destination
and its arguments are checked by the compiler. The destination set mirrors the
web client's URL space one for one — today, tomorrow, week, next week, inbox,
projects and a project, labels and a label, a context, completed, troiki,
search, settings, a task — plus the three authentication screens. What fills
each destination is handed to the graph rather than written into it
(`navigation/AppScreens.kt`), so a check about wiring can compose the whole shell
with placeholders instead of dragging a database and a sync engine along.

**Two graphs, one gate.** `session/SessionState.kt` has exactly three states —
resolving, logged out, logged in — and `graphFor` maps them onto a splash, the
authentication graph, or the app graph. Screens never see anything finer: which
server, whether a second factor is pending, whether the last refresh failed on
the network rather than on the token, all of that stays inside the
authentication implementation behind the `SessionStateSource` interface. Keeping
the two graphs separate means a signed-out user has no back stack into the app.

**Drawer.** The destination list is a catalog (`navigation/DrawerDestination.kt`)
rendered as-is, so adding a destination to the catalog makes it appear; only
argument-free destinations belong there, since a project or a label is reached
from its list rather than from a static menu that would have to invent an id.
Counters come from a `DrawerCountsSource` and are nullable: unknown means "draw
no badge" rather than zero. Below 840 dp the drawer is a modal overlay; above it
the drawer becomes part of the layout and the menu button disappears.

**A screen may own its own chrome.** The shell draws a top bar with a menu
button and the name of a destination, which is right for a list and wrong for one
task: a detail screen needs a back arrow, a pin and an overflow, and a back arrow
drawn *underneath* the shell's bar — which is what the task screen used to do —
reads as two navigations stacked on each other. So the shell asks whether the
destination on screen brings its own bar and draws none of its own when it does.
The capture button is hidden the same way and for the same reason, since it sits
in the corner such a screen draws in; the capture *sheet* stays composed either
way, because a share from another app opens it without anyone having touched the
button. The task screen is the only destination that takes the offer, and it is
handed the jump-pair control by the graph, since that control navigates and where
a destination leads is the graph's business.

One entry is conditional. The daily plan is a preference the user keeps
(`troikiEnabled` in the replicated preference document), and a workspace that does
not use it is not offered it — the drawer entry is absent, a project cannot be put
into a slot from the project screen, and no project row is marked as standing in
one. It is read off the replica exactly as the calendar's own switch is, so a
device that has not synced yet answers "off" and starts offering the plan the
moment the preference arrives. A project that already stands in a slot still fixes
the priority of its work, because that is what the server does regardless of the
preference — so the priority picker on such a task stays locked and says why.

**Theme.** The palette is the web client's, transcribed rather than invented:
the custom properties in `frontend/src/routes/layout.css` converted from OKLCH to
sRGB and filled into Material's roles (`ui/theme/Color.kt`), which is a warm
off-white page carrying a single red accent, with every grey pulled towards that
hue so nothing beside it reads as blue. Android's wallpaper-derived scheme is
deliberately **not** taken: a system-themed app feels native, but a phone whose
wallpaper is a grey photograph turns the same lists that are warm and red on a
laptop into a grey-on-grey page, and Turboist is one product with two front ends.
The product's orange (`#e2580e` — the launcher icon and the web client's
`theme-color`) stays in the scheme as Material's third accent, and the window
background is painted with the same page colour in `res/values*/colors.xml` so a
cold start does not flash the platform's white first.

The colours that stand for something rather than fill a role — the three priority
levels, the mark on a repeating task, the badge on a write still queued on the
device — travel beside the scheme in `ui/theme/Accents.kt`
(`TurboistTheme.accents`), and are the web client's own Tailwind steps: red means
urgent on both clients or the colour is not carrying anything. The two that are
read as words move a step lighter at night, exactly as the web's `dark:` variants
do. Unit tests pin both schemes to the transcribed tokens, check the five
container surfaces stay in Material's order, hold every foreground/background
pair the app puts text on to 4.5:1, and hold a filled accent's own label to the
3:1 bar WCAG sets for controls — which is where white on the web client's red
lands, and matching it is the point. Light or dark follows the phone unless the
device has been told otherwise in settings; the override is read at the root of
the content tree, so it covers the sign-in screens too and does not wait for a
session, a server or a sync.

**Two ways to name one task.** Inside the app a task is addressed by the id this
device holds it under, so a task written down while offline opens like any other.
A link from outside carries the server's id — the one the web client puts in its
URL — and arrives at a destination of its own, which shows the same screen and
resolves that id against the replica. The two are separate destinations rather
than one with two fields because they answer different questions: one always
names a row on this device, the other names a row the device may not have
replicated yet.

**Deep links.** `turboist://task/<server id>` opens the same task the web client
shows at `/task/<id>`. Only the app's own scheme is claimed: the server host is
whatever the user connected to, so it is not known at build time and cannot be
verified without an association file on it. The activity is `singleTop`, and a
link arriving while it is alive is forwarded into the graph rather than dropped.
It is the only destination a link from outside can land on; the other two ways in
from outside — the launcher's own shortcut and a shared piece of text — open the
capture sheet rather than a destination.

A link can also arrive when there is no graph to answer it: on a cold start, or
while nobody is signed in and the only tree on screen is the sign-in one, which
holds no task screen at all. The activity parks it (`navigation/PendingTaskLinks.kt`)
the same way it parks a capture asked for from outside, and the app shell follows
it the moment its graph exists — as a single top entry, so a link the graph also
read out of the launch intent still leaves one screen on the stack. One link is
held and a second replaces it, and it is cleared once followed, so putting the app
away and coming back does not jump to a task the user has already left.

**How the shell is checked.** The drawer walk is a JVM test, not a manual pass:
the shell is composed off-device at phone width, every catalog entry is tapped in
turn, and both the destination that opens and the title bar that follows it are
asserted. The coverage checks read the graph the composition actually built
rather than a list of routes repeated in the test, so a destination added to the
catalog but never registered — or registered with no title — fails there instead
of on a phone. The same test hands the running shell a task link and follows it
to the task screen, and the deep-link constant is held against the manifest's
intent filter by parsing the manifest, so the claim and the route cannot drift
apart.

**Strings.** Every user-visible string is a resource, and product wording is
written once — in `frontend/locales/{en,ru}.json`, the files the web client is
translated from. A `buildSrc` task projects them onto Android resources on every
build: it flattens each locale file (`a.b.c` becomes `a_b_c`), escapes the text
for the resource format and writes `values/strings.xml` (English) and
`values-ru/strings.xml` into a generated resource directory under `app/build/`.
Those files are never edited by hand — they are rewritten from the locale files,
which is also where a wording change belongs.

Named placeholders become positional format arguments: each distinct `{name}`
takes a position, numbered from 1 in the order it first appears in the *English*
message, written as `%N$s`. Translations reuse the English positions, so one call
site passes its arguments in one order whatever the device language is. A
`{count, plural, …}` message becomes `<plurals>`, with any wording around the
choice folded into every item and `#` written as the plural argument's position
— which is why the count is passed twice, `getQuantityString(id, count, count)`.
Exact branches such as `=0` are dropped, because Android picks an item by the
language's plural category and cannot test a number for equality.

A key the translation lacks is simply absent from its file and Android falls back
to the English resource, so a partial translation is not a build error. A key
only a translation has is dropped: English decides which strings exist.

Strings the native client alone shows are hand-written in
`app/src/main/res/values/strings_native.xml` and prefixed `native_`, which keeps
them clear of the shared wording. The generator fails the build if a generated
name lands on a name that file already declares.

Those strings need translating by hand as well, and for a long time none of them
were: a Russian phone showed English words in the middle of an otherwise
translated screen, because the generated wording had always been translated and
the hand-written wording never had. `values-ru/strings_native.xml` now carries
the task screen and the repeat editor — every string those two surfaces reach.
The rest of the file still falls back to English and is worth a pass of its own.
A name absent from a translation falls back to `values/`, which is why the
priority codes and the em dash that stands in for an empty field are not
repeated: they read the same in both languages.

### Session and sign-in (`app/.../auth`)

Everything between a fresh install and a session that survives a tunnel lives in
one place, `nativeapp/auth/`. The session is owned by a single object precisely
because the rotating refresh token is spendable exactly once: two code paths
refreshing at the same time would present the same token twice, which the server
reads as theft and answers by killing the session.

**Connecting.** There is no compiled-in host, so the first screen asks for one.
The address is normalised (a bare host becomes `https://host`, and it always ends
in a slash so endpoint paths append rather than replace), checked against the
unauthenticated `GET /api/config` before anything is typed into it, and only then
written to a small DataStore file. A failed check restores whatever address was
configured before, so a mistyped change cannot disconnect a working install.
**Plain HTTP is refused in anything that ships.** The shipping manifest keeps
`usesCleartextTraffic="false"`, so an `http://` address would fail at the
transport with an error that explains nothing; refusing it on the connect screen
turns that into a sentence. The debug manifest overrides that one attribute, so a
developer can point the app at a local server that terminates plain HTTP —
`10.0.2.2:18080` from the emulator reaches the host's loopback. There is
deliberately no second flag: the connect screen asks the platform whether
cleartext is permitted (`NetworkSecurityPolicy`), so the rule it applies and what
the transport will actually send can never drift apart.

**Setup or sign-in** is the server's answer, not a choice offered to the user.
There is no endpoint for the question: the setup gate short-circuits the whole
versioned API with `setup_required` until an account exists, so the client makes
one unauthenticated versioned call and reads only its refusal. A sign-in attempt
against an instance that turns out to have no account moves to the setup screen
instead of showing a failure. TOTP is a step of the sign-in screen rather than a
screen of its own — the ticket is short-lived, single-use and held in memory —
and a recovery code goes to the same endpoint as a code from the authenticator.

**Tokens.** The access token lives in memory only; the rotating refresh token is
the single persisted secret, sealed with an AES-GCM key generated inside the
platform keystore and stored as ciphertext beside the server address. The key is
deliberately not bound to the screen lock, or every background sync would become
a sign-out. A blob that will not open is treated as no token at all — the key can
genuinely disappear — so the user is asked to sign in again rather than met with
a crash on every launch. Refresh goes in the request **body** (`client_kind:
"android"`), every rotation is stored *before* the new session is used, and the
`AccessTokenRefresher` the HTTP layer calls is this same owner, behind one lock.

**Launch.** What the first screen *is* cannot be decided by a screen, so the
application object starts the resolution and the shell shows a splash until it
answers. The rule, in one pure function (`BootDecision.kt`):

| Stored state | Launch ends on |
|---|---|
| no server address | connect screen |
| refresh renewed | the app |
| refresh **unreachable** | the app, on the replica, with a notice |
| refresh **rejected** | sign-in screen; replica and queue kept |
| nothing stored, server has no account | setup screen |
| nothing stored, server has an account | sign-in screen |

The third and fourth rows are the whole point. A server that cannot be reached is
not a server that said no: sending a user with a perfectly good token to a
sign-in screen would also deny them a sign-in — the server is the thing they
cannot reach — and lock them out of data already on the device. A real rejection,
by contrast, is acted on at once; but the replica and the queue of unsent writes
survive it, so signing back in resumes where the user was.

**Leaving the offline launch** matters as much as entering it. Such a launch
keeps the refresh token but has no access token to show for it, and nothing in
the app would ask for one on its own — so the unproven session ends by two
routes, both of which retire the notice and refresh the data:

- the platform's default-network callback fires when the device has a network
  again, and the session is re-checked once per event (conflated, because
  spending a single-use token per handover would be a round trip and a risk for
  nothing). A network that still cannot reach the server changes nothing: the
  app stays on the replica and the next event tries again;
- any authenticated request that goes out meanwhile carries no token, so the
  server refuses it for want of credentials (`auth_invalid`) rather than because
  of them. `AuthInterceptor` treats a rejection of a request that carried *no*
  token as repairable, mints one through the same single-flight owner, and
  retries — the caller never learns anything happened.

**Passkeys** are the second way in, never the only one: the password stays the
recovery path, and an assertion is already two factors — the authenticator holds
the key and verified the user — so it never leads to the code step, even on an
account with a second factor configured. The sign-in screen offers the passkey
button only when the instance reports both that the feature is configured and
that something is enrolled, since a button that can only end in an explanation is
worse than no button. Enrolment and the credential list live behind **Settings →
Passkeys**, which never caches: a stale answer to "which devices can sign in to
this account" is worse than no answer. The ceremony options travel to the
platform and its answer travels back as the JSON the server wrote and verifies,
untouched — decoding and re-encoding base64url is how the URL alphabet gets lost
and a signature stops verifying. What the server has to publish before any of it
works on a device is in [passkey.md](passkey.md);
`just android-native-passkey-preflight` checks all of it against an instance
brought up as a relying party, which is everything short of the platform prompt
itself.

**Signing out** revokes the session server-side (best-effort — a device that
cannot reach its server must still be able to leave it), forgets the token and
empties the replica, the outbox and the quarantine. Because that discards work
the server has never seen, the queued and refused ops are counted first and the
user is asked, with the number, before anything is touched; declining leaves both
the session and the queue exactly as they were. The server address is kept: it is
configuration the user chose, not a credential they proved, so signing back in
does not start by typing a URL.

### Task lists (`app/.../tasks`)

The dated screens — today, tomorrow, the week, and the planning view — are the
same list asked four different questions, so they are built from one set of
parts.

**A list is a query, not a fetch.** `TaskListRepository` turns a view query into
a `Flow` of tasks and joins on the three things a stored task row does not carry:
the labels it is tagged with, how many relations it has, and whether anything
still open stands in its way. Those three are standing queries of their own,
combined with the list rather than re-issued for the rows currently on screen, so
tagging a task or finishing a blocker redraws the list without the queries behind
it being torn down. A screen therefore has no loading state and no refetch: a
change made here and a change that arrived from another device reach it by the
same route, the moment the rows change.

**Sections are decided before anything is drawn.** `TaskListSections.kt` flattens
tasks into rows — nesting a subtask under the parent it was listed with, drawing
one whose parent is out of frame as a root — and cuts them into blocks: the
overdue block above a day, the phases of a day, the calendar days of a week, or a
block with a fixed name. Each block carries a stable key so a redraw does not
move the scroll position, and a fixed-name block may carry its own wording for
when it holds nothing. A screen either has one message for the whole list being
empty or blocks that name their own emptiness — the planning view is the second
kind, because its two blocks are the two ends of one decision and the place work
is moved *to* has to stay on screen.

**One presenter, four view models.** `TaskListPresenter` is a plain object driven
by a scope rather than a view model: it holds what the screen renders, the
selection, the refresh spinner, and the one thing a list says back. Every action
it takes is optimistic, because the write path applies the change to the replica
inside the same transaction that queues it for the server — a ticked task leaves
every list it was on before the device has spoken to anything. The exception is a
refusal: a task with an open blocker cannot be completed, the device repeats that
rule, and saying so immediately is the whole reason for repeating it. Only that
refusal has words of its own; every other refusal and every unexpected failure
say the same thing — the change did not happen.

The four view models differ only in the query behind them and in what they group
on: today puts the overdue work first and then cuts the day into its phases,
tomorrow cuts into phases with none of them highlighted, the week cuts into
calendar days, and the planning view draws the backlog and the week side by side.
They read a ticking clock rather than the time of their construction, because a
screen left open has to notice midnight and the turn of a phase on its own —
neither arrives as a change to the replica.

**The row** carries what is worth seeing without opening the task: priority as
the colour of its control, the due date, its labels, its project, and markers for
recurring, complex, private, pinned, postponed-often, entangled, planned and
parked. Two of its states are rules made visible. A task something still blocks
shows a padlock instead of an empty box and cannot be ticked at all — a greyed
box would still read as "tick me later". A task the server has never seen shows
that it is waiting to be sent, which is exactly the set of rows with no server id
yet; it disappears by itself when the queue drains, with nothing to clear.

**Gestures.** Pulling a list down does not reload it — it cannot, it is a query —
so the gesture asks the sync engine to catch up and the rows change because the
catch-up wrote to the replica underneath them. Swiping a row one way completes
it, the other way moves it between the week and the backlog; neither dismisses
the row, which leaves the list only if the change took it out of the query. The
swipes are unreachable to an assistive reader, so the same two actions are also
named actions on the row.

**Acting on a selection.** A long press starts selection mode; tapping rows adds
and removes them, a heading offers its whole block at once, and un-picking the
last task leaves the mode rather than leaving a bar hanging over nothing. The
bar carries the count, the three actions a selection is usually made for —
finish it, file it somewhere, gather it under one new task — and an overflow
holding the priority levels, the two planning decisions and deleting. Grouping
appears only from two tasks up, and deleting is the one action asked about
first, because it is the only one that cannot be undone by doing the opposite.

Each action is applied to the replica and queued in the same transaction as any
other write, so the tasks leave the list before the device has spoken to
anything — which is exactly why an action says what it did. The rows it worked
on are gone, and the count is the only evidence left that the gesture landed. A
completion that had to leave part of the selection alone says how much is still
waiting on unfinished work.

**What is checked.** The grouping, the announcement gate and the presenter are
plain JVM tests — no Compose, no database, no server. The row is composed for
real off-device and read back through the descriptions an assistive reader would
announce, one test per state it can be in. One screen is driven end to end
against a stand-in for the replica that behaves the way the write path does, and
the sync engine behind it is a counter that has to stay at zero: a task ticked
off leaves the day at once, with nothing asked of a server. The selection bar is composed
for real too and driven by the descriptions an assistive reader would announce,
and the selection actions are checked against a port that records what it was
asked for — one call carrying the whole selection, never one call per task.

### Task detail (`app/.../tasks`)

The screen that shows one whole task, with every field on it editable while the
device is on a plane.

**What the task is stays on the page; what can be changed about it waits behind a
control.** A task carries a dozen fields, and laying every one of them out open
made a page nobody could read: five wrapping rows of chips, every label in the
workspace whether or not the task had it, seven outlined buttons in a block that
wrapped differently for every task, and four timestamps drawn with the same
weight as the fields above them. So the page now shows what the task *is* — where
it lives, what it is called, when it is due, how it is planned, what is under it,
what it links to — grouped into cards, one card per question, with the heading
outside the card saying where one question stops. What can be *changed* opens
from the row it belongs to: a choice out of several is a list, and a list is
legible in a sheet and a wall in a column. The sheets are hosted by the screen
rather than by the rows that raise them, so exactly one can be open at a time —
two sheets over one another is not a state a single back press gets out of.

The two exceptions are deliberate. The priority stays open as a connected group
of four, because it is the field most often changed here and the level reads at a
glance only when the others are beside it; the chosen one is filled with its own
signalling colour, since that colour is what a user reads first on every list in
both clients. And the record of the task — how often it was put off, when it was
written down, last changed and finished — is folded into a block whose summary
line carries the two facts anyone actually wants from it, because it is the part
of the screen a person reads once a month.

Pinning moved to the top bar. It is not a field: a pin is a place on a shelf with
a cap on it, and the write is what refuses a pin over that cap, so a switch would
promise something the product does not always keep.

**It reads the replica, like every list.** `TaskDetailRepository` combines the
task row, its whole subtree, the label taggings, the relation edges and the
named workspace rows into one value, so the screen has no fetch, no error state
and no retry. Placement is resolved to names there rather than in the screen: a
task filed in a column names its project through that column even when the task
row itself does not.

**Blocked work is named, not counted.** A list row only needs to know that
something is in the way, so it draws a padlock from a count. This screen has to
say *what* is in the way and let the user go there, so the blockers arrive with
their titles and the same inheritance rule the guard uses — a subtask inherits
what blocks the work above it, except for a blocker that lives inside its own
subtree, which is the work itself.

**One field at a time.** Every editor builds a patch carrying exactly the fields
that changed, and an editor handed the value the task already has sends nothing
at all. That is not tidiness: two devices that changed different fields of one
task both keep their change only because neither write mentions the other's
field. Screens re-emit values freely — a text field reports its content when it
loses focus whether or not it was touched — so the comparison happens before
anything is queued rather than after. The date arithmetic behind it is pure and
tested on its own: moving a timed task to another day keeps its time of day,
emptying a date takes the time with it, and a task with no date has nothing to
put a time on.

Planning is the one field that is not a patch. Committing a task to the week or
parking it carries the open work beneath it, so it goes through the planning
write rather than through a patch of one column.

**How the task repeats is chosen from five names or written out.** The stored
form is calendar notation, which is exact and unreadable, so five named choices —
every day, every weekday, weekly, monthly, yearly — write it for the user and
apply on the tap. They are a list in a sheet rather than a run of chips on the
page: exactly one of them is true at a time, which is what a list of radio
buttons says and a wrapping row of chips does not, and the row was three lines
tall on a phone for a field most tasks leave at "never". The sixth is the
notation itself, because a product that offers only five is one a user with a
sixth need has to leave, and because a rule written on another client has to
survive being opened here: one that has no sentence in this app is shown as it
was written rather than described wrongly or thrown away. A hand-written rule is
checked as it is typed by the same calculator the completion path uses — one that
is not a rule cannot be stored, since it is one the device could not have acted
on either — and is applied on a deliberate tap, because every prefix of a good
rule is a bad rule. Underneath sits the date the rule would move the task to
next, which is the answer people are actually after and the thing that makes the
notation usable at all.

**The description is markdown to read and text to write.** The subset is the same
one the web client renders, parsed by the same rules, because the two clients
show each other's text. Anything outside the subset stays visible as the
characters that were typed — a description is the user's own writing, and
swallowing part of it is worse than showing a stray asterisk. Links become
tappable only when the target is plainly a web address, a mail address or a path
inside the product: descriptions arrive from the server and may have been
written anywhere.

**Subtasks are the same rows as anywhere else**, so a subtask reads and behaves
here exactly as it does on the day it is due, and finished ones sit in a block of
their own. A task in the inbox is told why it has none rather than being offered
a field the write path would refuse.

**Links to other tasks are listed under the three things they say** — what this
task waits for, what waits for it, and what is merely worth reading beside it.
One stored kind of edge carries the first two; which one it is depends on the end
being looked at, and that reading is worked out once rather than in each screen
that shows a link. Each row names the task at the other end and goes there when
tapped, because a list of blockers you cannot reach is a list of complaints. A
peer that is finished or abandoned is struck through rather than dropped: the
link is still why the work was waiting.

Adding one searches the device's own index — the same one the search screen runs
on — and a term that is a plain number is also looked up as the id the server
knows a task by, which is the id printed in the web client's address bar. Neither
the task itself nor a task already linked to it is offered, because both would
only be turned down. The two refusals that remain are answered on the device
while the picker is still open, and each is a sentence with something to do in
it rather than "that did not work".

Because the whole graph is on the device, this section is one of the places where
the native client can do something the web client cannot: a blocker ticked off on
a plane releases what it was holding up immediately, padlock and all, instead of
on the next reconnect.

**What is checked.** The presenter is a plain JVM test with no Compose, no
database and no server: every field editor is asserted to name one field, an
editor handed an unchanged value is asserted to queue nothing, and a counter
standing in for the sync engine has to stay at zero across a rename, a priority
change, a completion, a duplicate and a link. The link rules are checked where
they live — the graph rules with no database at all, the refusals against a real
one — and the release is checked from both ends at once: after a blocker is
finished with no network, the padlock a list would draw and the answer the
completion guard gives are asserted to have changed together. The screen itself is composed for real
off-device in each state it has to survive — blocked, finished, described in
markup, described in plain text, undescribed, with subtasks, in the inbox, and
absent from the replica altogether. The arrangement is checked as well, because
it is a property a refactor can quietly undo while every unit test still passes:
a label the workspace has and the task does not is asserted to be off the page
until the sheet is opened, the record of the task to be absent until the block is
tapped, and everything that can be done to the task to be behind the one control
in the bar.

### Projects, boards and contexts (`app/.../projects`)

Browsing the workspace, and every write that reshapes it: projects, the columns
of a project board, and the contexts above both.

**The workspace is browsed under its contexts.** `ProjectsRepository` combines
the standing queries over contexts and projects into one heading per context with
its projects beneath it, and a context with no projects still appears — it is
where the next project is filed. Tapping a heading opens that context, which is
the whole of the context list this client needs; there is no second screen
listing contexts on their own. The projects the user pinned lead the screen,
whichever context they live in.

**Narrowing and searching happen in memory.** A workspace holds tens of projects
and the whole set is already there as a query, so re-running a database query per
keystroke would tear that query down and set it up again for an answer it already
holds. The six narrowings and the reading order — open work first, then the
daily plan in the order its slots are worked through, then alphabetical — are the
web client's own, restated as `ProjectFilter` and `projectsInReadingOrder`.

**A board is a column of columns.** `boardColumns` cuts a project's tasks into
its sections, with the project's own column first: the work that is in the
project but in none of its columns. That column always exists, because it is
where a task lands when it is created without one and where it returns when it is
taken out of one. A task naming a column the board no longer has falls into it
too rather than vanishing — deleting a column keeps the work that was in it, here
as on the server. Finished work is kept apart from the open work inside each
column instead of being filtered away, because a project page is the project's
whole history, and a finished parent takes its subtasks with it so a subtree
never gets torn in half. The board is drawn vertically rather than side by side:
a phone is a tall screen, and a sideways board puts every column but one out of
sight.

**Rearranging is a press, not a drag.** A drag on a touch screen is ambiguous
with a scroll and is unreachable to anyone driving the screen with an assistive
reader, so a column is moved one place at a time from its own menu and a task is
moved between columns by long-pressing it and picking where it goes. Both write
through the shared write path, so a board rearranged with no connection is right
on screen at once and right on the server whenever the phone comes back — and
because moving a column renumbers the whole board exactly as the server will, the
board settles once rather than twice.

**One port per screen, over the shared write path.** `ProjectActions` is
everything the project screens write — the project's fields, its status, its pin,
its place in the daily plan, its columns, and the tasks filed in them — and
`ContextActions` is the three writes a context screen makes. Ticking a row off,
parking it and planning it are *not* restated in either: those screens take the
task lists' own port for them, so "complete this task" has one meaning in the app
rather than several. Refusals the user can act on are told apart and carry the
number that matters — a full shelf carries its size, a full slot what it holds —
and everything else says only that the change did not happen.

**Actions that would be refused are not offered.** Only an open project can take
a place in the daily plan, so a finished one is offered none; a finished project
is offered "reopen" instead of "complete"; an archived one is also offered
"unarchive". The server enforces all three, and offering an action that will be
refused is worse than not offering it.

**What is checked.** The column cut, the narrowings, the reading order and the
three presenters are plain JVM tests — no Compose, no database, no server —
including which position a column move asks for and which placement a task move
asks for, since both are what a user would notice going wrong hours later. The
screens themselves are composed for real off-device and driven in their stateless
form, which is the one the app composes.

### The daily plan and the jump pair (`app/.../troiki`, `app/.../harpoon`)

The two signature habits of the product: three buckets of projects worked
through in order, and two things the user hops between from anywhere.

**The plan is drawn from the device's own data, in one query.** `TroikiRepository`
watches the projects standing in a bucket and asks for the work of all of them at
once — up to nine projects side by side, and nine standing queries over the same
table would each be woken by every write the replica takes. `troikiSlots` cuts
that into the three buckets, always drawn, always in the order they are worked
through. Only an open project holds a place: a finished one keeps the bucket it
was worked on under, because that is worth remembering, but it stops occupying a
place, which is the rule the server counts by. Inside a bucket the projects are
listed the way the server lists them — pinned first, most recently pinned ahead,
newest ahead of older — so the plan reads the same on the phone as in a browser.
Finished work stays under its project instead of vanishing when it is ticked off.

**A place in the plan fixes the priority of the work in it.** The server re-pins
every open task of a project the moment it takes a bucket — important to high,
medium to medium, the rest to low — and pins a task written into or moved into
such a project the same way. The device repeats all three, because a plan
rearranged in a tunnel would otherwise keep showing the priorities the tasks had
before it, on the very screen the change was made on. The other side of the same
rule is that the priority field on a task in such a project is not the user's to
set: the detail screen shows the level, says which project decides it, and
queues nothing.

**What the device knows about capacity, and what it does not.** A bucket starts
with room for three; it *earns* more as work is finished in the bucket above it,
and that counter is kept beside the user's account rather than copied down here.
So the plan draws the room a bucket starts with, the free places it shows are a
floor rather than the truth, and the capacity check the write path makes is never
stricter than the server's and sometimes looser: the plainly hopeless attempt is
refused while the user is still looking at the picker, and anything subtler is
left to the server, which has the number.

**Both controls of the cycle are always offered.** Whether a cycle is running is
part of that same account state, so the screen cannot know whether to offer
"start" or "reset" — and does not need to: beginning a running cycle changes
nothing, and so does ending a stopped one. Ending one is asked about first,
because it takes every project out of the plan; the projects leave the screen
straight away, since that part is the device's to apply, and the counters are
zeroed when the request lands.

**The jump pair is the device's own copy of an account setting.** The pair lives
with the user's preferences on the server and every change to it is sent there,
but it is not part of the data the replica copies down, so the device keeps its
own copy in a small store beside the replica — where a full re-copy cannot empty
it, and where signing out does. Both ends are remembered by the ids *this* device
holds its rows under, so something started with no connection can be hopped to
that afternoon; the request that carries the change translates the reference at
the moment it is sent. `withHarpooned` repeats the server's rule exactly — two
slots, the older one out first, hooking on something already in the pair moves it
rather than duplicating it — which is what keeps both copies at the same two
entries after the same taps. An end whose row has since been deleted drops out of
the pair as it is read, as it does on the server.

**One control, in the top bar, for both halves of it.** The pair is only useful
from wherever you happen to be, so it is not a screen: the control lists the two
things by their current names and jumps to whichever is picked, and it is also
where the task or project on screen is hooked on or taken off. It draws nothing
at all when there is nothing to jump to and nothing on screen to hook on.

**What is checked.** The bucket cut, the plan's presenter and the pair's rule are
plain JVM tests — no Compose, no database, no server — including that a bucket
the device already knows to be full refuses the project rather than queueing a
write that would come back rejected. Both surfaces are then composed for real
off-device and driven in their stateless form, which is the one the app composes.

### The inbox and capture (`app/.../tasks`, `app/.../quickadd`)

Writing something down is what the app is most often opened to do, so it has a
screen to land in and a surface that reaches it from everywhere.

**The inbox is one undivided list.** It is the same list every dated screen uses,
over the query that asks what has been written down without a home, and it is
deliberately not cut into blocks: the inbox is a pile to be emptied, and giving
it structure would suggest work lives there. The rows name no project, because
being in the inbox is the whole of what their placement says, and a subtask
cannot be filed there at all — so the list is flat by construction rather than by
being flattened. When it is empty it says what the inbox is *for*: an empty inbox
is the goal, not a problem to fix.

**Capture is a sheet over whichever screen is on top**, opened from a button that
is always in the same corner. It is the only place in the app that makes a task
from nothing, and the whole of it works with no network: the task is in the
replica and on every list it belongs to the moment the sheet closes, and the queue
reaches the server whenever the phone next can.

The title field takes several lines and writes one task per line — capturing five
things in one sitting is one gesture, and they all take the same project, date,
priority, phase and labels, because they were written down about one thing. If
the write path refuses partway through, the lines that landed stay landed and the
sheet is left holding only the ones that did not, so saving again cannot write
anything twice.

**Two title rules, and the difference between them is the point.** The
installation's rules document carries both, and the device reads it from the
replica. *Auto-labels are applied*: a rule whose mask occurs in the title attaches
its labels, on this device and again on the server. They are shown as chips before
the task is saved, because that is the only moment the user can still say no — and
a refusal travels with the write, so the server does not put back what was just
taken off. *Project suggestions are offered and never applied*: a match puts a
project chip in front of the user and nothing moves until one is tapped. Both
rules name their targets by server id, so a rule pointing at something this device
has never replicated contributes nothing rather than failing the write.

**Where it goes.** The inbox is the default, because capture is for thoughts whose
home has not been decided. The picker leads with the inbox, then the projects this
device has filed work into lately, then everything else; the recent ones are
lifted out of the main list so nothing is offered twice. That order is device-local
and stored on the device alone — which project the phone in your pocket reaches
for is a navigation habit, not a record the workspace owns, and there is no server
field for it. More projects are remembered than the row ever shows, so a picker
narrowed by its search box still has enough history left to fill one.

Due-date shortcuts appear only once the task has a home. A date is a plan and the
inbox holds what has not been planned yet, so scheduling waits — the same rule the
web client applies by hiding those controls there.

**Two ways in from outside the app.** A long press on the launcher icon offers
"add task", which opens the sheet empty. Sharing text from another app opens it
filled in: a subject with text beside it is a page, so the subject titles the task
and the text goes underneath; text on its own is split at its first non-blank
line; and a title too long to draw in a list is cut at the last whole word, with
what was cut kept at the top of the description. Nothing shared is ever dropped,
and the title is always a single line — the sheet reads one line as one task, so a
share must not silently become several. Only text is claimed: the product stores
no attachments, and offering to capture a shared image would quietly discard it.
Both routes park their request until there is a sheet to show it, which is what
lets a share arrive during sign-in or on a cold start without being lost.

**What is checked.** The title rules, the remembered order and the reading of
shared text are plain JVM tests; the mask cases are the web client's own,
restated, because both implementations answer the same question about the same
rules document and would otherwise drift. The sheet itself is composed for real
off-device and driven through its own presenter: a typed line reaches the write
path with the destination the sheet showed, an earned label can be refused before
it is attached, a suggested project stays a suggestion until it is tapped, and the
day shortcuts are absent while the task is headed for the inbox.

### Labels and their usage report (`app/.../labels`)

A label is a marking that cuts across projects and dates, so the screen behind
one is a plain task list — the shared one, with the shared row behaviour — under
a header about the label itself: renaming it, recolouring it, keeping it to hand,
hiding it from the shared read-only view, and taking it away. Finished and
abandoned work stays on that list, because the question a label answers is "what
did I mark this way", not "what is left to do".

**Deleting a label says how many tasks are about to lose it.** The delete is hard
on both sides and cannot be undone, and the count is the one fact that changes
the answer. The work itself is untouched: a tagging is an edge, not the task.

**The usage report is counted on the device.** The server has an endpoint that
answers the same question, and the app deliberately does not call it. The report
is a standing query over the replica like every other screen, so it is complete
in a tunnel — which is the point, because it is a review screen and a review
happens where it happens — and it moves the moment a task is tagged or ticked off
here rather than when a request comes back.

**All three windows are counted in one pass.** The report covers the last 7, 30
and 90 days, each ending at the end of today, plus the equally long window before
each one, which is what the trend beside a row is read from. Counting them
together is what makes switching the window on screen instant, and it is also
what keeps the three honest: they are cut against the same moment. The windows
are rolling rather than calendar-aligned, for the same reason the server's are —
a calendar week one day old would make every label look unused on a Tuesday
morning.

**Two counts are deliberately independent.** An *application* is a tagging made
inside the window; a *completion* is work carrying the label that was finished
inside it. A task tagged months ago and finished this week is this week's
completion and nobody's application. Everything else on a row — how many tasks
carry the label, how many are open, how many are overdue, how many projects it
appears in — describes the label as it stands now and does not move when the
window does.

**Where the boundaries fall.** Windows are cut on calendar days in the zone the
rest of the app measures days in, which is the device's own. The server cuts them
in the zone it is configured with, and that zone is not carried by the sync
contract, so the two agree while the phone is where the server thinks it is and
drift by the offset otherwise. A report is read as "lately", so the effect is
cosmetic — but it is the reason the windows are computed from a zone rather than
from UTC.

**One honest gap in the basis.** The moment a label was applied is what the
windows are counted from, and it is not on the wire: a task's payload carries the
labels themselves, not when each one was attached. Taggings this device made
carry the real moment; taggings that arrived by replication are stamped when the
change was applied here. Re-applying a change never restamps an edge that did not
move, so the drift is invisible in steady state — but on a fresh install every
tagging reads as "applied today" until real tagging washes it out. The totals,
the open and overdue counts, the project spread and the completion counts are
unaffected.

**What is checked.** The counting rules are plain JVM tests over a stopped clock
in a zone that is not UTC — every counting case the server's own aggregate is
checked with that still means something on a device, plus the window edges, which
is where a report goes wrong silently. The query behind them is checked against a real SQLite engine: that a tagging carries
its own moment rather than its task's, that work filed nowhere counts towards no
project, and that deleting a task takes its taggings out of the report. The
screens are checked as screens.

### Search (`app/.../search`)

Search runs entirely against the device. There is no request, no page to fetch
and no offline mode to fall back to — every record the user can see is already
in the replica, which makes search one of the few things that works better
without a connection than with one.

**The index is derived, never maintained by hand.** `core/database` carries four
content-backed full-text indexes — over task titles and descriptions, project
titles and descriptions, label names and context names. They store no copy of the
text and are kept current by triggers created with the schema, so a row written by
a screen, by an applied change or by a cascade nobody called cannot leave its
index entry behind. The one thing triggers cannot cover is a table changed
wholesale rather than row by row, so applying a complete copy rebuilds all four in
the same transaction (`rebuildSearchIndex()`); every ordinary write needs nothing.

**What a person typed is not a query.** `FtsQuery` turns typed text into one:
everything that is not a letter or a digit is a separator, every term becomes a
prefix, and terms are combined with AND. Splitting on non-alphanumerics is what
makes it total — the index's own language uses quotes, stars, colons, minus signs
and parentheses as operators, and a term that cannot contain one needs no
escaping and can never be refused as malformed. Matching itself is the
tokenizer's: it folds case and diacritics across scripts, so `cafe` finds `Café`
and `МОЛОКО` finds `молоко`. Two characters is the floor, the same one the web
client applies.

**Ranking is stated as facts about the row**, because the index keeps no
relevance score: a title hit before a body hit, open work before finished work,
recently touched before old, and the row's own id as the last word so the order is
total. The limit is applied after the ordering, so a capped search drops the worst
matches rather than an arbitrary slice. This is deliberately *not* the server's
ordering — `GET /api/v1/search` ranks a substring scan by the shared list sort
(pinned, then priority) because that is the sort all of its list endpoints use.
The device has the whole workspace in hand and can answer the search question
instead: best match first.

**The filters are the web client's, widened by one.** The web search offers a
single choice — read the task matches or the project ones — and its endpoint takes
no status, project, label or context filter at all. The device keeps that one
choice across the four kinds it indexes, and adds the single narrowing only a
local index makes cheap: leaving finished work out, which is a large share of a
long-lived workspace and almost never what a search is looking for.

**Results stay grouped by kind.** Ranking within a kind is a relevance question
and is answered in the query; ranking a label against a task is not a question
with an answer. Each result leads to the record it names, addressed by the id
this device holds it under, so something written down a moment ago with no
connection opens like anything else.

**Typing is debounced; everything else is not.** A keystroke is part of a query
rather than a request to run one, so it waits 300 ms for the next letter — the
same pause the web client uses. Changing a filter, tapping a past search or
pressing the keyboard's search key *are* the request and run at once, each
cancelling whatever was in flight. The last few searches are kept on the device
alone (`RecentSearches`, most-recent-first, de-duplicated, capped) and are never
synced: a past query is a fact about how this phone was used, not a record the
workspace owns. Only a search the user submitted or re-ran is remembered —
recording every keystroke would fill the list with the prefixes of one query.

**What is checked.** Tokenization and the ranking are pinned against a real SQLite
engine, in both of the product's languages, because both claims are the engine's.
The index staying in step with the replica is checked from the outside: an
optimistic write made with nothing sent anywhere, a page of changes that arrives
from another device, a rename, a delete, and a complete copy served over an index
that had been emptied behind the tables' back. The screen's own decisions — when a
query is worth running, when it is worth waiting, what counts as a search worth
remembering — are plain JVM tests with no database behind them.

### Templates, and turning one task into several (`app/.../templates`)

A template is a task and the checklist that comes with it, kept so the same piece
of work can be written down the same way next time. The screen behind Settings
lists them, the editor writes one, and three of the four things you can do with a
template have no server in them at all.

**Using a template writes the tree here.** Instantiating expands the template into
a real task with its checklist under it, in the project the user picked and the
context above that project, with the template's own wording, urgency and day part
carried onto the root and each line's onto its subtask — the same fields
`internal/service/templates.go` copies. The labels the template names are resolved
back to names and re-applied through the same rule an ordinary create uses, so the
installation's automatic labelling applies to a task made from a template exactly
as it would to one typed by hand. The rows carry no server id until the queue
drains, which is what makes a template usable on a plane: the work is on screen,
reorderable and tickable, before anything has been sent.

**One gesture is one queued write.** The whole expansion is a single
`template.instantiate`, and that op carries the ids of the rows the device drew —
the root first, then the checklist in the template's order. The server creates
them in that same order, so its answer is matched to them position for position
and the ids land on the rows already on screen instead of arriving as a second
copy. A server that made a different number of tasks leaves the odd rows unnamed
rather than misnaming them, and says so in the log; the catch-up that follows
carries the server's own copy either way.

**A template is cut from work that already exists, on the device.** The task
screen's "create template" action reads the task and its whole subtree out of the
replica and turns it into a draft — the task's title becomes the template's name,
and the tree is flattened into the checklist in reading order, because a template
is one level deep. Nothing is fetched: the point of the gesture is that it works
on a task written down a minute ago that the server has never heard of. The draft
is a value, not a record; it exists only until it is saved through the ordinary
template create.

**Editing a template is a replace, so the editor carries what it cannot show.**
The dialog writes the name, the description and the checklist; a template's labels
and its urgency are held through the edit untouched and submitted with it. An
editor that dropped what it does not draw would quietly strip a template last
edited in the browser.

**Splitting a task is done here too.** Decomposing turns one task into several
that inherit everything about it except its wording — where it sits, its urgency,
its dates, its labels, its privacy — and the task they were cut from stops being
one, exactly as `POST /tasks/:id/decompose` leaves the workspace. Blank lines in
the outline are dropped and the rest trimmed, and a task that already has work
under it is refused before anything is queued, for the server's own reason: the
work underneath would have nowhere to go.

The one thing worth knowing about how the split is applied: **the first piece
takes over the row the task was already in** rather than that row being deleted
and a fresh one written beside it. A queued write names the task it acts on by the
row that holds it, so a row deleted before the write went out would leave nothing
to name it by — the split would be on screen and never reach the server. Reusing
the row also leaves whatever was looking at the task looking at something. The
links the task had are dropped with it, because the server drops them too.

**What is checked.** The expansion is checked against a real replica, field by
field, against the rules the Go service applies — placement, carried fields,
labels, the order of the checklist, and one op per gesture. The split is checked
the same way, including both refusals and the trimming. On top of that, a
round-trip case uses a template with the server unreachable, drains the queue and
compares the tree the device drew with the tree the server ends up holding, with a
drainer built fresh over the same replica standing in for a relaunched app.

### The completion history (`app/.../tasks`, `core/sync/maintenance`)

Finished work is the one screen with two sources behind it, because it is the one
list with no natural end. Everything else a screen shows is bounded by the work
that is still open; the history only grows.

**The device keeps a window, the server keeps the rest.** A replica is seeded
with, and told about changes to, the last 90 days of completions —
`REPLICATED_COMPLETED_HISTORY_DAYS` in `core/sync/maintenance`, the same stretch
the server keeps its change log for. The two numbers are deliberately one number:
a device holds exactly the history it can still be told about corrections to, so
nothing it holds can quietly go stale. Anything older stays on the server and is
read on demand.

**The window is maintained, not just observed.** `CompletedHistoryPrune` runs on
the back of a sync cycle — after a turn that reached the server and agreed with
it, never after one that failed, because tidying up on a copy known to be behind
is acting on a guess. It drops completions that have aged out, with two
exceptions that are the whole of its safety: a row with an unsent write against it
is never touched, and a task is only removed when everything beneath it goes too.
Removing a task removes its subtasks with it, so a parent finished a season ago
whose subtask is still open, still unsent, or still inside the window stays where
it is.

**Older pages are held in memory, never written down.** Scrolling past the edge of
the device's copy fetches from `GET /api/v1/tasks/completed`, and those rows live
in the screen and nowhere else. The request names a window far wider than the one
the device holds and lets the server cap it, so the client never has the server's
current limit written into it; that endpoint deliberately serves a window much
wider than a replica is seeded with, because reading what has aged out of a
replica is what it is for (see
[API.md](../API.md#get-apiv1taskscompleted)). Writing them into the replica would
put rows there that the very next tidy-up removes again, so reading back through
a year of history would rewrite the database on every pass. The first request
starts at the offset where the device's own copy ends, which keeps the common
case to one round trip; a page that turns out to hold only rows the device
already has is stepped over, a few times and no more, because a boundary that
has drifted is worth stepping over and a hunt through the whole history is not.

**The edge always says something true.** Below the last line the screen is in
exactly one of six states: more of the device's own copy, older history on the
server, a request in flight, no connection, a refusal, or the end of the history.
The last three are words, not a spinner — with no signal the screen says older
history is kept on the server rather than appearing to load something that is
never going to arrive. What the device holds stays on screen throughout: it came
from the replica and is unaffected by any of this.

**Reopening works on both sources.** A line the device holds is reopened like any
other task. A fetched line has no row here to change and the queue carries device
ids rather than server ones, so the row is taken into the replica from the
server's own answer and reopened in the same transaction; the catch-up that
follows the queued write replaces it with the server's current version. It then
stays, because the window bounds *finished* work and the task is no longer
finished.

**What is checked.** The window's boundary, the two exceptions to the prune and
the ancestor rule are pinned against a real SQLite engine with a fixed clock, and
so is the count a tidy-up answers with, which has to include the subtasks that
left with their parents. The seam that carries housekeeping is checked in its own
right: that it runs after a turn that agreed with the server and after no other
kind, and that it leaves the turn's own answer alone however badly the tidying
itself goes. The behaviour at the edge — showing the device's own lines before
asking the server, where the first request starts, stepping over repeats, and
each of the six things the edge can say — is a plain JVM test with no database
and no server. That the offline edge renders words and no progress indicator is
checked on the composed screen.

### The external calendar (`app/.../calendar`)

The user's Google Calendar entries are shown beside their tasks on the dated
screens, and they are the one thing in this client that is **not** part of the
on-device replica.

**Why it sits outside the replica.** Appointments belong to a system this app
does not own. Nothing here creates, changes or deletes one, so there is no
optimistic write, no queued change and no conflict to resolve — and letting them
into the replica would mean a catch-up or a fresh full copy deciding the fate of
records the server never reports in its change history in the first place. They
are therefore read online and kept in a small key-value store of their own, which
a catch-up cannot reach and a replica wipe cannot empty. Signing out clears it
explicitly, alongside the replica.

**One span, rarely fetched.** `CalendarWindows.coverage` is a single span
covering every screen that shows entries — today, tomorrow and the current week —
plus two days of cushion so a day rolling over is not immediately a reason to go
back to the server. `CalendarRepository` fetches that one span at most once every
two minutes, which matches what the web client does and what the server's own
cache of the provider's answer makes worthwhile; a phone pays for a request in
radio wake-ups rather than in bytes, so one wide request beats three narrow ones.
The integration's status is read first, and when no calendar is connected or none
is chosen the entries are not asked for at all — the server would answer with an
empty list, and hearing that costs a second round trip.

**Gated by the user's own preference.** The `calendarEnabled` flag in the
replicated preference document decides whether anything is requested. Turning it
off does not merely hide the entries: it deletes the stored copy, because a
switch the user turned off must not leave their diary on the device.

**Offline is quiet.** A failed refresh is not an error the user has to dismiss —
their tasks are unaffected and every day still renders. What was last read stays
on screen, and the only thing said about it is one line at the top of the list
giving the moment it was read. A failed attempt counts against the two-minute
interval exactly like a successful one, so a device with no network does not
retry on every screen it opens.

**Where entries land.** The list decides its blocks from the tasks first, and the
calendar is folded in afterwards: an entry may join a block or bring one into
existence, never remove one and never touch the rows or the order the queries
decided. On a day view an entry is filed under the phase its start falls in, by
the same boundaries the day's highlight uses; a whole-day entry goes to "any
time", which is what it is. On the week view entries are filed by day, and a day
holding only appointments gets a block of its own in date order. Whole-day
entries are filed by the **days the provider named**, not by the instant beside
them — the two disagree by up to a day whenever the provider's midnight and the
user's zone differ.

**Entries are not rows.** They are drawn as one quiet band under the heading:
no checkbox, no swipe, nothing to tap. Everything a row offers — ticking,
selecting, opening — is an action this client cannot carry out on someone else's
calendar, and making them look like tasks would promise otherwise.

**Connecting a calendar is done in the web app.** Authorising a Google account
ends with a redirect back to the server, and choosing which calendars are shown
is part of the same surface, so neither is implemented here — the same deferral
the Capacitor shells make. Settings says so in a row that leads nowhere, rather
than leaving the user hunting for a switch that is not there.

**What is checked.** The two reads are exercised over the real HTTP stack,
including the whole-day day-pair. The repository is driven against a scripted
server and an in-memory copy: the preference gate, one fetch serving every dated
screen, the refresh interval, the span outlasting the week, an unreachable server
leaving the stored entries on screen with the moment they were read, and a
recovered server clearing that note. The folding is a plain JVM test per case.
And the isolation is asserted structurally, because it is the one property no
screen can show: nothing in the calendar layer holds a piece of the replica or of
the queue, no record the change history reports is a calendar, and no queued
write exists that could send one.

### Settings (`app/.../settings`)

One screen over **three stores that are never merged**, and keeping them apart is
the whole design:

- the **account's own preferences** — one document on the server that follows the
  user to every device: language, public view, the Today banner and its optional
  day-phase gate, the calendar and daily-plan switches, the two label lists, and
  the pinning caps;
- the **installation's rules** — a second document owned by the server rather
  than by a person, replaced whole through endpoints of its own;
- the **device's own choices** — light or dark, and whether background catch-ups
  may spend mobile data. They are stored in a small key-value file, never sent
  anywhere and never carried to a second device: a tablet on wi-fi and a phone on
  a metered plan want opposite answers to the same question.

**Every server-side edit says only what changed.** A switch writes one key; a
list writes that one list. Sending a full snapshot instead would hand the server
this build's idea of every preference, and a key it has never heard of would come
back absent — that is, destroyed. This matters more here than anywhere else,
because a self-hosted phone and its server are upgraded on different days. The
same reasoning applies one layer down: the stored copy of each document is
*merged into* rather than rebuilt, so an unknown key is a thing this build
ignores rather than a thing it destroys.

**Edits work with no network**, like every other write: the change lands in the
replica and in the queue in one transaction, and the screen redraws from the
replica. What the queue eventually sends is the same one-key change.

**The two rule lists look alike and do opposite things**, which is the one thing
the screen must make impossible to miss. An auto-label rule *acts* — a task whose
title matches comes out already carrying the labels, with nobody asked. A project
suggestion rule only *offers* — the matching projects appear while the user types
and reach the task only if the user picks one. Each list carries the sentence
that says which it is, and so does the editor they share. They also have separate
write paths, because they have separate endpoints: replacing one must never
rewrite the other with whatever this screen last read.

**Ids inside a preference or a rule are the server's.** Those documents travel
back to the server unchanged, so a label or project created on this device and
not yet sent is not offered — it has no server id to name, and it appears the
moment the queue drains.

**A pinning cap the user typed is refused when it is out of range**, and nothing
is written; a *stored* cap out of range falls back to the default instead. The
two are different situations: a blob written before the caps existed is a gap to
fill in, while a number a person just typed is a mistake to point at rather than
silently replace.

**Two actions empty the device, and both ask first with the number at stake.**
Clearing the on-device copy keeps the session and asks for the workspace again —
the repair for a replica that has gone wrong. Changing the server is stronger: it
ends the session, empties the replica and forgets the address, so the app comes
back at the screen that asks which server to talk to. There is no way to carry a
replica across, because an id in it means nothing on another installation.
Whatever the queue was still holding is lost either way, which is why the count
is part of the question rather than a warning after it.

**Turning off syncing on mobile data re-schedules the repeating job.** A job's
constraints are fixed at the moment it is handed to the platform, and the
schedule keeps an existing job rather than replacing it, so the old job is
cancelled and asked for again. The stored answer is also read before the triggers
are armed at launch, or the first job of the process would run under whatever the
flag happened to hold. It governs the background only: a catch-up the user pulled
for is a person standing there waiting.

**About** names the version the package carries — read from the installed package
rather than compiled in, so it cannot drift from the artifact, and carrying the
commit on a build made by a machine that supplied one. The terms and the privacy
policy are opened on the connected server rather than bundled: the product is
self-hosted, so the documents that apply are the ones that installation
publishes, and they are not offered at all before there is a server to open them
on.

**What is checked.** The presenter is a plain JVM test with no Compose, no
database and no server: every edit is asserted to encode as the one key it
changed, a preference write is asserted never to reach the rules' recorder and a
rule write never to reach the preferences', the two rule lists are asserted to
have separate write paths, an out-of-range cap is asserted to write nothing and
leave what was typed on screen, and neither destructive action happens until it
has been confirmed. The same separation is asserted again against a real database
on the write path, where it is a claim about which table a write lands in and
which document keeps the keys this build cannot read. The screen is composed for
real off-device: that the two rule lists say which one acts, that the device's own
choices say they stay on the phone, and that changing servers asks first and says
how much work would be lost.

### The account's own screens (`app/.../account`)

Three administrative surfaces — **the sessions that can reach the account, the
long-lived API tokens, and the time-based second factor** — reached from settings
rather than from the drawer, because none of them is a place work happens.

**They are read live and kept nowhere**, and that is a decision rather than an
omission. Everything else on this client is answered from the on-device copy, so
it works with no connection; these three cannot be, because what they report is
only true at the instant the server says it. A stored session list would go on
showing a device the user revoked from another phone — false reassurance about
exactly the question the screen exists to answer. So none of it enters the
replica, the queue or any preference file, and all three say the same sentence
when the server cannot be reached instead of rendering as an account with nothing
signed in, no tokens and no second factor. An empty list here is only ever the
server's own answer.

**Nothing on them can be queued either.** Every action needs the server to agree
at the moment it is taken: revoking a session offline and sending it a week later
would tell the user a device was locked out while it stayed signed in.

**Three values cross these screens and none is written down.** A token's
plaintext exists in one response — the server keeps only a hash — so it lives in
memory for as long as the dialog showing it, which carries the warning before the
value rather than after it. An enrolment secret is live on the server the moment
it is issued, so a copy of it on the phone holding the first factor would not be
a second factor; it is dropped when the enrolment is proved and equally when the
user walks away, which leaves the account exactly as it was. The recovery codes
are readable once for the same reason. All three are also put on the clipboard
marked sensitive, so the system does not draw a preview of them over whatever the
user switches to next.

**The permission form answers the server's rules early.** A token's scopes are
fixed for its whole life — there is no call that widens or narrows one — so a
selection the server would refuse is not a recoverable mistake but a form to fill
in again. Ticking write therefore ticks read (a token that may change work it
cannot see is refused), unticking read takes write away, a read-only resource is
offered no write box because no such scope exists, and the wildcard stands alone:
it is not "every box ticked", it also covers scopes a later server version adds,
so choosing it clears the boxes and touching a box drops it.

**Signing out everywhere ends this device's session too**, so it goes through the
same sign-out the shell uses and asks the same question first, with the number of
changes the server has not taken in it. The session making the request has no
revoke button on its own row: ending it there would sign the device out sideways,
without that question.

**A deployment without a second factor is an answer, not a failure.** An instance
configured without one never registers those routes, and their refusal is read as
"not offered here" — the screen stops offering rather than inviting another tap.
A wrong code is told apart from it and from a lost connection, because the three
lead to three different things for the user to do.

**What is checked.** The view models are plain JVM tests against a scripted
server: an unfetchable list is asserted to read as missing rather than empty, a
row is asserted to leave the list only once the server has agreed, the plaintext
is asserted to reach the shown-once dialog and never the list, the enrolment
secret is asserted to be dropped both on success and on cancellation, and a
missing route is asserted to settle the whole screen. The permission rules have a
test of their own. A structural check asserts that nothing handling one of these
payloads holds a database, a queue or a preference store, that no replicated
record and no queued write names one, so a later change cannot quietly wire them
together. The screens are composed for real off-device: that all three admit a
missing connection in the same words, that neither secret is ever shown without
the sentence that governs it, and that signing out everywhere says how much
unsent work goes with it.

### On-device round trip (`app/src/androidTest`)

Every other test on this client stands in for something: a fake server, an
in-memory replica, a stand-in for the write path. The on-device suite stands in
for nothing. It builds the real backend binary, starts it on a port nothing else
is using with a database of its own, installs the app on an emulator and drives
the app's *own* object graph — its session manager, its write path, its sync
engine, its replica — against that server, with the emulator's radios genuinely
switched off for the offline half.

**Two addresses to one server.** The app dials the server over the emulator's own
network, so switching the radios off really does cut it off. The suite's own
calls go through a port forwarded over the debug bridge, which does not depend on
the radios at all. That second channel is what makes the interesting cases
possible: the dataset can be seeded, and a row can be deleted *underneath* a
device that is holding an unsent change against it, at a moment when the device
could not possibly hear about it.

**What it proves.**

- A device with nothing on it ends up holding what the server holds: an address
  is accepted, the single account is created (or signed in to, once it exists),
  and the complete copy lands with a recorded position to continue from.
- A batch made with no network reaches the server exactly once and in order —
  a task created offline, a subtask created offline *under it*, an edit, the
  subtask ticked off, and the parent moved to another project. Nothing in that
  chain can work unless the queue goes out in the order it was written and each
  answer is fed back onto the rows the next write names. The check that matters
  most is for a duplicate: the server is asked how many copies it has, and the
  replica is asked whether the id the server assigned landed on the row created
  on the device rather than on a second one beside it.
- A change to a task the server no longer has is set aside exactly once, filed
  as the row being gone, and the writes queued behind it still go out.

The screens are deliberately not driven. What these scenarios are about happens
underneath them, and a walk through the interface would test the layout instead,
slowly and flakily.

The app's graph is reached through an entry point compiled only into the debug
build (`app/src/debug/.../devtools/EngineAccess.kt`), so nothing that ships
contains a way in from outside. Restoring the radios is a rule rather than a line
at the end of a test: a failed assertion would otherwise leave the emulator with
no network and every later test would fail for an unrelated reason.

A run takes under two minutes on a warm build. On failure it leaves the server
log and a logcat capture in `android-native/build/e2e/` and prints the path to
the instrumentation report.

Prerequisites are the same as for `deploy-android`: a JDK, an Android SDK found
via `ANDROID_SDK_ROOT`/`ANDROID_HOME` (or a standard install path), and `adb`.

```sh
just android-native-build    # debug APK
just android-native-run      # build, install and launch on a device/emulator
just android-native-deploy   # install an APK on a connected device, without launching it
just android-native-test     # JVM unit tests
just android-native-lint     # ktlint + Android Lint
just android-native-format   # rewrite sources to the ktlint style
just android-native-e2e      # on-device round trip against a real backend (needs a running emulator)
just android-native-release  # signed release bundle for the store
```

`android-native-test` and `android-native-lint` are part of `just test-all` and
`just lint`, so both aggregates — and `just build-image`, which is gated on them
— now need a JDK and an Android SDK on the machine running them.

`android-native-deploy` is the one to hand a build to someone. It installs and
stops there, and `just android-native-deploy release` installs the **minified**
build — the shape a tester should be running, and the one `android-native-release`
cannot give a phone, because a store bundle is not installable. That variant needs
the signing credentials below: without them Gradle produces an unsigned APK, and
an unsigned APK is refused by the phone at install time rather than by the build.

A debug build and a release build declare the same application id under different
signatures, so one cannot replace the other in place. The recipe says so and
stops, because removing the installed app takes the device's copy of the
workspace, anything queued but not yet sent, and the sign-in with it — that is a
decision for whoever is holding the phone.

### Continuous integration

`.gitlab-ci.yml` runs the client's unit tests and its lints on every pipeline,
in a container image pinned to an exact Android SDK tag so a new SDK release
cannot change what the build produces without a commit. Gradle's home directory
is cached per branch, which is what keeps those jobs to a couple of minutes
instead of a cold dependency download each time.

The on-device suite is **not** attached to every change. It needs an emulator
with nested virtualisation plus a Go toolchain to build the server it runs
against, it takes minutes, and it is the only check here that can fail for
reasons unrelated to the commit. It runs on a schedule and on demand, on a
runner tagged `android-emulator`.

### Releasing

A release build is **minified**: R8 removes what the app does not use and
renames what is left. That is also the only build where a missing shrinker rule
shows up, and it shows up as a crash rather than as a broken build — see
[Surviving the shrinker](#surviving-the-shrinker) below.

Signing credentials are read from the environment and never from the
repository. Put them in `.env` (gitignored) or export them:

| Name | What it is |
|---|---|
| `TURBOIST_ANDROID_KEYSTORE` | path to the upload keystore |
| `TURBOIST_ANDROID_KEYSTORE_PASSWORD` | its password |
| `TURBOIST_ANDROID_KEY_ALIAS` | the key inside it |
| `TURBOIST_ANDROID_KEY_PASSWORD` | that key's password |

A machine that supplies none of the four builds an **unsigned** release
artifact, which is what makes `assembleRelease` runnable on a laptop with no
secrets on it. A machine that supplies *some* of them is stopped with a message
naming the missing ones: it meant to sign and could not, and an artifact that
was supposed to be signed is the one failure that would otherwise be discovered
at the store's upload form.

```sh
just android-native-release
```

It builds `:app:bundleRelease` and leaves two files in
`android-native/build/release/`:

- `turboist-<version>+<commit>.aab` — the bundle. Its version name is the
  repository `VERSION` with any working suffix dropped plus the short commit, so
  a tester's report names the exact code; its version code is derived from the
  bare `major.minor.patch` and never from the commit, so the ordering the store
  enforces cannot be disturbed by a build stamp.
- `turboist-<version>+<commit>-mapping.txt` — the deobfuscation map. A release
  build's names are single letters, so a crash report is unreadable without the
  map that goes with **that** bundle; a later build produces different names.

In CI the same bundle is produced by a manual job on a protected ref, reading
the keystore from a file-type CI variable and the three passwords from masked
ones, and keeps the bundle and the map as artifacts.

Upload is manual for now: Play Console → the app → *Testing* → *Internal
testing* → *Create new release* → upload the `.aab`, then upload the mapping
file under *App bundle explorer* → the version → *Downloads* → *Deobfuscation
file* (Play also carries the map inside the bundle, so this is only needed if
the upload is stripped). Add testers by email or Google group, save, review and
roll out. Nothing about the internal track is reviewed, so a build is available
to testers within minutes.

#### Surviving the shrinker

Three things in this client are found by name at runtime rather than through a
compiled reference, so the shrinker sees nothing pointing at them and is free to
rename or delete them: the wire types and their generated JSON codecs, the HTTP
endpoint interfaces the client builds implementations from, and the generated
database implementation Room looks up by appending a suffix to the class name.
Two more are persistence formats rather than in-process details and therefore
have to keep their names across an app update: the queued writes stored in the
replica, and the navigation destinations written into saved state.

Each module ships the rules for what it declares — `consumer-rules.pro` in the
`core/*` modules, `proguard-rules.pro` in `app` — and the app is minified with
the union of them.

Nobody has to remember to add one. The build logic enumerates those surfaces
from the sources on every run and holds them against the rules, so adding a wire
type, an endpoint interface or a database without a rule for it fails
`just android-native-test` with the class names and the module each belongs to.
A rule that only keeps *members* does not count: it says what happens to them if
the class survives, which is not a reason for the class to survive.

Which modules it reads comes from the build's own module list rather than a list
kept beside the check, so a module added to the build is audited from its first
commit instead of being silently skipped by the one check meant to keep a
release-only crash out. It reads Kotlin, and a production source in another
language fails rather than being passed over.

### Publishing checklist

Before the app leaves the internal track for any wider audience:

- **Application id.** The native client is published as
  `ru.tinyops.turboist.native`, deliberately distinct from the WebView shell's
  `ru.tinyops.turboist` so both install side by side. An id is permanent once a
  build is uploaded under it — the store has no rename — so if the native client
  is ever meant to *replace* the shell rather than sit beside it, that has to
  happen before the first upload on any public track, and it means uploading the
  native build under the shell's id with a higher version code and signing it
  with the shell's key.
- **Privacy policy.** The listing needs a public URL. Every instance serves the
  policy at `<base URL>/privacy-policy`; point the listing at a public one.
- **Data safety.** What the app holds and where it sends it: a full replica of
  the user's workspace in a private on-device database, the server address, and
  a refresh token sealed by a key that never leaves the platform keystore.
  Everything it transmits goes to the one server the user pointed it at and to
  no third party; there is no analytics or advertising SDK in the build. Data is
  encrypted in transit, and the user can delete it by deleting their account on
  their own server or by uninstalling the app, which takes the replica with it.
