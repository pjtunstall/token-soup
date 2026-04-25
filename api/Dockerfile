# syntax=docker/dockerfile:1.7

FROM golang:1.26-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
        -o /out/token-soup ./cmd/token-soup

FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /out/token-soup /token-soup

USER 65532:65532

EXPOSE 8080

ENTRYPOINT ["/token-soup"]