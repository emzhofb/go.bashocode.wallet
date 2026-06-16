module github.com/emzhofb/gowallet/ledger-service

go 1.25.0

replace github.com/emzhofb/gowallet/pkg => ../pkg

replace github.com/emzhofb/gowallet/proto => ../proto

require (
	github.com/emzhofb/gowallet/pkg v0.0.0-00010101000000-000000000000
	github.com/emzhofb/gowallet/proto v0.0.0-00010101000000-000000000000
	github.com/google/uuid v1.6.0
	google.golang.org/grpc v1.81.1
)

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/go-sql-driver/mysql v1.10.0 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	go.uber.org/zap v1.28.0 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260226221140-a57be14db171 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)
