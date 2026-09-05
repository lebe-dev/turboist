# Load variables from .env (gitignored) into recipe environments — e.g. SONAR_TOKEN.
set dotenv-load

# --- Variables ---

version := `cat VERSION`
gitShortHash := `git rev-parse --short HEAD`
mobileVersion := version + "+" + gitShortHash
imageName := 'tinyops/turboist'

# --- Demo environment (init-env / reset-env) ---
# Override via env vars on the command line if you run a different setup.
turboistUrl := env_var_or_default("TURBOIST_URL", "http://127.0.0.1:18080")
seedUser := env_var_or_default("TURBOIST_SEED_USER", "eugene")
seedPass := env_var_or_default("TURBOIST_SEED_PASS", "test")
dbPath := env_var_or_default("DATA_PATH", "data/turboist.db")

# --- Dependencies ---
bump-backend-deps:
    go get -u ./...
    go mod tidy

bump-frontend-deps:
    cd frontend && yarn upgrade

bump-deps: bump-backend-deps && bump-frontend-deps

# --- Build ---
build-frontend:
    cd frontend && yarn && yarn build

build: build-frontend && format
    go build -ldflags="-X main.Version={{ version }}" -o turboist ./cmd/turboist

# --- Lints ---
lint-backend: format
    golangci-lint run ./...

lint-frontend:
    cd frontend && yarn run check && yarn run lint

frontend-lint:
    cd frontend && yarn run check && yarn run lint

lint: format
    just lint-backend
    just lint-frontend
    just android-native-lint

# --- Tests ---
test name="":
    go test -run "{{ name }}" ./...

test-frontend name="":
    cd frontend && yarn vitest run {{ if name != "" { name } else { "" } }}

frontend-test-watch:
    cd frontend && yarn vitest

test-all: test && test-frontend android-native-test

# --- Coverage ---
coverage:
    go test ./... -coverprofile=coverage.out
    go tool cover -func=coverage.out
    go tool cover -html=coverage.out -o coverage.html
    @echo "Coverage report generated at coverage.html"

coverage-frontend:
    cd frontend && yarn vitest run --coverage
    @echo "Frontend coverage report generated at frontend/coverage/lcov.info"

coverage-all: coverage && coverage-frontend

# --- Format ---
format:
    go fmt ./...

# --- Development ---
run-backend:
    go run ./cmd/turboist

run-frontend:
    cd frontend && yarn dev -- --port=4200

frontend-dev:
    cd frontend && yarn dev -- --port=4200

dev:
    cd frontend && yarn dev &
    go run ./cmd/turboist

# Bootstrap the local .env by merging the committed template (.env.example) with
# your personal overrides (.env.dev). Keys in .env.dev win; keys only
# in .env.dev are appended. Refuses to clobber an existing .env — edit it by hand
# or delete it and re-run.
init-dev:
    #!/usr/bin/env bash
    set -euo pipefail
    if [ -f .env ]; then
        echo ".env already exists — not overwriting. Edit it directly, or delete it and re-run 'just init-dev'." >&2
        exit 1
    fi
    [ -f .env.example ] || { echo "error: .env.example not found" >&2; exit 1; }
    cp .env.example .env
    if [ -f .env.dev ]; then
        while IFS= read -r line || [ -n "$line" ]; do
            case "$line" in
                ''|'#'*) continue ;;                      # skip blanks/comments
                *=*) ;;                                    # a KEY=VALUE line
                *) continue ;;
            esac
            key=${line%%=*}
            if grep -qE "^[[:space:]]*${key}=" .env; then
                # override the template's value in place
                tmp=$(mktemp)
                grep -vE "^[[:space:]]*${key}=" .env > "$tmp"
                mv "$tmp" .env
            fi
            printf '%s\n' "$line" >> .env
        done < .env.dev
        echo "==> .env created from .env.example + .env.dev"
    else
        echo "==> .env created from .env.example (no .env.dev found)"
    fi

# --- Mobile (Capacitor) ---
# Rebuild the SPA bundle and copy it into the native iOS + Android projects.
# Run after every web change before building a native app.
cap-sync:
    cd frontend && yarn build && yarn cap sync

# Same as cap-sync, but stamps frontend/package.json's version with the current
# VERSION + short git commit hash (e.g. "1.14.0-dev+a1b2c3d") before building, so
# a locally sideloaded Debug build shows exactly what's installed in
# Settings -> Version on the device. Mirrors the Docker build's version
# substitution (see Dockerfile) but adds the commit hash, since local debug
# builds share a VERSION between commits. package.json is restored afterwards
# so the stamp never leaks into a git diff.
#
# The restore copies the file back rather than running `git checkout --`: the
# checkout would also discard UNCOMMITTED edits to package.json, which silently
# uninstalled a freshly added Capacitor plugin (it then vanished from `cap sync`
# and the native build shipped without it).
cap-sync-versioned:
    #!/usr/bin/env bash
    set -euo pipefail
    cd frontend
    cp package.json package.json.stamp-backup
    trap 'mv -f package.json.stamp-backup package.json' EXIT
    perl -pi -e 's/"version": "[^"]*"/"version": "{{ mobileVersion }}"/' package.json
    yarn build
    yarn cap sync

# Full mobile refresh (web bundle + native sync of both platforms).
mobile: cap-sync

# iOS: rebuild web, sync, open the project in Xcode. Capacitor 8 uses Swift
# Package Manager — no CocoaPods needed. Requires Xcode.
ios-build: cap-sync
    cd frontend && yarn cap open ios

