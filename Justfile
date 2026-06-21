# Load variables from .env (gitignored) into recipe environments — e.g. SONAR_TOKEN.
set dotenv-load := true

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
