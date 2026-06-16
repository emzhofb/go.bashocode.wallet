package grpc

import (
	"context"

	pb "github.com/emzhofb/gowallet/proto/user"
	"github.com/emzhofb/gowallet/user-service/internal/user/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type UserGRPCServer struct {
	pb.UnimplementedUserServiceServer
	svc service.UserService
}

func NewUserGRPCServer(svc service.UserService) *UserGRPCServer {
	return &UserGRPCServer{svc: svc}
}

func (s *UserGRPCServer) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.GetUserResponse, error) {
	user, err := s.svc.GetProfile(req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "user not found: %v", err)
	}

	return &pb.GetUserResponse{
		UserId:       user.ID,
		Email:        user.Email,
		Name:         user.Name,
		Role:         user.Role,
		PasswordHash: user.Password,
	}, nil
}

func (s *UserGRPCServer) GetUserByEmail(ctx context.Context, req *pb.GetUserByEmailRequest) (*pb.GetUserResponse, error) {
	user, err := s.svc.GetByEmail(req.Email)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "user not found: %v", err)
	}

	return &pb.GetUserResponse{
		UserId:       user.ID,
		Email:        user.Email,
		Name:         user.Name,
		Role:         user.Role,
		PasswordHash: user.Password,
	}, nil
}
