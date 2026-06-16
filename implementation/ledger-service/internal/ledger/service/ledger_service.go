package service

import (
	"context"

	"github.com/emzhofb/gowallet/ledger-service/internal/ledger/model"
	"github.com/emzhofb/gowallet/ledger-service/internal/ledger/repository"
	"github.com/google/uuid"
)

type LedgerService interface {
	RecordEntry(ctx context.Context, txID, walletID, entryType string, amount int64) (*model.LedgerEntry, error)
}

type ledgerService struct {
	repo repository.LedgerRepository
}

func NewLedgerService(repo repository.LedgerRepository) LedgerService {
	return &ledgerService{repo: repo}
}

func (s *ledgerService) RecordEntry(ctx context.Context, txID, walletID, entryType string, amount int64) (*model.LedgerEntry, error) {
	entry := &model.LedgerEntry{
		ID:            uuid.New().String(),
		TransactionID: txID,
		WalletID:      walletID,
		Type:          entryType,
		Amount:        amount,
	}

	err := s.repo.Create(entry)
	if err != nil {
		return nil, err
	}

	return entry, nil
}
