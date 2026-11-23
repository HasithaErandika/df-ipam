# syntax=docker/dockerfile:1

# One image holds the server and the admin commands. Compose picks which to run.
# Base images are pinned by digest, so a build can be repeated exactly;
# Dependabot proposes new digests weekly.
FROM golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/ \
    ./cmd/server ./cmd/migrate ./cmd/seed

FROM alpine:3.24@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf
RUN apk add --no-cache ca-certificates \
 && adduser -D -H -u 10001 dfipam \
 && mkdir -p /data/attachments && chown dfipam:dfipam /data/attachments && chmod 700 /data/attachments
COPY --from=build /out/ /app/
USER dfipam
EXPOSE 8080
CMD ["/app/server"]
