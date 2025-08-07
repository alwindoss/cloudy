FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -o ./bin/cloudy ./cmd/cloudy/...

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /app/bin/cloudy .

EXPOSE 8080
CMD ["./cloudy"]
