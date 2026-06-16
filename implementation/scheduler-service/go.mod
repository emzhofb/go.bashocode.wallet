module github.com/emzhofb/gowallet/scheduler-service

go 1.25.0

replace github.com/emzhofb/gowallet/pkg => ../pkg

replace github.com/emzhofb/gowallet/proto => ../proto

require (
	github.com/emzhofb/gowallet/pkg v0.0.0-00010101000000-000000000000
	github.com/robfig/cron/v3 v3.0.1
)

require (
	go.uber.org/multierr v1.10.0 // indirect
	go.uber.org/zap v1.28.0 // indirect
)
