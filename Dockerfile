# connectd only — deploy to Render (agent stays on Windows host).
FROM golang:1.22-alpine AS build
RUN apk add --no-cache ca-certificates git
WORKDIR /src
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
CMD ["-no-tls", "-no-turn"]
