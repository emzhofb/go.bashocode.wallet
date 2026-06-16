package main

import (
	"log"
	"net"
	"net/http"
	"os"

	"github.com/emzhofb/gowallet/auth-service/internal/auth/grpc"
	"github.com/emzhofb/gowallet/auth-service/internal/auth/handler"
	"github.com/emzhofb/gowallet/auth-service/internal/auth/service"
	"github.com/emzhofb/gowallet/pkg/config"
	"github.com/emzhofb/gowallet/pkg/logger"
	"github.com/emzhofb/gowallet/pkg/redis"
	pbAuth "github.com/emzhofb/gowallet/proto/auth"
	pbUser "github.com/emzhofb/gowallet/proto/user"
	"github.com/gin-gonic/gin"
	googlegrpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	cfg := config.LoadConfig()
	logZap := logger.NewLogger(cfg.AppEnv, "auth-service")
	logZap.Info("Starting Auth Service...")

	// 1. Connect to Redis
	rdb, err := redis.NewRedisClient(cfg.RedisHost, cfg.RedisPort)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	// 2. gRPC Client to User Service
	userGRPCAddr := os.Getenv("USER_GRPC_ADDR")
	if userGRPCAddr == "" {
		userGRPCAddr = "localhost:50052"
	}
	conn, err := googlegrpc.Dial(userGRPCAddr, googlegrpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to user gRPC server: %v", err)
	}
	defer conn.Close()

	userClient := pbUser.NewUserServiceClient(conn)

	// 3. Setup Components
	authSvc := service.NewAuthService(rdb, userClient, cfg)
	authHandler := handler.NewAuthHandler(authSvc)

	// 4. Start gRPC Server
	go func() {
		lis, err := net.Listen("tcp", ":50051")
		if err != nil {
			log.Fatalf("gRPC Server failed to listen: %v", err)
		}

		s := googlegrpc.NewServer()
		pbAuth.RegisterAuthServiceServer(s, grpc.NewAuthGRPCServer(authSvc))

		logZap.Info("Auth gRPC Server listening on port 50051...")
		if err := s.Serve(lis); err != nil {
			log.Fatalf("Failed to serve gRPC: %v", err)
		}
	}()

	// 5. Start HTTP Server
	r := gin.New()
	r.Use(gin.Recovery())

	r.POST("/api/v1/auth/login", authHandler.Login)
	r.POST("/api/v1/auth/refresh", authHandler.Refresh)
	r.POST("/api/v1/auth/logout", authHandler.Logout)

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	logZap.Info("Auth HTTP Server listening on port 8081...")
	if err := r.Run(":8081"); err != nil {
		log.Fatalf("HTTP Server failed to run: %v", err)
	}
}
