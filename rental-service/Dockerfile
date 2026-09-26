FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/rental-service ./cmd/api

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/rental-service /bin/rental-service
COPY --from=builder /app/migrations /migrations
ENV MIGRATIONS_PATH=/migrations
EXPOSE 8083
ENTRYPOINT ["/bin/rental-service"]
