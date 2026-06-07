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

frontend-test name="":
    cd frontend && yarn vitest run {{ if name != "" { name } else { "" } }}

frontend-test-watch:
    cd frontend && yarn vitest

test-all: test && test-frontend

# bench-federation runs the NFR-1 federation performance benchmarks (F7.6):
# inbox apply p95 < 50ms @100k, snapshot 10k < 30s, bootstrap 1k < 60s, push < 5s
# with commit-ping, and the buffer-first no-write-stall availability check. These
# are ADVISORY and gated behind FEDERATION_BENCH=1 so they never run under
# `just test` / CI (whose absolute wall-clock thresholds would be flaky on shared
# runners) — run them on demand here. They use explicit p95 percentile sampling,
# not the framework's ns/op mean; -v surfaces the measured latencies.
bench-federation:
    FEDERATION_BENCH=1 go test -run 'TestF76' -v -count=1 -timeout 600s ./internal/federation/fedtest/

# --- Coverage ---
coverage:
    go test ./... -coverprofile=coverage.out
    go tool cover -func=coverage.out
    go tool cover -html=coverage.out -o coverage.html
    @echo "Coverage report generated at coverage.html"

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

deploy-dev:
    ssh kaiman "cd /opt/turboist-dev && sed -i 's|{{ imageName }}:[^\"]*|{{ imageName }}:{{ version }}|' docker-compose.yml && docker compose pull && docker compose down && docker compose up -d"
