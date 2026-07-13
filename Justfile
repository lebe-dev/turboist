# Load variables from .env (gitignored) into recipe environments — e.g. SONAR_TOKEN.
set dotenv-load

# --- Variables ---

version := `cat VERSION`
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

# --- Tests ---
test name="":
    go test -run "{{ name }}" ./...

test-frontend name="":
    cd frontend && yarn vitest run {{ if name != "" { name } else { "" } }}

frontend-test-watch:
    cd frontend && yarn vitest

test-all: test && test-frontend

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
deploy-ios: cap-sync
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
# (Homebrew android-commandlinetools, ~/Library/Android/sdk). Override the target
# with ANDROID_DEVICE_ID=<serial> (from `adb devices`) when several are attached.
androidDeviceId := env_var_or_default("ANDROID_DEVICE_ID", "")
deploy-android: cap-sync
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
        if [ "$count" -gt 1 ]; then
            echo "error: multiple devices attached — set ANDROID_DEVICE_ID to one of:" >&2
            printf '%s\n' "$devs" | sed 's/^/  /' >&2
            exit 1
        fi
        DEVICE=$(printf '%s\n' "$devs" | head -1)
    fi
    echo "==> deploying to device $DEVICE (sdk $SDK)"
    cd frontend/android
    ./gradlew assembleDebug
    APK="app/build/outputs/apk/debug/app-debug.apk"
    echo "==> installing $APK"
    "$ADB" -s "$DEVICE" install -r "$APK"
    echo "==> launching"
    "$ADB" -s "$DEVICE" shell am start -n ru.tinyops.turboist/.MainActivity

# --- Development Environment ---
start-env: stop-env
    docker compose up -d

stop-env:
    docker compose down

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
