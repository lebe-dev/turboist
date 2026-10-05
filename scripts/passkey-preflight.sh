#!/usr/bin/env bash
#
# Checks everything a server has to get right before a phone can use a passkey
# against it, and that the ceremony recordings the native client's tests are
# pinned to still describe what the server emits today.
#
# Why this exists as a command rather than as a paragraph: the ordinary
# development instance is plain HTTP on localhost, which brings no relying party
# up at all, and its /.well-known/ answers with the web app's index.html — a
# 200, of the wrong content type, containing no association at all. Reading a
# status code there proves nothing. This starts an instance configured the way a
# deployment has to be, asks it the same questions the app asks, and compares
# the answers with the recordings.
#
# It does NOT prove a passkey works on a phone. The platform fetches the
# association file itself, over HTTPS, from the relying party's real domain, and
# nothing local can stand in for that. What it proves is everything on either
# side of the platform prompt.
#
# Usage: scripts/passkey-preflight.sh [port]
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PORT="${1:-18099}"
OUT="$ROOT/android-native/build/passkey-preflight"
FIXTURES="$ROOT/android-native/app/src/testDebug/resources/passkey"

# The host the credentials would be bound to. Any name works here — nothing
# resolves it — but it has to match the recordings, which were taken against it.
RP_HOST="todo.example.com"
# A signing certificate's digest, in the form the app's origin takes. The value
# is a placeholder: what is checked is that an extra origin of this shape is
# accepted alongside the web one, not which certificate it names.
APK_KEY_HASH="9oRgHtEt1cDBDxCiJRnPGGDNVOn2xr4iOTOHIVBWpjE"

USERNAME="preflight"
PASSWORD="preflight-Passw0rd!"

rm -rf "$OUT"
mkdir -p "$OUT"

SERVER_PID=""
cleanup() {
    if [ -n "$SERVER_PID" ]; then kill "$SERVER_PID" 2>/dev/null || true; wait "$SERVER_PID" 2>/dev/null || true; fi
}
trap cleanup EXIT

echo "==> building the server"
go build -o "$OUT/turboist" ./cmd/turboist

echo "==> starting it as a relying party on port $PORT, with a database of its own"
# Started from its own directory so it cannot pick up the repository's .env and
# talk to a real database; the config file is named absolutely.
(
    cd "$OUT"
    BIND="127.0.0.1:$PORT" \
    BASE_URL="https://$RP_HOST" \
    WEBAUTHN_ORIGINS="https://$RP_HOST,android:apk-key-hash:$APK_KEY_HASH" \
    WELL_KNOWN_PATH="$ROOT/deploy/well-known" \
    JWT_SECRET="passkey-preflight-jwt-secret-000000" \
    API_TOKEN_SALT="passkey-preflight-api-token-salt-00" \
    DATA_PATH="$OUT/turboist.db" \
    LOG_LEVEL="warn" \
    "$OUT/turboist" -config "$ROOT/config.yml" >"$OUT/server.log" 2>&1 &
    echo $! >"$OUT/server.pid"
)
SERVER_PID=$(cat "$OUT/server.pid")

for _ in $(seq 1 60); do
    if curl -fsS "http://127.0.0.1:$PORT/api/config" >/dev/null 2>&1; then break; fi
    kill -0 "$SERVER_PID" 2>/dev/null || { echo "error: the server exited on start-up:" >&2; cat "$OUT/server.log" >&2; exit 1; }
    sleep 0.5
done
curl -fsS "http://127.0.0.1:$PORT/api/config" -o "$OUT/config.json" ||
    { echo "error: the server never became reachable" >&2; cat "$OUT/server.log" >&2; exit 1; }

echo "==> reading what it says about passkeys"
curl -fsS -D "$OUT/assetlinks.headers" -o "$OUT/assetlinks.json" \
    "http://127.0.0.1:$PORT/.well-known/assetlinks.json"

echo "==> creating an account and asking for both ceremonies"
curl -fsS -X POST "http://127.0.0.1:$PORT/auth/setup" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"$USERNAME\",\"password\":\"$PASSWORD\",\"clientKind\":\"cli\"}" \
    -o "$OUT/session.json"
TOKEN=$(python3 -c "import json,sys;print(json.load(open(sys.argv[1]))['access'])" "$OUT/session.json")

