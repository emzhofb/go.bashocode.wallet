package service

import (
	"context"
	"fmt"
	"time"

	pkgerrors "github.com/emzhofb/gowallet/pkg/errors"
	"github.com/emzhofb/gowallet/wallet-service/internal/wallet/model"
	"github.com/emzhofb/gowallet/wallet-service/internal/wallet/repository"
	"github.com/google/uuid"
)

type WalletService interface {
	CreateWallet(ctx context.Context, userID string) (*model.Wallet, error)
	GetByUserID(ctx context.Context, userID string) (*model.Wallet, error)
	GetByID(ctx context.Context, id string) (*model.Wallet, error)
	UpdateBalanceWithRetry(ctx context.Context, id string, amount int64) (int64, error)
}

type walletService struct {
	repo repository.WalletRepository
}

func NewWalletService(repo repository.WalletRepository) WalletService {
	return &walletService{repo: repo}
}

func (s *walletService) CreateWallet(ctx context.Context, userID string) (*model.Wallet, error) {
	existing, err := s.repo.GetByUserID(userID)
	if err == nil && existing != nil {
		return nil, pkgerrors.ErrDuplicateEntry
	}

	wallet := &model.Wallet{
		ID:      uuid.New().String(),
		UserID:  userID,
		Balance: 0,
		Version: 1,
	}

	err = s.repo.Create(wallet)
	if err != nil {
		return nil, err
	}

	return wallet, nil
}

func (s *walletService) GetByUserID(ctx context.Context, userID string) (*model.Wallet, error) {
	w, err := s.repo.GetByUserID(userID)
	if err != nil {
		return nil, err
	}
	if w == nil {
		return nil, pkgerrors.ErrNotFound
	}
	return w, nil
}

func (s *walletService) GetByID(ctx context.Context, id string) (*model.Wallet, error) {
	w, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if w == nil {
		return nil, pkgerrors.ErrNotFound
	}
	return w, nil
}

func (s *walletService) UpdateBalanceWithRetry(ctx context.Context, id string, amount int64) (int64, error) {
	const maxRetries = 3
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		w, err := s.repo.GetByID(id)
		if err != nil {
			return 0, err
		}
		if w == nil {
			return 0, pkgerrors.ErrNotFound
		}

		// Check if debit would cause negative balance
		if amount < 0 && w.Balance+amount < 0 {
			return 0, pkgerrors.ErrInsufficientBalance
		}

		newBal, err := s.repo.UpdateBalance(id, amount, w.Version)
		if err == nil {
			return newBal, nil
		}

		lastErr = err
		// Exponential backoff or sleep before retry
		time.Sleep(time.Duration(50*(i+1)) * time.Millisecond)
	}

	return 0, fmt.Errorf("failed to update balance after %d retries: %v", maxRetries, lastErr)
}
