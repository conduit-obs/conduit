FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /conduit ./cmd/conduit

FROM alpine:3.20
RUN apk add --no-cache ca-certificates openssl
COPY --from=builder /conduit /usr/local/bin/conduit
COPY scripts/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 8080 4320
ENTRYPOINT ["/entrypoint.sh"]
