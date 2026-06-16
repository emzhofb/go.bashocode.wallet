package main

import (
	"log"
	"net/http"

	"github.com/emzhofb/gowallet/pkg/config"
	"github.com/emzhofb/gowallet/pkg/logger"
	"github.com/emzhofb/gowallet/pkg/rabbitmq"
	"github.com/emzhofb/gowallet/payment-service/internal/payment/handler"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.LoadConfig()
	logZap := logger.NewLogger(cfg.AppEnv, "payment-service")
	logZap.Info("Starting Payment Service...")

	// 1. Connect to RabbitMQ
	rmq, err := rabbitmq.NewRabbitMQ(cfg.RabbitMQURL)
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ for payment-service: %v", err)
	}
	defer rmq.Close()

	// 2. Setup Components
	pHandler := handler.NewPaymentHandler(rmq, cfg)

	// 3. Start HTTP Server
	r := gin.New()
	r.Use(gin.Recovery())

	r.POST("/api/v1/payments/webhook", pHandler.Webhook)

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	logZap.Info("Payment HTTP Server listening on port 8083...")
	if err := r.Run(":8083"); err != nil {
		log.Fatalf("HTTP Server failed to run: %v", err)
	}
}
