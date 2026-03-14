FROM node:22-alpine3.23 AS frontend-build

WORKDIR /build

COPY frontend/ /build
COPY cmd/turboist/main.go /build/main.go

RUN APP_VERSION=$(grep 'Version' /build/main.go | head -1 | cut -d '"' -f 2) && \
    sed -i "s/\"version\": \"[^\"]*\"/\"version\": \"$APP_VERSION\"/" /build/package.json && \
    yarn --frozen-lockfile && \
    yarn build

FROM golang:1.25-alpine3.23 AS app-build

WORKDIR /build

RUN apk --no-cache add upx

COPY go.mod go.sum ./
RUN go mod download

COPY . /build
COPY --from=frontend-build /build/build/ /build/frontend/build/

RUN CGO_ENABLED=0 go build -ldflags="-w -s" -o turboist ./cmd/turboist && \
    upx -9 --lzma turboist && \
    chmod +x turboist

# Binary export stage (for `docker build --target binary -o out .`)
FROM scratch AS binary
COPY --from=app-build /build/turboist /turboist

FROM alpine:3.23.3

WORKDIR /app

RUN apk --no-cache add tzdata && \
    addgroup -g 10001 turboist && \
    adduser -h /app -D -u 10001 -G turboist turboist && \
    chmod 700 /app && \
    chown -R turboist: /app

COPY --from=app-build /build/turboist /app/turboist
# COPY --from=app-build /build/config.yml /app/config.yml

RUN chown -R turboist: /app && chmod +x /app/turboist

USER turboist

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -q -O- http://localhost:8080/api/health || exit 1

CMD ["/app/turboist"]
