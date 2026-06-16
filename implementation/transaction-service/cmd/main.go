package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/emzhofb/gowallet/pkg/config"
	"github.com/emzhofb/gowallet/pkg/database"
	"github.com/emzhofb/gowallet/pkg/logger"
	"github.com/emzhofb/gowallet/pkg/rabbitmq"
	pbLedger "github.com/emzhofb/gowallet/proto/ledger"
	pbUser "github.com/emzhofb/gowallet/proto/user"
	pbWallet "github.com/emzhofb/gowallet/proto/wallet"
	"github.com/emzhofb/gowallet/transaction-service/internal/transaction/handler"
	"github.com/emzhofb/gowallet/transaction-service/internal/transaction/repository"
	"github.com/emzhofb/gowallet/transaction-service/internal/transaction/service"
	"github.com/emzhofb/gowallet/transaction-service/internal/transaction/worker"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	cfg := config.LoadConfig()
	logZap := logger.NewLogger(cfg.AppEnv, "transaction-service")
	logZap.Info("Starting Transaction Service...")

	// 1. MySQL Connection
	db, err := database.NewMySQLConnection(cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, "gowallet_transactions")
	if err != nil {
		log.Fatalf("Failed to connect to MySQL for transaction-service: %v", err)
	}
	defer db.Close()

	// 2. Connect to RabbitMQ
	rmq, err := rabbitmq.NewRabbitMQ(cfg.RabbitMQURL)
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ for transaction-service: %v", err)
	}
	defer rmq.Close()

	// Declare events exchange
	err = rmq.DeclareExchange("gowallet.events", "topic")
	if err != nil {
		log.Fatalf("Failed to declare exchange: %v", err)
	}

	// 3. Setup gRPC clients
	userGRPCAddr := os.Getenv("USER_GRPC_ADDR")
	if userGRPCAddr == "" {
		userGRPCAddr = "localhost:50052"
	}
	walletGRPCAddr := os.Getenv("WALLET_GRPC_ADDR")
	if walletGRPCAddr == "" {
		walletGRPCAddr = "localhost:50053"
	}
	ledgerGRPCAddr := os.Getenv("LEDGER_GRPC_ADDR")
	if ledgerGRPCAddr == "" {
		ledgerGRPCAddr = "localhost:50054"
	}

	userConn, err := grpc.Dial(userGRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to User gRPC server: %v", err)
	}
	defer userConn.Close()
	userClient := pbUser.NewUserServiceClient(userConn)

	walletConn, err := grpc.Dial(walletGRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to Wallet gRPC server: %v", err)
	}
	defer walletConn.Close()
	walletClient := pbWallet.NewWalletServiceClient(walletConn)

	ledgerConn, err := grpc.Dial(ledgerGRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to Ledger gRPC server: %v", err)
	}
	defer ledgerConn.Close()
	ledgerClient := pbLedger.NewLedgerServiceClient(ledgerConn)

	// 4. Setup Components
	txRepo := repository.NewMySQLTransactionRepository(db)
	txSvc := service.NewTransactionService(txRepo, userClient, walletClient, ledgerClient)
	txHandler := handler.NewTransactionHandler(txSvc)

	// 5. Start Outbox background publisher worker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	outboxWorker := worker.NewOutboxWorker(txRepo, rmq)
	go outboxWorker.Start(ctx)

	// 6. Start HTTP Server
	r := gin.New()
	r.Use(gin.Recovery())

	r.POST("/api/v1/transactions/transfer", txHandler.Transfer)

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	logZap.Info("Transaction HTTP Server listening on port 8086...")
	if err := r.Run(":8086"); err != nil {
		log.Fatalf("HTTP Server failed to run: %v", err)
	}
}