# iOS: rebuild web, sync iOS, run on a simulator/device.
ios-run:
    cd frontend && yarn build && yarn cap sync ios && yarn cap run ios

# iOS: (re)wire the lock-screen widget extension target into App.xcodeproj.
# The widget sources (App/TurboistWidget/) and the target are committed, so this
# is only needed after regenerating the iOS project from scratch (cap add ios).
# Idempotent; requires the xcodeproj gem (`gem install xcodeproj`).
ios-widget:
    ruby frontend/ios/setup-widget.rb

# iOS: build a signed Debug app and deploy it onto a connected iPhone.
# Rebuilds the web bundle, syncs it into the native project, code-signs with your
# Apple Development team, then installs (and tries to launch) via devicectl.
#
# The team is NOT hardcoded in the checked-in Xcode project. It comes from
# IOS_DEV_TEAM_ID, which lives in .env.dev and is merged into .env by
# `just init-dev`; `set dotenv-load` (top of this file) then loads it. Find your team id:
#   security find-identity -v -p codesigning   (the OU field of the Apple Development cert).
# Override the device with IOS_DEVICE_ID=<udid>; otherwise the first connected one is used.
#
# First launch on a personal (free) profile: iOS blocks the app until you trust the
# developer once in Settings -> General -> VPN & Device Management -> Developer App.
iosDeviceId := env_var_or_default("IOS_DEVICE_ID", "")
iosDevTeamId := env_var_or_default("IOS_DEV_TEAM_ID", "")
deploy-ios: cap-sync-versioned
    #!/usr/bin/env bash
    set -euo pipefail
    TEAM="{{ iosDevTeamId }}"
    if [ -z "$TEAM" ]; then
        echo "error: IOS_DEV_TEAM_ID is not set (expected in .env)." >&2
        echo "  Put it in .env.dev and run 'just init-dev' to merge it into .env. Find it via:" >&2
        echo "  security find-identity -v -p codesigning   (the OU field of the Apple Development cert)." >&2
        exit 1
    fi
    DEVICE="{{ iosDeviceId }}"
    if [ -z "$DEVICE" ]; then
        # First physical device in the Devices section — its line carries the iOS
        # version in parens (the Mac's does not); grab the trailing UDID group.
        DEVICE=$(xcrun xctrace list devices 2>&1 \
            | sed -n '/== Devices ==/,/== Simulators ==/p' \
            | grep -E '\([0-9]+\.[0-9]+' \
            | head -1 \
            | sed -E 's/.*\(([^)]+)\)[[:space:]]*$/\1/')
    fi
    if [ -z "$DEVICE" ]; then
        echo "error: no connected iPhone found (and IOS_DEVICE_ID unset)." >&2
        echo "  Plug in the device, trust this Mac, then retry." >&2
        exit 1
    fi
    echo "==> deploying to device $DEVICE (team $TEAM)"
    cd frontend/ios/App
    xcodebuild -project App.xcodeproj -scheme App -configuration Debug \
        -destination "id=$DEVICE" \
        -derivedDataPath ../DerivedData \
        -allowProvisioningUpdates \
        DEVELOPMENT_TEAM="$TEAM" \
        build
    APP="../DerivedData/Build/Products/Debug-iphoneos/App.app"
    echo "==> installing $APP"
    xcrun devicectl device install app --device "$DEVICE" "$APP"
    echo "==> launching"
    xcrun devicectl device process launch --device "$DEVICE" ru.tinyops.turboist || {
        echo "note: launch was denied — on a personal profile trust the developer once in" >&2
        echo "      Settings -> General -> VPN & Device Management, then tap the app icon." >&2
    }

# iOS: package an UNSIGNED Release .ipa for sideloading with AltStore/AltServer.
# Rebuilds the web bundle, syncs it into the native project, builds the App scheme
# for a generic iOS device with code signing switched off, then wraps the product
# into the Payload/ layout an .ipa expects. The result lands in dist/.
#
# No IOS_DEV_TEAM_ID and no connected device are needed: AltStore re-signs the
# bundle (app + the TurboistWidget extension) with your own Apple ID when you
# install it, so any signature baked in here would just be thrown away. Transfer
# dist/Turboist-<version>.ipa to the iPhone (AirDrop / Files / iCloud) and open it
# with AltStore -> "+" -> pick the file. On a free Apple ID the app expires after
# 7 days; keep AltServer reachable on this Mac so AltStore can refresh it.
build-ipa: cap-sync-versioned
    #!/usr/bin/env bash
    set -euo pipefail
    OUT="{{ justfile_directory() }}/dist"
    IPA="$OUT/Turboist-{{ mobileVersion }}.ipa"
    mkdir -p "$OUT"
    cd frontend/ios/App
    echo "==> building unsigned Release (generic/platform=iOS)"
    xcodebuild -project App.xcodeproj -scheme App -configuration Release \
        -destination 'generic/platform=iOS' \
        -derivedDataPath ../DerivedData \
        CODE_SIGNING_ALLOWED=NO \
        CODE_SIGNING_REQUIRED=NO \
        CODE_SIGN_IDENTITY="" \
        build
    APP="../DerivedData/Build/Products/Release-iphoneos/App.app"
    [ -d "$APP" ] || { echo "error: $APP not found after build" >&2; exit 1; }
    echo "==> packaging $IPA"
    STAGE=$(mktemp -d)
    trap 'rm -rf "$STAGE"' EXIT
    mkdir -p "$STAGE/Payload"
    cp -R "$APP" "$STAGE/Payload/"
    rm -f "$IPA"
    (cd "$STAGE" && zip -qry "$IPA" Payload)
    echo "==> done: $IPA ($(du -h "$IPA" | cut -f1))"

