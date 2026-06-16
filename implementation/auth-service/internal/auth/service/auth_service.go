package service

import (
	"context"
	"fmt"
	"time"

	"github.com/emzhofb/gowallet/pkg/config"
	pkgerrors "github.com/emzhofb/gowallet/pkg/errors"
	"github.com/emzhofb/gowallet/pkg/redis"
	pbUser "github.com/emzhofb/gowallet/proto/user"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type Claims struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

type AuthService interface {
	Login(ctx context.Context, email, password string) (string, string, error)
	Refresh(ctx context.Context, refreshToken string) (string, string, error)
	Logout(ctx context.Context, token, refreshToken string) error
	ValidateToken(ctx context.Context, tokenStr string) (*Claims, error)
}

type authService struct {
	rdb        *redis.Client
	userClient pbUser.UserServiceClient
	cfg        *config.Config
}

func NewAuthService(rdb *redis.Client, userClient pbUser.UserServiceClient, cfg *config.Config) AuthService {
	return &authService{
		rdb:        rdb,
		userClient: userClient,
		cfg:        cfg,
	}
}

func (s *authService) Login(ctx context.Context, email, password string) (string, string, error) {
	// Call user service via gRPC to fetch password hash
	resp, err := s.userClient.GetUserByEmail(ctx, &pbUser.GetUserByEmailRequest{Email: email})
	if err != nil {
		return "", "", pkgerrors.ErrUnauthorized
	}

	err = bcrypt.CompareHashAndPassword([]byte(resp.PasswordHash), []byte(password))
	if err != nil {
		return "", "", pkgerrors.ErrUnauthorized
	}

	// Generate Access Token
	accessToken, err := s.generateAccessToken(resp.UserId, resp.Role)
	if err != nil {
		return "", "", err
	}

	// Generate Refresh Token
	refreshToken, err := s.generateRefreshToken(ctx, resp.UserId)
	if err != nil {
		return "", "", err
	}

	return accessToken, refreshToken, nil
}

func (s *authService) Refresh(ctx context.Context, refreshToken string) (string, string, error) {
	// Check if token exists in Redis
	userID, err := s.rdb.Get(ctx, fmt.Sprintf("ref_token:%s", refreshToken))
	if err != nil {
		return "", "", pkgerrors.ErrUnauthorized
	}

	// Rotate token: delete old, create new
	err = s.rdb.Del(ctx, fmt.Sprintf("ref_token:%s", refreshToken))
	if err != nil {
		return "", "", err
	}

	// Fetch user role via gRPC to make sure it's valid
	resp, err := s.userClient.GetUser(ctx, &pbUser.GetUserRequest{UserId: userID})
	if err != nil {
		return "", "", pkgerrors.ErrUnauthorized
	}

	newAccessToken, err := s.generateAccessToken(userID, resp.Role)
	if err != nil {
		return "", "", err
	}

	newRefreshToken, err := s.generateRefreshToken(ctx, userID)
	if err != nil {
		return "", "", err
	}

	return newAccessToken, newRefreshToken, nil
}

func (s *authService) Logout(ctx context.Context, token, refreshToken string) error {
	// Blacklist access token
	claims, err := s.ValidateToken(ctx, token)
	if err == nil {
		ttl := time.Until(claims.ExpiresAt.Time)
		if ttl > 0 {
			s.rdb.Set(ctx, fmt.Sprintf("blacklist:%s", token), "1", ttl)
		}
	}

	// Delete refresh token
	s.rdb.Del(ctx, fmt.Sprintf("ref_token:%s", refreshToken))
	return nil
}

func (s *authService) ValidateToken(ctx context.Context, tokenStr string) (*Claims, error) {
	// Check if blacklisted
	blacklisted, err := s.rdb.Exists(ctx, fmt.Sprintf("blacklist:%s", tokenStr))
	if err == nil && blacklisted {
		return nil, pkgerrors.ErrUnauthorized
	}

	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(s.cfg.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		return nil, pkgerrors.ErrUnauthorized
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, pkgerrors.ErrUnauthorized
	}

	return claims, nil
}

func (s *authService) generateAccessToken(userID, role string) (string, error) {
	claims := Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.cfg.JWTAccessExp)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.cfg.JWTSecret))
}

func (s *authService) generateRefreshToken(ctx context.Context, userID string) (string, error) {
	refreshToken := fmt.Sprintf("rf_%d_%s", time.Now().UnixNano(), userID)
	// Save to Redis
	err := s.rdb.Set(ctx, fmt.Sprintf("ref_token:%s", refreshToken), userID, s.cfg.JWTRefreshExp)
	if err != nil {
		return "", err
	}
	return refreshToken, nil
}
