# connectd — signaling + dashboard (agent stays on Windows host).
# VPS: docker compose in deploy/ (Caddy TLS + external coturn).
# LAN dev: run connectd.exe directly with embedded TURN.
# Keep this Go image major.minor >= the `go` line in go.mod.
FROM golang:1.25-alpine AS build
RUN apk add --no-cache ca-certificates git
WORKDIR /src
ENV GOPROXY=https://proxy.golang.org,direct
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/connectd ./cmd/connectd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /connectd ./cmd/connectd

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /connectd /connectd
ENV PORT=8787
EXPOSE 8787
ENTRYPOINT ["/connectd"]
CMD ["-no-tls", "-no-turn", "-key", "/data/server.key", "-db", "/data/connect.db"]