# Android: rebuild web, sync, open the project in Android Studio. Requires
# ANDROID_HOME + an installed SDK to build.
android-build: cap-sync
    cd frontend && yarn cap open android

# Android: rebuild web, sync Android, run on an emulator/device.
android-run:
    cd frontend && yarn build && yarn cap sync android && yarn cap run android

# Android: build a Debug APK and deploy it onto a connected device.
# Rebuilds the web bundle, syncs it into the native project, assembles a Debug APK
# (auto-signed with Gradle's debug keystore — no dev team/keystore needed), then
# installs and launches it over adb.
#
# The SDK is found via ANDROID_SDK_ROOT/ANDROID_HOME, else common install paths
# (Homebrew android-commandlinetools, ~/Library/Android/sdk). The target device is
# picked with fzf (showing manufacturer/model/Android version, not just the serial);
# override with ANDROID_DEVICE_ID=<serial> (from `adb devices`) to skip the picker.
androidDeviceId := env_var_or_default("ANDROID_DEVICE_ID", "")
deploy-android: cap-sync-versioned
    #!/usr/bin/env bash
    set -euo pipefail
    # Locate the Android SDK.
    SDK="${ANDROID_SDK_ROOT:-${ANDROID_HOME:-}}"
    if [ -z "$SDK" ]; then
        for c in /opt/homebrew/share/android-commandlinetools "$HOME/Library/Android/sdk"; do
            [ -d "$c/platform-tools" ] && { SDK="$c"; break; }
        done
    fi
    if [ -z "$SDK" ] || [ ! -d "$SDK" ]; then
        echo "error: Android SDK not found. Set ANDROID_SDK_ROOT, or install the SDK." >&2
        echo "  brew install --cask android-commandlinetools android-platform-tools" >&2
        exit 1
    fi
    export ANDROID_SDK_ROOT="$SDK" ANDROID_HOME="$SDK"
    ADB="$(command -v adb || echo "$SDK/platform-tools/adb")"
    # Pick the target device.
    DEVICE="{{ androidDeviceId }}"
    if [ -z "$DEVICE" ]; then
        # Portable (bash 3.2, no mapfile): collect authorized serials into a string.
        devs=$("$ADB" devices | awk 'NR>1 && $2=="device" {print $1}')
        count=$(printf '%s\n' "$devs" | grep -c . || true)
        if [ "$count" -eq 0 ]; then
            echo "error: no authorized Android device found." >&2
            echo "  Enable USB debugging and accept the 'Allow USB debugging?' prompt, then: adb devices" >&2
            exit 1
        fi
        if ! command -v fzf >/dev/null 2>&1; then
            echo "error: fzf is required to pick a device (brew install fzf), or set ANDROID_DEVICE_ID to one of:" >&2
            printf '%s\n' "$devs" | sed 's/^/  /' >&2
            exit 1
        fi
        # Build "serial<TAB>manufacturer model (Android version, device|emulator)" rows,
        # then show only the label column in fzf while keeping the serial to extract.
        rows=""
        while IFS= read -r s; do
            [ -z "$s" ] && continue
            model=$("$ADB" -s "$s" shell getprop ro.product.model 2>/dev/null | tr -d '\r')
            manuf=$("$ADB" -s "$s" shell getprop ro.product.manufacturer 2>/dev/null | tr -d '\r')
            ver=$("$ADB" -s "$s" shell getprop ro.build.version.release 2>/dev/null | tr -d '\r')
            kind="device"
            case "$s" in emulator-*) kind="emulator" ;; esac
            rows="${rows}${s}\t${manuf} ${model} (Android ${ver}, ${kind}) [${s}]\n"
        done <<< "$devs"
        sel=$(printf '%b' "$rows" | fzf --with-nth=2 --delimiter='\t' --select-1 \
            --prompt="Android device> " --height=~40% --reverse)
        [ -z "$sel" ] && { echo "error: no device selected" >&2; exit 1; }
        DEVICE=$(printf '%s' "$sel" | cut -f1)
    fi
    echo "==> deploying to device $DEVICE (sdk $SDK)"
    cd frontend/android
    ./gradlew assembleDebug
    APK="app/build/outputs/apk/debug/app-debug.apk"
    echo "==> installing $APK"
    "$ADB" -s "$DEVICE" install -r "$APK"
    echo "==> launching"
    "$ADB" -s "$DEVICE" shell am start -n ru.tinyops.turboist/.MainActivity

# --- Native Android client ---
# `android-native/` is a Gradle build of its own, entirely separate from the
# Capacitor shell in `frontend/android/`. It shares the repo-root VERSION file.
#
# `android-native-test` and `android-native-lint` are part of `test-all` and
# `lint`, so an Android SDK (found via ANDROID_SDK_ROOT/ANDROID_HOME, or a
# standard install path) and a JDK are needed to run either of those aggregates
# — and therefore to run `build-image`, which is gated on them.

# Build the native Android client's debug APK.
android-native-build:
    cd android-native && ./gradlew --console=plain :app:assembleDebug

# The build-logic tests run first: they pin the rules that turn the repo-root
# VERSION into the app's versionName and versionCode.
# Run the native Android client's JVM unit tests.
android-native-test:
    cd android-native && ./gradlew --console=plain -p buildSrc test
    cd android-native && ./gradlew --console=plain test

# Check the native Android client: ktlint (Kotlin style) + Android Lint.
android-native-lint:
    cd android-native && ./gradlew --console=plain ktlintCheck lint

