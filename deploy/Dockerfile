# syntax=docker/dockerfile:1
# Multi-stage build, non-root, target image < 60 MB (PRD §6 footprint).
FROM golang:1.27-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION:-dev}" -o /out/raven ./cmd/raven

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=builder /out/raven /app/raven
COPY deploy/Caddyfile /app/Caddyfile
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/raven"]
CMD ["serve"]
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD ["/app/raven", "doctor"] || exit 1
