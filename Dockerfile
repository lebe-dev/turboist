# -- Build stage --
FROM golang:1.25-alpine AS build

RUN apk add --no-cache nodejs yarn upx

WORKDIR /src
COPY . .

# Extract version from main.go and patch frontend/package.json
RUN VERSION=$(grep 'const Version' cmd/turboist/main.go | head -1 | cut -d'"' -f2) && \
    sed -i "s/\"version\": \".*\"/\"version\": \"${VERSION}\"/" frontend/package.json

RUN cd frontend && yarn --frozen-lockfile && yarn build

RUN CGO_ENABLED=0 go build -ldflags="-w -s" -o /turboist ./cmd/turboist && \
    upx -9 /turboist

# -- Binary export stage (for `docker build --target binary -o out .`) --
FROM scratch AS binary
COPY --from=build /turboist /turboist

# -- Runtime stage --
FROM alpine:3.22

RUN addgroup -g 10001 -S app && \
    adduser -u 10001 -S app -G app

COPY --from=build /turboist /usr/local/bin/turboist

USER 10001:10001
WORKDIR /app

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:8080/api/health || exit 1

ENTRYPOINT ["turboist"]