# Rewrite the native Android client's Kotlin sources to the ktlint style.
android-native-format:
    cd android-native && ./gradlew --console=plain ktlintFormat

# The bundle carries the repository VERSION as its version name plus the commit
# it was built from, so a report from a tester names the exact code that produced
# what they are running. It is minified, which is what makes the shrinker rules
# matter: a class the runtime looks up by name and no rule protects is gone from
# this artifact and from no other.
#
# Signing credentials are read from the environment, never from the repository —
# put them in .env (gitignored) or export them:
#
#   TURBOIST_ANDROID_KEYSTORE           path to the upload keystore
#   TURBOIST_ANDROID_KEYSTORE_PASSWORD  its password
#   TURBOIST_ANDROID_KEY_ALIAS          the key inside it
#   TURBOIST_ANDROID_KEY_PASSWORD       that key's password
#
# Gradle would happily produce an unsigned bundle without them and the store
# would reject it at the upload form, so the recipe stops here instead and says
# which value is missing.
#
# Build a signed release bundle of the native Android client for the store.
android-native-release:
    #!/usr/bin/env bash
    set -euo pipefail
    ROOT="{{ justfile_directory() }}"

    missing=""
    for name in TURBOIST_ANDROID_KEYSTORE TURBOIST_ANDROID_KEYSTORE_PASSWORD \
                TURBOIST_ANDROID_KEY_ALIAS TURBOIST_ANDROID_KEY_PASSWORD; do
        eval "value=\${$name:-}"
        [ -n "$value" ] || missing="$missing $name"
    done
    if [ -n "$missing" ]; then
        echo "error: the release signing credentials are not set:$missing" >&2
        echo "  Put them in .env (gitignored) or export them, then re-run." >&2
        exit 1
    fi
    [ -f "$TURBOIST_ANDROID_KEYSTORE" ] || {
        echo "error: keystore not found: $TURBOIST_ANDROID_KEYSTORE" >&2
        exit 1
    }

    # What the artifact is called is what the bundle declares inside it: the
    # release version with any working suffix dropped, plus the commit. Naming
    # the file anything else means the answer to "which build is this" depends on
    # where you read it.
    STAMPED="{{ version }}"
    STAMPED="${STAMPED%%-*}"
    STAMPED="${STAMPED%%+*}+{{ gitShortHash }}"

    echo "==> building the signed bundle ($STAMPED)"
    cd "$ROOT/android-native"
    ./gradlew --console=plain :app:bundleRelease -Pturboist.buildStamp={{ gitShortHash }}

    OUT="$ROOT/android-native/build/release"
    mkdir -p "$OUT"
    cp app/build/outputs/bundle/release/app-release.aab "$OUT/turboist-$STAMPED.aab"
    # The names in a release build are single letters, so a stack trace from the
    # store is unreadable without this file. Keep the one that goes with this
    # exact bundle — a later build produces different names.
    cp app/build/outputs/mapping/release/mapping.txt "$OUT/turboist-$STAMPED-mapping.txt"

    echo "==> done"
    echo "    bundle:  $OUT/turboist-$STAMPED.aab"
    echo "    mapping: $OUT/turboist-$STAMPED-mapping.txt"
    echo "    upload both to the internal testing track (see docs/mobile.md)"

# It starts an instance configured as a relying party, serves the association
# file from deploy/well-known, and compares both ceremonies with the recordings
# the client's tests are pinned to. Pass a port to use one other than 18099.
# Check what a server must get right before a phone can sign in with a passkey.
android-native-passkey-preflight port="18099":
    ./scripts/passkey-preflight.sh {{ port }}

# The SDK is located exactly like deploy-android does; the target device is
# picked with fzf, or set ANDROID_DEVICE_ID=<serial> to skip the picker.
# Build, install and launch the native Android client on a device or emulator.
android-native-run:
    #!/usr/bin/env bash
    set -euo pipefail
    # Locate the Android SDK.
    SDK="${ANDROID_SDK_ROOT:-${ANDROID_HOME:-}}"
    if [ -z "$SDK" ]; then
        for c in /opt/homebrew/share/android-commandlinetools "$HOME/Library/Android/sdk"; do
            [ -d "$c/platform-tools" ] && { SDK="$c"; break; }
        done
    fi
    if [ -z "$SDK" ] || [ ! -d "$SDK" ]; then
        echo "error: Android SDK not found. Set ANDROID_SDK_ROOT, or install the SDK." >&2
        echo "  brew install --cask android-commandlinetools android-platform-tools" >&2
        exit 1
    fi
    export ANDROID_SDK_ROOT="$SDK" ANDROID_HOME="$SDK"
    ADB="$(command -v adb || echo "$SDK/platform-tools/adb")"
    # Pick the target device.
    DEVICE="{{ androidDeviceId }}"
    if [ -z "$DEVICE" ]; then
        devs=$("$ADB" devices | awk 'NR>1 && $2=="device" {print $1}')
        count=$(printf '%s\n' "$devs" | grep -c . || true)
        if [ "$count" -eq 0 ]; then
            echo "error: no authorized Android device found." >&2
            echo "  Enable USB debugging and accept the 'Allow USB debugging?' prompt, then: adb devices" >&2
            exit 1
        fi
        if [ "$count" -eq 1 ]; then
            DEVICE="$devs"
        elif ! command -v fzf >/dev/null 2>&1; then
            echo "error: fzf is required to pick a device (brew install fzf), or set ANDROID_DEVICE_ID to one of:" >&2
            printf '%s\n' "$devs" | sed 's/^/  /' >&2
            exit 1
        else
            rows=""
            while IFS= read -r s; do
                [ -z "$s" ] && continue
                model=$("$ADB" -s "$s" shell getprop ro.product.model 2>/dev/null | tr -d '\r')
                manuf=$("$ADB" -s "$s" shell getprop ro.product.manufacturer 2>/dev/null | tr -d '\r')
                ver=$("$ADB" -s "$s" shell getprop ro.build.version.release 2>/dev/null | tr -d '\r')
                kind="device"
                case "$s" in emulator-*) kind="emulator" ;; esac
                rows="${rows}${s}\t${manuf} ${model} (Android ${ver}, ${kind}) [${s}]\n"
            done <<< "$devs"
            sel=$(printf '%b' "$rows" | fzf --with-nth=2 --delimiter='\t' --select-1 \
                --prompt="Android device> " --height=~40% --reverse)
            [ -z "$sel" ] && { echo "error: no device selected" >&2; exit 1; }
            DEVICE=$(printf '%s' "$sel" | cut -f1)
        fi
    fi
    echo "==> building the native client (sdk $SDK)"
    cd android-native
    ./gradlew --console=plain :app:assembleDebug
    APK="app/build/outputs/apk/debug/app-debug.apk"
    echo "==> installing $APK on $DEVICE"
    "$ADB" -s "$DEVICE" install -r "$APK"
    echo "==> launching"
    "$ADB" -s "$DEVICE" shell am start -n ru.tinyops.turboist.native/ru.tinyops.turboist.nativeapp.MainActivity

