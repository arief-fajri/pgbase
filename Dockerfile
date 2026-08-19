FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o pgbase ./examples/base

FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata postgresql17-client
COPY --from=builder /app/pgbase /usr/local/bin/
EXPOSE 8090
ENTRYPOINT ["pgbase"]
CMD ["serve", "--http", "0.0.0.0:8090"]
