ARG GO_VERSION=1.25
ARG ALPINE_VERSION=3.22

FROM golang:${GO_VERSION}-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
COPY third_party ./third_party
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o pgbase ./examples/base

FROM alpine:${ALPINE_VERSION}
RUN apk --no-cache add ca-certificates tzdata postgresql17-client \
    && addgroup -S pgbase \
    && adduser -S -G pgbase -h /app -s /sbin/nologin pgbase
WORKDIR /app
COPY --from=builder /app/pgbase /usr/local/bin/
USER pgbase:pgbase
EXPOSE 8090
ENTRYPOINT ["pgbase"]
CMD ["serve", "--http", "0.0.0.0:8090"]
