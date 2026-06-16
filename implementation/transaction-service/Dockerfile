FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.work ./
COPY pkg/ pkg/
COPY proto/ proto/
COPY transaction-service/ transaction-service/
RUN cd transaction-service && GOWORK=off go build -o /service ./cmd/main.go

FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /service .
EXPOSE 8086
EXPOSE 50056
CMD ["./service"]
