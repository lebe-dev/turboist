# --- Variables ---

version := `cat cmd/turboist/main.go | grep Version | head -1 | cut -d " " -f 4 | tr -d "\""`

# --- Utility ---
cleanup:
    rm -f turboist

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

# --- Release ---
release-linux-static: test && lint
    just cleanup
    rm -rf out
    docker build --progress=plain --platform linux/amd64 --target binary -o out .
    mv out/turboist .
    rm -rf out
    tar -czf turboist-v{{ version }}-linux-amd64.tar.gz turboist README.md

release-macos: test && lint
    just cleanup
    go build -ldflags="-w -s" -o turboist ./cmd/turboist
    zip -9 turboist-v{{ version }}-macos-arm.zip turboist README.md
