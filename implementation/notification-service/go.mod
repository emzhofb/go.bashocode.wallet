module github.com/emzhofb/gowallet/notification-service

go 1.25.0

replace github.com/emzhofb/gowallet/pkg => ../pkg

replace github.com/emzhofb/gowallet/proto => ../proto

require github.com/emzhofb/gowallet/pkg v0.0.0-00010101000000-000000000000

require (
	github.com/rabbitmq/amqp091-go v1.11.0 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	go.uber.org/zap v1.28.0 // indirect
)
