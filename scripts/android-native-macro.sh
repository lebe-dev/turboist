#!/usr/bin/env bash
#
# Runs the native Android client's baseline profile generator or its benchmarks
# against a throwaway local server, with a workspace the harness creates itself.
#
#   scripts/android-native-macro.sh profile    regenerate the committed profile
#   scripts/android-native-macro.sh benchmark  measure startup and scrolling
#
# ANDROID_DEVICE_ID picks the device when more than one is attached.
#
# The app it installs carries a suffixed application id, so a phone's real
# installation, its replica, its unsent writes and its sign-in are not touched.
# Benchmark numbers mean something on a physical device only: on an emulator the
# run is allowed to finish so the harness can be checked, nothing more.
set -euo pipefail

MODE="${1:-}"
case "$MODE" in
    profile | benchmark) ;;
    *)
        echo "usage: $0 profile|benchmark" >&2
        exit 2
        ;;
esac

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$ROOT/android-native/build/macro"
TARGET_APP="ru.tinyops.turboist.native.benchmark"
rm -rf "$OUT"
mkdir -p "$OUT"

SDK="${ANDROID_SDK_ROOT:-${ANDROID_HOME:-}}"
if [ -z "$SDK" ]; then
    for candidate in /opt/homebrew/share/android-commandlinetools "$HOME/Library/Android/sdk"; do
        [ -d "$candidate/platform-tools" ] && {
            SDK="$candidate"
            break
        }
    done
fi
if [ -z "$SDK" ] || [ ! -d "$SDK" ]; then
    echo "error: Android SDK not found. Set ANDROID_SDK_ROOT, or install the SDK." >&2
    exit 1
fi
export ANDROID_SDK_ROOT="$SDK" ANDROID_HOME="$SDK"
ADB="$(command -v adb || echo "$SDK/platform-tools/adb")"

DEVICE="${ANDROID_DEVICE_ID:-}"
if [ -z "$DEVICE" ]; then
    devices=$("$ADB" devices | awk 'NR>1 && $2=="device" {print $1}')
    count=$(printf '%s\n' "$devices" | grep -c . || true)
    if [ "$count" -eq 0 ]; then
        echo "error: no running emulator or authorized device found." >&2
        exit 1
    fi
    if [ "$count" -ne 1 ]; then
        echo "error: more than one device is attached; set ANDROID_DEVICE_ID to one of:" >&2
        printf '%s\n' "$devices" | sed 's/^/  /' >&2
        exit 1
    fi
    DEVICE="$devices"
fi

# Recording a profile without root needs Android 13 or later.
API=$("$ADB" -s "$DEVICE" shell getprop ro.build.version.sdk | tr -d '\r')
if [ "$API" -lt 33 ]; then
    echo "error: $DEVICE runs API $API; the profile harness needs API 33 or later." >&2
    exit 1
fi

PORT=$(python3 -c "import socket; s = socket.socket(); s.bind(('127.0.0.1', 0)); print(s.getsockname()[1]); s.close()")
# Named rather than numbered: localhost is the one host the benchmark library's own
# network security config lets the harness reach over plain HTTP.
SERVER_URL="http://localhost:$PORT"

SERVER_PID=""
cleanup() {
    set +e
    if [ -n "$SERVER_PID" ]; then
        kill "$SERVER_PID" >/dev/null 2>&1
        wait "$SERVER_PID" >/dev/null 2>&1
    fi
    "$ADB" -s "$DEVICE" reverse --remove "tcp:$PORT" >/dev/null 2>&1
    "$ADB" -s "$DEVICE" uninstall "$TARGET_APP" >/dev/null 2>&1
    rm -f "$OUT/turboist.db" "$OUT/turboist.db-shm" "$OUT/turboist.db-wal"
}
trap cleanup EXIT

echo "==> building the server"
(cd "$ROOT" && go build -o "$OUT/turboist" ./cmd/turboist)

echo "==> starting it on port $PORT with a database of its own"
# Started from its own directory so it cannot pick up the repository's .env and
# talk to a real database; the config file is named absolutely.
(
    cd "$OUT"
    BIND="127.0.0.1:$PORT" \
        BASE_URL="$SERVER_URL" \
        JWT_SECRET="macrobenchmark-harness-jwt-secret-000000" \
        API_TOKEN_SALT="macrobenchmark-harness-api-token-salt-00" \
        DATA_PATH="$OUT/turboist.db" \
        LOG_LEVEL="warn" \
        "$OUT/turboist" -config "$ROOT/config.yml" >"$OUT/server.log" 2>&1 &
    echo $! >"$OUT/server.pid"
)
SERVER_PID=$(cat "$OUT/server.pid")

for _ in $(seq 1 60); do
    if curl -fsS "$SERVER_URL/api/config" >/dev/null 2>&1; then break; fi
    kill -0 "$SERVER_PID" 2>/dev/null || {
        echo "error: the server exited on start-up:" >&2
        cat "$OUT/server.log" >&2
        exit 1
    }
    sleep 0.5
done
curl -fsS "$SERVER_URL/api/config" >/dev/null || {
    echo "error: the server never became reachable" >&2
    cat "$OUT/server.log" >&2
    exit 1
}

echo "==> forwarding port $PORT onto $DEVICE"
"$ADB" -s "$DEVICE" reverse "tcp:$PORT" "tcp:$PORT" >/dev/null

# A run stopped part-way leaves an app signed in to a server that no longer exists.
"$ADB" -s "$DEVICE" uninstall "$TARGET_APP" >/dev/null 2>&1 || true

cd "$ROOT/android-native"
ARGUMENT_PREFIX="-Pandroid.testInstrumentationRunnerArguments"
export ANDROID_SERIAL="$DEVICE"

set +e
if [ "$MODE" = "profile" ]; then
    echo "==> recording the baseline profile on $DEVICE"
    ./gradlew --console=plain :app:generateReleaseBaselineProfile \
        "$ARGUMENT_PREFIX.turboistServer=$SERVER_URL"
    STATUS=$?
else
    echo "==> measuring on $DEVICE"
    SUPPRESS=()
    case "$DEVICE" in
        emulator-*)
            echo "    an emulator: these numbers check the harness and say nothing about the app"
            SUPPRESS=("$ARGUMENT_PREFIX.androidx.benchmark.suppressErrors=EMULATOR")
            ;;
    esac
    ./gradlew --console=plain :baselineprofile:connectedBenchmarkReleaseAndroidTest \
        "$ARGUMENT_PREFIX.turboistServer=$SERVER_URL" \
        "$ARGUMENT_PREFIX.androidx.benchmark.enabledRules=macrobenchmark" \
        ${SUPPRESS[@]+"${SUPPRESS[@]}"}
    STATUS=$?
fi
set -e

if [ "$STATUS" -ne 0 ]; then
    "$ADB" -s "$DEVICE" logcat -d -v time >"$OUT/logcat.txt" 2>&1 || true
    echo "" >&2
    echo "==> the run failed" >&2
    echo "    server log: $OUT/server.log" >&2
    echo "    device log: $OUT/logcat.txt" >&2
    echo "    reports:    $ROOT/android-native/baselineprofile/build/reports/androidTests/" >&2
    exit "$STATUS"
fi

if [ "$MODE" = "profile" ]; then
    echo "==> done: commit android-native/app/src/release/generated/baselineProfiles/"
else
    echo "==> done: results in android-native/baselineprofile/build/outputs/connected_android_test_additional_output/"
fi
