# user-service/Dockerfile
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/user-service ./cmd/api

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/user-service /bin/user-service
COPY --from=builder /app/migrations /migrations
EXPOSE 8081
ENTRYPOINT ["/bin/user-service"]
