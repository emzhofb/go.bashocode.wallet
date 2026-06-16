package main

import (
	"log"
	"net"
	"net/http"

	"github.com/emzhofb/gowallet/pkg/config"
	"github.com/emzhofb/gowallet/pkg/database"
	"github.com/emzhofb/gowallet/pkg/logger"
	pb "github.com/emzhofb/gowallet/proto/user"
	usergrpc "github.com/emzhofb/gowallet/user-service/internal/user/grpc"
	userhandler "github.com/emzhofb/gowallet/user-service/internal/user/handler"
	userrepository "github.com/emzhofb/gowallet/user-service/internal/user/repository"
	userservice "github.com/emzhofb/gowallet/user-service/internal/user/service"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
)

func main() {
	cfg := config.LoadConfig()
	logZap := logger.NewLogger(cfg.AppEnv, "user-service")
	logZap.Info("Starting User Service...")

	// 1. MySQL Connection
	db, err := database.NewMySQLConnection(cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, "gowallet_user")
	if err != nil {
		log.Fatalf("Failed to connect to MySQL for user-service: %v", err)
	}
	defer db.Close()

	// 2. Setup Layer Components
	uRepo := userrepository.NewMySQLUserRepository(db)
	uSvc := userservice.NewUserService(uRepo)
	uHandler := userhandler.NewUserHandler(uSvc)

	// 3. Start gRPC Server in a goroutine
	go func() {
		lis, err := net.Listen("tcp", ":50052")
		if err != nil {
			log.Fatalf("gRPC Server failed to listen: %v", err)
		}

		s := grpc.NewServer()
		pb.RegisterUserServiceServer(s, usergrpc.NewUserGRPCServer(uSvc))

		logZap.Info("User gRPC Server listening on port 50052...")
		if err := s.Serve(lis); err != nil {
			log.Fatalf("Failed to serve gRPC: %v", err)
		}
	}()

	// 4. Start HTTP Server
	r := gin.New()
	r.Use(gin.Recovery())

	r.POST("/api/v1/users/register", uHandler.Register)
	r.GET("/api/v1/users/:id", uHandler.GetProfile)

	// Add basic healthcheck
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	logZap.Info("User HTTP Server listening on port 8084...")
	if err := r.Run(":8084"); err != nil {
		log.Fatalf("HTTP Server failed to run: %v", err)
	}
}
