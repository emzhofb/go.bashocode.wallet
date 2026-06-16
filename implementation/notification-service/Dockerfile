FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.work ./
COPY pkg/ pkg/
COPY proto/ proto/
COPY notification-service/ notification-service/
RUN cd notification-service && GOWORK=off go build -o /service ./cmd/main.go

FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /service .
CMD ["./service"]
