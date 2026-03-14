# --- Variables ---

version := `cat cmd/turboist/main.go | grep Version | head -1 | cut -d " " -f 4 | tr -d "\""`
imageName := 'tinyops/turboist'

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
    go build -o turboist ./cmd/turboist

# --- Lints ---
lint-backend: format
    golangci-lint run ./...

lint-frontend:
    cd frontend && yarn check

lint: format
    just lint-backend
    just lint-frontend

# --- Tests ---
test name="":
    go test -run "{{ name }}" ./...

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

dev:
    cd frontend && yarn dev &
    go run ./cmd/turboist

# --- Development Environment ---
start-env: stop-env
    docker compose up -d

stop-env:
    docker compose down

# --- Image ---
build-image: test && lint
    docker build --progress=plain --platform linux/amd64 -t {{ imageName }}:{{ version }} .

push-image:
    docker push {{ imageName }}:{{ version }}

release-image: build-image && push-image

release: release-image

# --- Deploy ---
deploy:
    ssh kaiman "cd /opt/turboist && sed -i 's|tinyops/turboist:[^\"]*|tinyops/turboist:{{ version }}|' docker-compose.yml && docker compose pull && docker compose up -d"
