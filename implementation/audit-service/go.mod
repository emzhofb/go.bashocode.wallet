module github.com/emzhofb/gowallet/audit-service

go 1.25.0

replace github.com/emzhofb/gowallet/pkg => ../pkg

replace github.com/emzhofb/gowallet/proto => ../proto

require (
	github.com/emzhofb/gowallet/pkg v0.0.0-00010101000000-000000000000
	go.mongodb.org/mongo-driver/v2 v2.6.1
)

require (
	github.com/klauspost/compress v1.17.6 // indirect
	github.com/rabbitmq/amqp091-go v1.11.0 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.2.0 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/youmark/pkcs8 v0.0.0-20240726163527-a2c0da244d78 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	go.uber.org/zap v1.28.0 // indirect
	golang.org/x/crypto v0.33.0 // indirect
	golang.org/x/sync v0.11.0 // indirect
	golang.org/x/text v0.22.0 // indirect
)
