package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/emzhofb/gowallet/pkg/config"
	"github.com/emzhofb/gowallet/pkg/database"
	"github.com/emzhofb/gowallet/pkg/logger"
	"github.com/emzhofb/gowallet/pkg/rabbitmq"
	pb "github.com/emzhofb/gowallet/proto/wallet"
	walletgrpc "github.com/emzhofb/gowallet/wallet-service/internal/wallet/grpc"
	wallethandler "github.com/emzhofb/gowallet/wallet-service/internal/wallet/handler"
	walletrepository "github.com/emzhofb/gowallet/wallet-service/internal/wallet/repository"
	walletservice "github.com/emzhofb/gowallet/wallet-service/internal/wallet/service"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
)

func main() {
	cfg := config.LoadConfig()
	logZap := logger.NewLogger(cfg.AppEnv, "wallet-service")
	logZap.Info("Starting Wallet Service...")

	// 1. MySQL Connection
	db, err := database.NewMySQLConnection(cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, "gowallet_wallet")
	if err != nil {
		log.Fatalf("Failed to connect to MySQL for wallet-service: %v", err)
	}
	defer db.Close()

	// 2. Setup Components
	wRepo := walletrepository.NewMySQLWalletRepository(db)
	wSvc := walletservice.NewWalletService(wRepo)
	wHandler := wallethandler.NewWalletHandler(wSvc)

	// 3. Connect to RabbitMQ
	rmq, err := rabbitmq.NewRabbitMQ(cfg.RabbitMQURL)
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ for wallet-service: %v", err)
	}
	defer rmq.Close()

	// Start consumer for payment.completed events
	err = rmq.Consume(
		"wallet.payment.queue",
		"gowallet.events",
		"payment.completed",
		func(body []byte) error {
			logZap.Info("Wallet Service received payment event: " + string(body))
			var evt struct {
				UserID string `json:"user_id"`
				Amount int64  `json:"amount"`
				Type   string `json:"type"`
			}
			if err := json.Unmarshal(body, &evt); err != nil {
				return err
			}

			// Get wallet ID first
			ctx := context.Background()
			w, err := wRepo.GetByUserID(evt.UserID)
			if err != nil {
				return err
			}
			if w == nil {
				return fmt.Errorf("wallet not found for user %s", evt.UserID)
			}

			amount := evt.Amount
			if evt.Type == "withdrawal" {
				amount = -evt.Amount
			}

			_, err = wSvc.UpdateBalanceWithRetry(ctx, w.ID, amount)
			if err != nil {
				return err
			}

			logZap.Info(fmt.Sprintf("Successfully updated wallet %s by %d from event", w.ID, amount))
			return nil
		},
	)
	if err != nil {
		log.Fatalf("Failed to start payment event consumer: %v", err)
	}

	// 4. Start gRPC Server
	go func() {
		lis, err := net.Listen("tcp", ":50053")
		if err != nil {
			log.Fatalf("gRPC Server failed to listen: %v", err)
		}

		s := grpc.NewServer()
		pb.RegisterWalletServiceServer(s, walletgrpc.NewWalletGRPCServer(wSvc))

		logZap.Info("Wallet gRPC Server listening on port 50053...")
		if err := s.Serve(lis); err != nil {
			log.Fatalf("Failed to serve gRPC: %v", err)
		}
	}()

	// 4. Start HTTP Server
	r := gin.New()
	r.Use(gin.Recovery())

	r.POST("/api/v1/wallets/create", wHandler.Create)
	r.GET("/api/v1/wallets/balance", wHandler.GetBalance)

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	logZap.Info("Wallet HTTP Server listening on port 8082...")
	if err := r.Run(":8082"); err != nil {
		log.Fatalf("HTTP Server failed to run: %v", err)
	}
}
