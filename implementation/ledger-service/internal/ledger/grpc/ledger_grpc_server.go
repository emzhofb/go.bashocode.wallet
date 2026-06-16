package grpc

import (
	"context"

	"github.com/emzhofb/gowallet/ledger-service/internal/ledger/service"
	pb "github.com/emzhofb/gowallet/proto/ledger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type LedgerGRPCServer struct {
	pb.UnimplementedLedgerServiceServer
	svc service.LedgerService
}

func NewLedgerGRPCServer(svc service.LedgerService) *LedgerGRPCServer {
	return &LedgerGRPCServer{svc: svc}
}

func (s *LedgerGRPCServer) RecordEntry(ctx context.Context, req *pb.RecordEntryRequest) (*pb.RecordEntryResponse, error) {
	entry, err := s.svc.RecordEntry(ctx, req.TransactionId, req.WalletId, req.Type, req.Amount)
	if err != nil {
		return &pb.RecordEntryResponse{Success: false}, status.Errorf(codes.Internal, "failed to record entry: %v", err)
	}

	return &pb.RecordEntryResponse{
		Success: true,
		EntryId: entry.ID,
	}, nil
}
