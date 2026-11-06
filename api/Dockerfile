# syntax=docker/dockerfile:1

# One image holds the server and the admin commands. Compose picks which to run.
# The built-in discovery agent has its own image, so the server image has no scanner
# and the agent image has no server. Keep the server stage last: it is the default.
ARG GO_VERSION=1.26.10

FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/ \
    ./cmd/server ./cmd/migrate ./cmd/seed ./cmd/retention ./cmd/rotate-secrets \
 && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /agent/agent ./cmd/agent

FROM alpine:3.22 AS agent
RUN apk add --no-cache ca-certificates nmap \
 && adduser -D -H -u 10001 dfipam-agent \
 && install -d -o dfipam-agent -g dfipam-agent -m 700 /var/lib/df-ipam-agent
COPY --from=build /agent/agent /app/agent
USER dfipam-agent
CMD ["/app/agent"]

FROM alpine:3.22
RUN apk add --no-cache ca-certificates \
 && adduser -D -H -u 10001 dfipam
COPY --from=build /out/ /app/
USER dfipam
EXPOSE 8080
CMD ["/app/server"]
