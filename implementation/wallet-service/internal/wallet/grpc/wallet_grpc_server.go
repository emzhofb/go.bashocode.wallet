package grpc

import (
	"context"

	pb "github.com/emzhofb/gowallet/proto/wallet"
	"github.com/emzhofb/gowallet/wallet-service/internal/wallet/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type WalletGRPCServer struct {
	pb.UnimplementedWalletServiceServer
	svc service.WalletService
}

func NewWalletGRPCServer(svc service.WalletService) *WalletGRPCServer {
	return &WalletGRPCServer{svc: svc}
}

func (s *WalletGRPCServer) GetBalance(ctx context.Context, req *pb.GetBalanceRequest) (*pb.GetBalanceResponse, error) {
	w, err := s.svc.GetByUserID(ctx, req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "wallet not found: %v", err)
	}

	return &pb.GetBalanceResponse{
		WalletId: w.ID,
		UserId:   w.UserID,
		Balance:  w.Balance,
	}, nil
}

func (s *WalletGRPCServer) UpdateBalance(ctx context.Context, req *pb.UpdateBalanceRequest) (*pb.UpdateBalanceResponse, error) {
	newBal, err := s.svc.UpdateBalanceWithRetry(ctx, req.WalletId, req.Amount)
	if err != nil {
		return &pb.UpdateBalanceResponse{Success: false}, status.Errorf(codes.FailedPrecondition, "failed to update balance: %v", err)
	}

	return &pb.UpdateBalanceResponse{
		Success:    true,
		NewBalance: newBal,
	}, nil
}
