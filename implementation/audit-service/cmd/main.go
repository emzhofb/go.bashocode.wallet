package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/emzhofb/gowallet/pkg/config"
	"github.com/emzhofb/gowallet/pkg/logger"
	"github.com/emzhofb/gowallet/pkg/rabbitmq"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func main() {
	cfg := config.LoadConfig()
	logZap := logger.NewLogger(cfg.AppEnv, "audit-service")
	logZap.Info("Starting Audit Service...")

	// 1. Connect to MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(cfg.MongoURI))
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer client.Disconnect(ctx)

	db := client.Database(cfg.MongoDBName)
	collection := db.Collection("events")

	// 2. Connect to RabbitMQ
	rmq, err := rabbitmq.NewRabbitMQ(cfg.RabbitMQURL)
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ for audit-service: %v", err)
	}
	defer rmq.Close()

	// 3. Consume and save events
	err = rmq.Consume(
		"audit.queue",      // Queue name
		"gowallet.events",  // Exchange name
		"#",                // Routing key (all events)
		func(body []byte) error {
			logZap.Info("Audit Service Received event: " + string(body))

			// Parse JSON into map
			var eventMap map[string]interface{}
			err := json.Unmarshal(body, &eventMap)
			if err != nil {
				logZap.Error("Failed to unmarshal audit event JSON: " + err.Error())
				return err
			}

			// Add timestamp
			eventMap["received_at"] = time.Now()

			// Convert map to BSON
			doc, err := bson.Marshal(eventMap)
			if err != nil {
				logZap.Error("Failed to marshal map to BSON: " + err.Error())
				return err
			}

			// Insert into MongoDB
			dbCtx, dbCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer dbCancel()

			_, err = collection.InsertOne(dbCtx, doc)
			if err != nil {
				logZap.Error("Failed to insert event into MongoDB: " + err.Error())
				return err
			}

			logZap.Info("Audit event successfully stored in MongoDB")
			return nil
		},
	)
	if err != nil {
		log.Fatalf("Failed to register audit consumer: %v", err)
	}

	logZap.Info("Audit Service consumer registered. Listening for events...")

	// Wait for termination signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logZap.Info("Stopping Audit Service...")
}
