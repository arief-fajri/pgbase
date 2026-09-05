# base images are pinned by digest (CFG-06); bump deliberately by re-pinning
FROM golang:1.25-alpine@sha256:78c9e40f10516d7070156948a1747347cc5093022f06deeff3c9decd5faa3af7 AS builder
WORKDIR /app
COPY go.mod go.sum ./
COPY third_party ./third_party
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o pgbase ./examples/base

FROM alpine:3.19@sha256:b58899f069c47216f6002a6850143dc6fae0d35eb8b0df9300bbe6327b9c2171
RUN apk --no-cache add ca-certificates tzdata postgresql16-client
COPY --from=builder /app/pgbase /usr/local/bin/
# run as an unprivileged user to limit the blast radius of any RCE/traversal
RUN adduser -D -h /pb_data -u 10001 pgbase && \
    mkdir -p /pb_data /pb_migrations && \
    chown -R pgbase:pgbase /pb_data /pb_migrations
USER pgbase
EXPOSE 8090
ENTRYPOINT ["pgbase"]
# --encryptionEnv points at the env var holding the 32-char settings encryption
# key; empty (unset) keeps current behavior (plaintext at rest + startup warning).
CMD ["serve", "--http", "0.0.0.0:8090", "--encryptionEnv=PB_ENCRYPTION_KEY"]