# `android-native-run` builds, installs and starts the app in one go, which is
# what you want while writing code. This one only puts a build on a phone: it
# does not launch it, and it can install the **minified release** build, which
# is the shape a tester should be given and the one `android-native-release`
# cannot hand to a device — a store bundle is not installable.
#
# Pass `release` to install that build. It needs the same TURBOIST_ANDROID_KEYSTORE*
# values as `android-native-release`, and the recipe insists on them: without
# them Gradle produces an *unsigned* APK, which the phone refuses at install time
# rather than at build time.
#
# The SDK is located exactly like `android-native-run` does; the target device is
# picked with fzf, or set ANDROID_DEVICE_ID=<serial> to skip the picker.
#
# Install the native Android client's APK on a connected device.
android-native-deploy variant="debug":
    #!/usr/bin/env bash
    set -euo pipefail
    VARIANT="{{ variant }}"
    case "$VARIANT" in
        debug|release) ;;
        *) echo "error: variant must be 'debug' or 'release', not '$VARIANT'" >&2; exit 1 ;;
    esac

    # A release APK that nothing signed cannot be installed, so ask for the
    # credentials here rather than letting the phone deliver the verdict.
    if [ "$VARIANT" = "release" ]; then
        missing=""
        for name in TURBOIST_ANDROID_KEYSTORE TURBOIST_ANDROID_KEYSTORE_PASSWORD \
                    TURBOIST_ANDROID_KEY_ALIAS TURBOIST_ANDROID_KEY_PASSWORD; do
            eval "value=\${$name:-}"
            [ -n "$value" ] || missing="$missing $name"
        done
        if [ -n "$missing" ]; then
            echo "error: a release build needs the signing credentials:$missing" >&2
            echo "  Put them in .env (gitignored) or export them, then re-run." >&2
            echo "  Or install the debug build instead: just android-native-deploy" >&2
            exit 1
        fi
        [ -f "$TURBOIST_ANDROID_KEYSTORE" ] || {
            echo "error: keystore not found: $TURBOIST_ANDROID_KEYSTORE" >&2
            exit 1
        }
    fi

    # Locate the Android SDK.
    SDK="${ANDROID_SDK_ROOT:-${ANDROID_HOME:-}}"
    if [ -z "$SDK" ]; then
        for c in /opt/homebrew/share/android-commandlinetools "$HOME/Library/Android/sdk"; do
            [ -d "$c/platform-tools" ] && { SDK="$c"; break; }
        done
    fi
    if [ -z "$SDK" ] || [ ! -d "$SDK" ]; then
        echo "error: Android SDK not found. Set ANDROID_SDK_ROOT, or install the SDK." >&2
        echo "  brew install --cask android-commandlinetools android-platform-tools" >&2
        exit 1
    fi
    export ANDROID_SDK_ROOT="$SDK" ANDROID_HOME="$SDK"
    ADB="$(command -v adb || echo "$SDK/platform-tools/adb")"

    # A phone that is plugged in but whose "Allow USB debugging?" prompt was never
    # accepted is invisible to every serial listing below. Say so, or installing
    # onto the one other attached device looks like installing onto this one.
    pending=$("$ADB" devices | awk 'NR>1 && $2!="device" && $1!="" {print $1" ("$2")"}')
    if [ -n "$pending" ]; then
        echo "note: these attached devices cannot be used yet:" >&2
        printf '%s\n' "$pending" | sed 's/^/  /' >&2
        echo "  Accept the 'Allow USB debugging?' prompt on the phone to include it." >&2
    fi

    # Pick the target device.
    DEVICE="{{ androidDeviceId }}"
    if [ -z "$DEVICE" ]; then
        devs=$("$ADB" devices | awk 'NR>1 && $2=="device" {print $1}')
        count=$(printf '%s\n' "$devs" | grep -c . || true)
        if [ "$count" -eq 0 ]; then
            echo "error: no authorized Android device found." >&2
            echo "  Enable USB debugging and accept the 'Allow USB debugging?' prompt, then: adb devices" >&2
            exit 1
        fi
        if [ "$count" -eq 1 ]; then
            DEVICE="$devs"
        elif ! command -v fzf >/dev/null 2>&1; then
            echo "error: fzf is required to pick a device (brew install fzf), or set ANDROID_DEVICE_ID to one of:" >&2
            printf '%s\n' "$devs" | sed 's/^/  /' >&2
            exit 1
        else
            rows=""
            while IFS= read -r s; do
                [ -z "$s" ] && continue
                model=$("$ADB" -s "$s" shell getprop ro.product.model 2>/dev/null | tr -d '\r')
                manuf=$("$ADB" -s "$s" shell getprop ro.product.manufacturer 2>/dev/null | tr -d '\r')
                ver=$("$ADB" -s "$s" shell getprop ro.build.version.release 2>/dev/null | tr -d '\r')
                kind="device"
                case "$s" in emulator-*) kind="emulator" ;; esac
                rows="${rows}${s}\t${manuf} ${model} (Android ${ver}, ${kind}) [${s}]\n"
            done <<< "$devs"
            sel=$(printf '%b' "$rows" | fzf --with-nth=2 --delimiter='\t' --select-1 \
                --prompt="Android device> " --height=~40% --reverse)
            [ -z "$sel" ] && { echo "error: no device selected" >&2; exit 1; }
            DEVICE=$(printf '%s' "$sel" | cut -f1)
        fi
    fi

    echo "==> building the $VARIANT APK (sdk $SDK)"
    cd android-native
    if [ "$VARIANT" = "release" ]; then
        # Stamped the same way the store bundle is, so a report from a phone names
        # the exact code that produced what is running on it.
        ./gradlew --console=plain :app:assembleRelease -Pturboist.buildStamp={{ gitShortHash }}
        APK="app/build/outputs/apk/release/app-release.apk"
    else
        ./gradlew --console=plain :app:assembleDebug
        APK="app/build/outputs/apk/debug/app-debug.apk"
    fi
    [ -f "$APK" ] || { echo "error: the build produced no APK at $APK" >&2; exit 1; }

    echo "==> installing $APK on $DEVICE"
    # A debug build and a release build carry the same application id under
    # different signatures, so swapping one for the other is a reinstall the
    # platform refuses. Removing the old one takes the device's copy of the
    # workspace, its queued changes and its sign-in with it, so say that and stop
    # rather than deciding it here.
    if ! out=$("$ADB" -s "$DEVICE" install -r "$APK" 2>&1); then
        printf '%s\n' "$out" >&2
        case "$out" in
            *INSTALL_FAILED_UPDATE_INCOMPATIBLE*|*signatures*do*not*match*|*INSTALL_FAILED_VERSION_DOWNGRADE*)
                echo "" >&2
                echo "The app already on this device was signed with a different key, so it cannot" >&2
                echo "be replaced in place. Uninstalling first WIPES its local copy of the workspace," >&2
                echo "anything queued but not yet sent, and the sign-in:" >&2
                echo "  adb -s $DEVICE uninstall ru.tinyops.turboist.native" >&2
                ;;
        esac
        exit 1
    fi
    printf '%s\n' "$out"

    echo "==> done"
    echo "    installed the $VARIANT build on $DEVICE"
    echo "    start it from the launcher, or: just android-native-run"

