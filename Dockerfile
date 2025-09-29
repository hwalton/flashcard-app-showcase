# Build stage
FROM golang:1.23 as builder

WORKDIR /app

# Copy Go mod files
COPY src/go.mod .
COPY src/go.sum .
RUN go mod download

# Copy all backend code
COPY src/ .

# Build the Go app
RUN CGO_ENABLED=0 go build -o flashcards-server

# Final runtime image
FROM debian:bullseye-slim

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/flashcards-server .
COPY --from=builder /app/frontend /app/frontend

EXPOSE 8080

CMD ["./flashcards-server"]
