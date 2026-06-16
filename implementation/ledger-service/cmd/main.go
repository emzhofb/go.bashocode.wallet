package main

import (
	"log"
	"net"

	"github.com/emzhofb/gowallet/ledger-service/internal/ledger/grpc"
	"github.com/emzhofb/gowallet/ledger-service/internal/ledger/repository"
	"github.com/emzhofb/gowallet/ledger-service/internal/ledger/service"
	"github.com/emzhofb/gowallet/pkg/config"
	"github.com/emzhofb/gowallet/pkg/database"
	"github.com/emzhofb/gowallet/pkg/logger"
	pb "github.com/emzhofb/gowallet/proto/ledger"
	googlegrpc "google.golang.org/grpc"
)

func main() {
	cfg := config.LoadConfig()
	logZap := logger.NewLogger(cfg.AppEnv, "ledger-service")
	logZap.Info("Starting Ledger Service...")

	// 1. MySQL Connection
	db, err := database.NewMySQLConnection(cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, "gowallet_ledger")
	if err != nil {
		log.Fatalf("Failed to connect to MySQL for ledger-service: %v", err)
	}
	defer db.Close()

	// 2. Setup Components
	lRepo := repository.NewMySQLLedgerRepository(db)
	lSvc := service.NewLedgerService(lRepo)

	// 3. Start gRPC Server
	lis, err := net.Listen("tcp", ":50054")
	if err != nil {
		log.Fatalf("gRPC Server failed to listen: %v", err)
	}

	s := googlegrpc.NewServer()
	pb.RegisterLedgerServiceServer(s, grpc.NewLedgerGRPCServer(lSvc))

	logZap.Info("Ledger gRPC Server listening on port 50054...")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve gRPC: %v", err)
	}
}
