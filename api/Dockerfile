## Build stage
FROM golang:1.26 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

COPY bin ./bin
COPY src ./src
COPY conf ./conf

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o copro-api ./bin/app

## Runtime stage
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/copro-api /app/copro-api
COPY --from=builder /app/conf /app/conf

EXPOSE 8080

ENTRYPOINT ["/app/copro-api"]

HEALTHCHECK --interval=60s --timeout=10s --retries=20 \
    CMD curl --fail http://0.0.0.0:8080/ping || exit 1