e2eUser := env_var_or_default("TURBOIST_E2E_USER", "harness")
e2ePassword := env_var_or_default("TURBOIST_E2E_PASSWORD", "harness-password")
e2eAppHost := env_var_or_default("TURBOIST_E2E_APP_HOST", "")
# Builds the Go binary, starts it on a free port with a database of its own,
# reinstalls the app from scratch and runs the on-device suite against it.
#
# Two addresses reach the same server, and the difference is what makes an
# offline test possible. The app dials it over the emulator's own radios
# (10.0.2.2 is the host as the emulator sees it), so switching those off really
# does cut the app off. The suite's own calls — seeding the dataset, deleting a
# row underneath a device that is offline, asking the server what it ended up
# holding — go through a port forwarded over the debug bridge, which does not
# depend on the radios at all.
#
# Set TURBOIST_E2E_APP_HOST to run against a physical device on the same
# network; the server is then bound on every interface instead of loopback.
#
# Everything started here is stopped again on the way out, including after a
# failure, and a failure leaves the server log and a logcat capture behind.
#
# Drive the native Android client on an emulator against a freshly built server.
android-native-e2e:
    #!/usr/bin/env bash
    set -euo pipefail
    ROOT="{{ justfile_directory() }}"
    OUT="$ROOT/android-native/build/e2e"
    rm -rf "$OUT"
    mkdir -p "$OUT"

    # Locate the Android SDK.
    SDK="${ANDROID_SDK_ROOT:-${ANDROID_HOME:-}}"
    if [ -z "$SDK" ]; then
        for c in /opt/homebrew/share/android-commandlinetools "$HOME/Library/Android/sdk"; do
            [ -d "$c/platform-tools" ] && { SDK="$c"; break; }
        done
    fi
    if [ -z "$SDK" ] || [ ! -d "$SDK" ]; then
        echo "error: Android SDK not found. Set ANDROID_SDK_ROOT, or install the SDK." >&2
        exit 1
    fi
    export ANDROID_SDK_ROOT="$SDK" ANDROID_HOME="$SDK"
    ADB="$(command -v adb || echo "$SDK/platform-tools/adb")"

    # Pick the target device.
    DEVICE="{{ androidDeviceId }}"
    if [ -z "$DEVICE" ]; then
        devs=$("$ADB" devices | awk 'NR>1 && $2=="device" {print $1}')
        count=$(printf '%s\n' "$devs" | grep -c . || true)
        if [ "$count" -eq 0 ]; then
            echo "error: no running emulator or authorized device found. Start an emulator first." >&2
            exit 1
        fi
        if [ "$count" -ne 1 ]; then
            echo "error: more than one device is attached; set ANDROID_DEVICE_ID to one of:" >&2
            printf '%s\n' "$devs" | sed 's/^/  /' >&2
            exit 1
        fi
        DEVICE="$devs"
    fi

    # Which address the app dials. An emulator reaches the host it runs on at a
    # fixed alias, so the server can stay on loopback; anything else has to be
    # named explicitly and needs the server reachable from the network.
    APP_HOST="{{ e2eAppHost }}"
    if [ -z "$APP_HOST" ]; then
        case "$DEVICE" in
            emulator-*) APP_HOST="10.0.2.2" ;;
            *)
                echo "error: $DEVICE is not an emulator, so it cannot reach this machine at the emulator alias." >&2
                echo "  Set TURBOIST_E2E_APP_HOST=<this machine's address on the device's network> and retry." >&2
                exit 1
                ;;
        esac
        BIND_HOST="127.0.0.1"
    else
        BIND_HOST="0.0.0.0"
    fi

    # A port nobody else is on. The harness never assumes it owns the port a
    # development server is usually started on.
    command -v python3 >/dev/null 2>&1 || { echo "error: python3 is needed to pick a free port" >&2; exit 1; }
    PORT=$(python3 -c "import socket; s = socket.socket(); s.bind(('127.0.0.1', 0)); print(s.getsockname()[1]); s.close()")

    SERVER_PID=""
    cleanup() {
        set +e
        if [ -n "$SERVER_PID" ]; then
            kill "$SERVER_PID" >/dev/null 2>&1
            wait "$SERVER_PID" >/dev/null 2>&1
        fi
        "$ADB" -s "$DEVICE" reverse --remove "tcp:$PORT" >/dev/null 2>&1
        # A run stopped part-way through could leave the emulator with no
        # network, which would break every later run for an unrelated reason.
        "$ADB" -s "$DEVICE" shell svc wifi enable >/dev/null 2>&1
        "$ADB" -s "$DEVICE" shell svc data enable >/dev/null 2>&1
        rm -f "$OUT/turboist.db" "$OUT/turboist.db-shm" "$OUT/turboist.db-wal"
    }
    trap cleanup EXIT

    echo "==> building the server"
    go build -o "$OUT/turboist" ./cmd/turboist

    echo "==> starting it on port $PORT with a database of its own"
    # Started from its own directory so it cannot pick up the repository's .env
    # and talk to a real database; the config file is named absolutely.
    (
        cd "$OUT"
        BIND="$BIND_HOST:$PORT" \
        BASE_URL="http://$APP_HOST:$PORT" \
        JWT_SECRET="on-device-harness-jwt-secret-000000" \
        API_TOKEN_SALT="on-device-harness-api-token-salt-0000" \
        DATA_PATH="$OUT/turboist.db" \
        LOG_LEVEL="info" \
        "$OUT/turboist" -config "$ROOT/config.yml" >"$OUT/server.log" 2>&1 &
        echo $! >"$OUT/server.pid"
    )
    SERVER_PID=$(cat "$OUT/server.pid")

    for _ in $(seq 1 60); do
        if curl -fsS "http://127.0.0.1:$PORT/api/config" >/dev/null 2>&1; then break; fi
        kill -0 "$SERVER_PID" 2>/dev/null || { echo "error: the server exited on start-up:" >&2; cat "$OUT/server.log" >&2; exit 1; }
        sleep 0.5
    done
    curl -fsS "http://127.0.0.1:$PORT/api/config" >/dev/null || { echo "error: the server never became reachable" >&2; cat "$OUT/server.log" >&2; exit 1; }

    echo "==> forwarding the checking channel onto $DEVICE"
    "$ADB" -s "$DEVICE" reverse "tcp:$PORT" "tcp:$PORT" >/dev/null

    echo "==> reinstalling the app so the run starts from a fresh install"
    "$ADB" -s "$DEVICE" uninstall ru.tinyops.turboist.native >/dev/null 2>&1 || true
    "$ADB" -s "$DEVICE" uninstall ru.tinyops.turboist.native.test >/dev/null 2>&1 || true
    "$ADB" -s "$DEVICE" logcat -c >/dev/null 2>&1 || true

    echo "==> running the on-device suite"
    cd "$ROOT/android-native"
    set +e
    ANDROID_SERIAL="$DEVICE" ./gradlew --console=plain :app:connectedDebugAndroidTest \
        "-Pandroid.testInstrumentationRunnerArguments.turboistAppBaseUrl=http://$APP_HOST:$PORT" \
        "-Pandroid.testInstrumentationRunnerArguments.turboistControlBaseUrl=http://127.0.0.1:$PORT" \
        "-Pandroid.testInstrumentationRunnerArguments.turboistUsername={{ e2eUser }}" \
        "-Pandroid.testInstrumentationRunnerArguments.turboistPassword={{ e2ePassword }}"
    STATUS=$?
    set -e
    if [ "$STATUS" -ne 0 ]; then
        "$ADB" -s "$DEVICE" logcat -d -v time >"$OUT/logcat.txt" 2>&1 || true
        echo "" >&2
        echo "==> the on-device run failed" >&2
        echo "    server log:  $OUT/server.log" >&2
        echo "    device log:  $OUT/logcat.txt" >&2
        echo "    report:      $ROOT/android-native/app/build/reports/androidTests/connected/debug/index.html" >&2
        exit "$STATUS"
    fi
    echo "==> done"

