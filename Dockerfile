# ---- Build stage ----
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Enforce a static amd64 build target to fix the exec format error
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app/server ./cmd/api

# ---- Runtime stage ----
FROM alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=builder /app/server .

# Explicitly grant executable permissions to the binary
RUN chmod +x /app/server

EXPOSE 80

CMD ["./server"]