curl -fsS -X POST "http://127.0.0.1:$PORT/auth/passkey/login/begin" \
    -H 'Content-Type: application/json' -d '{"clientKind":"android"}' \
    -o "$OUT/login-ceremony.json"
curl -fsS -X POST "http://127.0.0.1:$PORT/api/v1/passkeys/register/begin" \
    -H "Authorization: Bearer $TOKEN" \
    -o "$OUT/enrolment-ceremony.json"

echo "==> checking the answers"
python3 - "$OUT" "$FIXTURES" "$RP_HOST" "$USERNAME" <<'PY'
import json
import sys

out, fixtures, rp_host, username = sys.argv[1:5]
failures = []


def check(ok, message):
    if not ok:
        failures.append(message)


def load(path):
    with open(path) as handle:
        return json.load(handle)


# 1. The instance says it has passkeys to offer. The app hides the sign-in
#    button unless it does, so a server that answers false here can never be
#    signed in to with one however well the rest is configured.
config = load(f"{out}/config.json")
check(config.get("passkeys", {}).get("enabled") is True,
      "the instance does not report passkeys as enabled")

# 2. The association file is served, as JSON, and names both apps. The platform
#    refuses the file outright on the wrong content type, and an app the file
#    does not name is an app the domain does not vouch for.
headers = open(f"{out}/assetlinks.headers").read().lower()
check("content-type: application/json" in headers,
      "/.well-known/assetlinks.json is not served as application/json")
assetlinks = load(f"{out}/assetlinks.json")
packages = {t.get("target", {}).get("package_name") for t in assetlinks}
for name in ("ru.tinyops.turboist", "ru.tinyops.turboist.native"):
    check(name in packages, f"the association file does not name {name}")
for target in assetlinks:
    check(bool(target.get("target", {}).get("sha256_cert_fingerprints")),
          f"{target.get('target', {}).get('package_name')} has no signing fingerprint")

# 3. The enrolment ceremony still asks for a discoverable credential bound to
#    this relying party, and still carries a user the platform can put on the
#    sheet. Without the first the later login could not be usernameless.
enrolment = load(f"{out}/enrolment-ceremony.json")["options"]["publicKey"]
check(enrolment["rp"]["id"] == rp_host, "the enrolment options name another relying party")
check(enrolment["user"]["name"] == username, "the enrolment options carry no account name")
check(enrolment["authenticatorSelection"]["residentKey"] == "required",
      "the enrolment options no longer ask for a discoverable credential")
check(bool(enrolment.get("challenge")), "the enrolment options carry no challenge")

# 4. The login ceremony names no account and no credential: the device reports
#    which account it holds, which is the whole point of a resident credential.
login = load(f"{out}/login-ceremony.json")["options"]["publicKey"]
check(login["rpId"] == rp_host, "the login options name another relying party")
check(bool(login.get("challenge")), "the login options carry no challenge")
check("user" not in login, "the login options name an account")
check("allowCredentials" not in login, "the login options name a credential to use")


# 5. The recordings the client's tests are pinned to still have the shape of
#    what came back. Values differ every run — the challenge, the user handle,
#    the ceremony id — so what is compared is which members exist and where.
def shape(value, path=""):
    if isinstance(value, dict):
        members = set()
        for key, member in value.items():
            members.add(f"{path}.{key}")
            members |= shape(member, f"{path}.{key}")
        return members
    if isinstance(value, list):
        members = set()
        for item in value:
            members |= shape(item, f"{path}[]")
        return members
    return set()


for name in ("enrolment-ceremony.json", "login-ceremony.json"):
    live = shape(load(f"{out}/{name}"))
    pinned = shape(load(f"{fixtures}/{name}"))
    for missing in sorted(pinned - live):
        failures.append(f"{name}: the server no longer sends {missing}, which the recording has")
    for added in sorted(live - pinned):
        failures.append(f"{name}: the server now sends {added}, which the recording does not have")

if failures:
    print("passkey preflight FAILED:")
    for failure in failures:
        print(f"  - {failure}")
    raise SystemExit(1)

print("passkey preflight OK:")
print("  - the instance reports passkeys as enabled")
print("  - the association file is served as JSON and names both apps, each with a fingerprint")
print("  - enrolment asks for a discoverable credential, login names no account")
print("  - both recorded ceremonies still match the shape the server sends")
PY

echo "==> done. The platform prompt itself still needs a phone and an HTTPS host."
