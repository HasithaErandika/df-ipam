# syntax=docker/dockerfile:1

# One image holds the server and the admin commands. Compose picks which to run.
# The discovery agent isn't here: it runs on hosts inside the networks it covers.
ARG GO_VERSION=1.26.10

FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/ \
    ./cmd/server ./cmd/migrate ./cmd/seed ./cmd/retention ./cmd/rotate-secrets

FROM alpine:3.22
RUN apk add --no-cache ca-certificates \
 && adduser -D -H -u 10001 dfipam
COPY --from=build /out/ /app/
USER dfipam
EXPOSE 8080
CMD ["/app/server"]
