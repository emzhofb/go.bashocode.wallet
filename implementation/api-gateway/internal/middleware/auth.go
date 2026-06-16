package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	pbAuth "github.com/emzhofb/gowallet/proto/auth"
	"github.com/gin-gonic/gin"
)

func AuthMiddleware(authClient pbAuth.AuthServiceClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Authorization header is required"})
			c.Abort()
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Authorization header format must be Bearer {token}"})
			c.Abort()
			return
		}

		token := parts[1]

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		resp, err := authClient.ValidateToken(ctx, &pbAuth.ValidateTokenRequest{Token: token})
		if err != nil || !resp.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Invalid or expired token"})
			c.Abort()
			return
		}

		// Inject identity headers for downstream microservices
		c.Request.Header.Set("X-User-ID", resp.UserId)
		c.Request.Header.Set("X-User-Role", resp.Role)

		c.Next()
	}
}
