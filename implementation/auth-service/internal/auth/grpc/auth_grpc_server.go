package grpc

import (
	"context"

	"github.com/emzhofb/gowallet/auth-service/internal/auth/service"
	pb "github.com/emzhofb/gowallet/proto/auth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AuthGRPCServer struct {
	pb.UnimplementedAuthServiceServer
	svc service.AuthService
}

func NewAuthGRPCServer(svc service.AuthService) *AuthGRPCServer {
	return &AuthGRPCServer{svc: svc}
}

func (s *AuthGRPCServer) ValidateToken(ctx context.Context, req *pb.ValidateTokenRequest) (*pb.ValidateTokenResponse, error) {
	claims, err := s.svc.ValidateToken(ctx, req.Token)
	if err != nil {
		return &pb.ValidateTokenResponse{Valid: false}, status.Errorf(codes.Unauthenticated, "invalid token: %v", err)
	}

	return &pb.ValidateTokenResponse{
		Valid:  true,
		UserId: claims.UserID,
		Role:   claims.Role,
	}, nil
}
