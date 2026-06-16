package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/emzhofb/gowallet/pkg/config"
	"github.com/emzhofb/gowallet/pkg/logger"
	"github.com/emzhofb/gowallet/pkg/rabbitmq"
)

func main() {
	cfg := config.LoadConfig()
	logZap := logger.NewLogger(cfg.AppEnv, "notification-service")
	logZap.Info("Starting Notification Service...")

	// 1. Connect to RabbitMQ
	rmq, err := rabbitmq.NewRabbitMQ(cfg.RabbitMQURL)
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ for notification-service: %v", err)
	}
	defer rmq.Close()

	// 2. Consume Events
	err = rmq.Consume(
		"notification.queue", // Queue name
		"gowallet.events",    // Exchange name
		"#",                  // Routing key (all events)
		func(body []byte) error {
			logZap.Info("Notification Service Received event: " + string(body))
			// Simulate sending notification (email/push)
			logZap.Info("SIMULATION: Email notification dispatched successfully.")
			return nil
		},
	)
	if err != nil {
		log.Fatalf("Failed to register consumer: %v", err)
	}

	logZap.Info("Notification Service consumer registered. Listening for events...")

	// Wait for termination signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logZap.Info("Stopping Notification Service...")
}