# --- Development Environment ---
start-env: stop-env
    docker compose up -d

stop-env:
    docker compose down

stop:
    lsof -ti :4200 | xargs kill -9
    lsof -ti :18080 | xargs kill -9

# --- Demo data ---
# init-env populates the SQLite database with demo contexts/projects/labels/tasks
# and switches on the Troiki system with 9 projects (3-3-3 across slots).
# Requires the backend to be running so /auth/setup can create the single user.
init-env:
    #!/usr/bin/env bash
    set -euo pipefail
    URL="{{ turboistUrl }}"
    DB="{{ dbPath }}"
    USER="{{ seedUser }}"
    PASS="{{ seedPass }}"

    echo "==> ensuring user exists at $URL"
    body=$(printf '{"username":"%s","password":"%s","clientKind":"cli"}' "$USER" "$PASS")
    tmp=$(mktemp)
    code=$(curl -sS -o "$tmp" -w '%{http_code}' \
        -X POST "$URL/auth/setup" \
        -H 'Content-Type: application/json' \
        -d "$body" || echo "000")
    case "$code" in
        200) echo "    created user '$USER'" ;;
        410) echo "    user already exists — skipping /auth/setup" ;;
        000) echo "    cannot reach $URL — is the backend running?" >&2; rm -f "$tmp"; exit 1 ;;
        *)   echo "    /auth/setup returned HTTP $code:" >&2; cat "$tmp" >&2; echo >&2; rm -f "$tmp"; exit 1 ;;
    esac
    rm -f "$tmp"

    echo "==> seeding $DB from scripts/seed-env.sql"
    sqlite3 "$DB" < scripts/seed-env.sql

    echo "==> done. Login with $USER / $PASS at $URL"

