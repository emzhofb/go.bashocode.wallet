package main

import (
	"log"
	"net/http"
	"os"

	"github.com/emzhofb/gowallet/api-gateway/internal/middleware"
	"github.com/emzhofb/gowallet/api-gateway/internal/proxy"
	"github.com/emzhofb/gowallet/pkg/config"
	"github.com/emzhofb/gowallet/pkg/logger"
	pbAuth "github.com/emzhofb/gowallet/proto/auth"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	cfg := config.LoadConfig()
	logZap := logger.NewLogger(cfg.AppEnv, "api-gateway")
	logZap.Info("Starting API Gateway...")

	// 1. Dial Auth Service gRPC for validation
	authGRPCAddr := os.Getenv("AUTH_SERVICE_GRPC_ADDR")
	if authGRPCAddr == "" {
		authGRPCAddr = "localhost:50051"
	}
	authConn, err := grpc.Dial(authGRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to Auth gRPC: %v", err)
	}
	defer authConn.Close()
	authClient := pbAuth.NewAuthServiceClient(authConn)

	// 2. Setup proxies
	authServiceUrl := os.Getenv("AUTH_SERVICE_URL")
	if authServiceUrl == "" {
		authServiceUrl = "http://localhost:8081"
	}
	userServiceUrl := os.Getenv("USER_SERVICE_URL")
	if userServiceUrl == "" {
		userServiceUrl = "http://localhost:8084"
	}
	walletServiceUrl := os.Getenv("WALLET_SERVICE_URL")
	if walletServiceUrl == "" {
		walletServiceUrl = "http://localhost:8082"
	}
	transactionServiceUrl := os.Getenv("TRANSACTION_SERVICE_URL")
	if transactionServiceUrl == "" {
		transactionServiceUrl = "http://localhost:8086"
	}
	paymentServiceUrl := os.Getenv("PAYMENT_SERVICE_URL")
	if paymentServiceUrl == "" {
		paymentServiceUrl = "http://localhost:8083"
	}

	authProxy, _ := proxy.NewReverseProxy(authServiceUrl)
	userProxy, _ := proxy.NewReverseProxy(userServiceUrl)
	walletProxy, _ := proxy.NewReverseProxy(walletServiceUrl)
	transactionProxy, _ := proxy.NewReverseProxy(transactionServiceUrl)
	paymentProxy, _ := proxy.NewReverseProxy(paymentServiceUrl)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.CORSMiddleware())

	// 3. Define routes
	// Public routes
	r.Any("/api/v1/auth/login", func(c *gin.Context) {
		authProxy.ServeHTTP(c.Writer, c.Request)
	})
	r.Any("/api/v1/auth/refresh", func(c *gin.Context) {
		authProxy.ServeHTTP(c.Writer, c.Request)
	})
	r.Any("/api/v1/users/register", func(c *gin.Context) {
		userProxy.ServeHTTP(c.Writer, c.Request)
	})
	r.Any("/api/v1/payments/webhook", func(c *gin.Context) {
		paymentProxy.ServeHTTP(c.Writer, c.Request)
	})

	// Protected routes (JWT middleware protected)
	protected := r.Group("/api/v1")
	protected.Use(middleware.AuthMiddleware(authClient))
	{
		protected.Any("/auth/logout", func(c *gin.Context) {
			authProxy.ServeHTTP(c.Writer, c.Request)
		})
		protected.Any("/users/:id", func(c *gin.Context) {
			userProxy.ServeHTTP(c.Writer, c.Request)
		})
		protected.Any("/wallets/*path", func(c *gin.Context) {
			walletProxy.ServeHTTP(c.Writer, c.Request)
		})
		protected.Any("/transactions/*path", func(c *gin.Context) {
			transactionProxy.ServeHTTP(c.Writer, c.Request)
		})
	}

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	logZap.Info("API Gateway listening on port 8080...")
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("Gateway failed: %v", err)
	}
}
