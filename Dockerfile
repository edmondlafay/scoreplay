FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o scoreplay ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o seed ./cmd/seed

FROM alpine:3.20
# su-exec: minimal setuid helper to drop from root to scoreplay after fixing
# volume permissions at container start (see docker-entrypoint.sh).
RUN apk add --no-cache ca-certificates su-exec

# Run as non-root user
RUN addgroup -S scoreplay && adduser -S scoreplay -G scoreplay

WORKDIR /app
COPY --from=builder /app/scoreplay .
COPY --from=builder /app/seed .
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

RUN mkdir -p uploads && chmod +x /usr/local/bin/docker-entrypoint.sh

EXPOSE 8080
ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["./scoreplay"]