# reset-env wipes the SQLite database. The backend must be stopped first so it
# releases the WAL/SHM files; restart it afterwards to re-run migrations.
reset-env:
    #!/usr/bin/env bash
    set -euo pipefail
    DB="{{ dbPath }}"
    if [ ! -f "$DB" ]; then
        echo "no db at $DB — nothing to remove"
        exit 0
    fi
    if lsof "$DB" >/dev/null 2>&1; then
        echo "error: $DB is open by another process (likely the backend) — stop it first" >&2
        exit 1
    fi
    echo "==> removing $DB (+ -shm/-wal)"
    rm -f "$DB" "$DB-shm" "$DB-wal"
    echo "==> done. Start the backend to re-run migrations: just run-backend"

# --- SonarQube (static analysis) ---
# Compose stack lives in sonarqube.yml (SonarQube Community Build + PostgreSQL).
# The scanner runs as a one-shot `docker run` and reads sonar-project.properties.
sonarComposeFile := "sonarqube.yml"
# Host URL as seen from *inside* the scanner container. host.docker.internal
# reaches the host's published port 9000 on Docker Desktop and (via --add-host)
# on Linux. Override with SONAR_HOST_URL when scanning a remote instance.
sonarHostUrl := env_var_or_default("SONAR_HOST_URL", "http://host.docker.internal:9000")

# Start SonarQube + PostgreSQL (UI at http://localhost:9000, admin/admin on first login)
sonar-up:
    docker compose -f {{ sonarComposeFile }} up -d
    @echo "==> SonarQube starting at http://localhost:9000 (first boot ~1-2 min). Login admin/admin, then:"
    @echo "==> 1. Set password 'h18D-a9127DaA8'."
    @echo "==> 2. Create a token for 'just sonar-scan'."

# Stop SonarQube (named volumes keep the DB + analysis history)
sonar-down:
    docker compose -f {{ sonarComposeFile }} down

# Wipe SonarQube including all data volumes
sonar-clean:
    docker compose -f {{ sonarComposeFile }} down -v

# Run the scanner against the running instance. Reads SONAR_TOKEN from .env
# (auto-loaded via `set dotenv-load`); add a line like: SONAR_TOKEN=sqp_xxx
# Run `just coverage-all` first to generate coverage.out + frontend/coverage/lcov.info.
sonar-scan-with-coverage: coverage-all sonar-scan

sonar-scan:
    #!/usr/bin/env bash
    set -euo pipefail
    if [ -z "${SONAR_TOKEN:-}" ]; then
        echo "error: SONAR_TOKEN is not set." >&2
        echo "  Generate a token at {{ sonarHostUrl }} -> My Account -> Security," >&2
        echo "  then add it to .env:  SONAR_TOKEN=sqp_xxx" >&2
        exit 1
    fi
    docker run --rm \
        --add-host=host.docker.internal:host-gateway \
        -e SONAR_HOST_URL="{{ sonarHostUrl }}" \
        -e SONAR_TOKEN="$SONAR_TOKEN" \
        -v "$PWD:/usr/src" \
        sonarsource/sonar-scanner-cli:latest \
        -Dsonar.projectVersion="{{ version }}"

# --- Image ---
build-image: test-all && lint
    docker build --progress=plain --platform linux/amd64 -t {{ imageName }}:{{ version }} .

push-image:
    docker push {{ imageName }}:{{ version }}

release-image: build-image && push-image

release: release-image

ssh:
    ssh kaiman

# --- Deploy ---
deploy:
    ssh kaiman "cd /opt/turboist && sed -i 's|{{ imageName }}:[^\"]*|{{ imageName }}:{{ version }}|' docker-compose.yml && docker compose pull && docker compose down && docker compose up -d"
