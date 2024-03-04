# Build stage
FROM golang:1.21.6 as builder
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -ldflags '-s -w' \
    -o go-service-a main.go

# Run stage
FROM alpine as production
COPY --from=builder /app/go-service-a .
CMD ["./go-service-a"]
