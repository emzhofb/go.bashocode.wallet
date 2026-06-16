package service

import (
	"context"
	"encoding/json"
	"fmt"

	pkgerrors "github.com/emzhofb/gowallet/pkg/errors"
	ledgerPb "github.com/emzhofb/gowallet/proto/ledger"
	userPb "github.com/emzhofb/gowallet/proto/user"
	walletPb "github.com/emzhofb/gowallet/proto/wallet"
	"github.com/emzhofb/gowallet/transaction-service/internal/transaction/model"
	"github.com/emzhofb/gowallet/transaction-service/internal/transaction/repository"
	"github.com/google/uuid"
)

type TransactionService interface {
	Transfer(ctx context.Context, senderUserID string, req model.TransferRequest) (*model.Transaction, error)
}

type transactionService struct {
	repo         repository.TransactionRepository
	userClient   userPb.UserServiceClient
	walletClient walletPb.WalletServiceClient
	ledgerClient ledgerPb.LedgerServiceClient
}

func NewTransactionService(
	repo repository.TransactionRepository,
	userClient userPb.UserServiceClient,
	walletClient walletPb.WalletServiceClient,
	ledgerClient ledgerPb.LedgerServiceClient,
) TransactionService {
	return &transactionService{
		repo:         repo,
		userClient:   userClient,
		walletClient: walletClient,
		ledgerClient: ledgerClient,
	}
}

func (s *transactionService) Transfer(ctx context.Context, senderUserID string, req model.TransferRequest) (*model.Transaction, error) {
	// 1. Check Idempotency Key
	existing, _ := s.repo.GetByIdempotencyKey(ctx, req.IdempotencyKey)
	if existing != nil {
		return existing, nil
	}

	// 2. Find receiver details
	receiverUser, err := s.userClient.GetUserByEmail(ctx, &userPb.GetUserByEmailRequest{Email: req.ReceiverEmail})
	if err != nil {
		return nil, fmt.Errorf("receiver email not found: %v", err)
	}

	// 3. Get sender wallet details
	senderWallet, err := s.walletClient.GetBalance(ctx, &walletPb.GetBalanceRequest{UserId: senderUserID})
	if err != nil {
		return nil, fmt.Errorf("sender wallet not found: %v", err)
	}

	if senderWallet.Balance < req.Amount {
		return nil, pkgerrors.ErrInsufficientBalance
	}

	// 4. Get receiver wallet details
	receiverWallet, err := s.walletClient.GetBalance(ctx, &walletPb.GetBalanceRequest{UserId: receiverUser.UserId})
	if err != nil {
		return nil, fmt.Errorf("receiver wallet not found: %v", err)
	}

	txID := uuid.New().String()
	txRecord := &model.Transaction{
		ID:               txID,
		SenderWalletID:   senderWallet.WalletId,
		ReceiverWalletID: receiverWallet.WalletId,
		Amount:           req.Amount,
		Status:           "PENDING",
		IdempotencyKey:   req.IdempotencyKey,
	}

	eventPayload, _ := json.Marshal(map[string]interface{}{
		"transaction_id":     txID,
		"sender_wallet_id":   senderWallet.WalletId,
		"receiver_wallet_id": receiverWallet.WalletId,
		"amount":             req.Amount,
		"sender_user_id":     senderUserID,
		"receiver_user_id":   receiverUser.UserId,
	})

	outboxEvent := &model.OutboxEvent{
		ID:        uuid.New().String(),
		EventType: "transfer.completed",
		Payload:   string(eventPayload),
		Status:    "pending",
	}

	// Save pending transaction and outbox event
	err = s.repo.CreateWithOutbox(ctx, txRecord, outboxEvent)
	if err != nil {
		return nil, err
	}

	// 5. Debit sender balance
	_, err = s.walletClient.UpdateBalance(ctx, &walletPb.UpdateBalanceRequest{
		WalletId:      senderWallet.WalletId,
		Amount:        -req.Amount,
		TransactionId: txID,
	})
	if err != nil {
		s.repo.UpdateStatus(ctx, txID, "FAILED")
		return nil, fmt.Errorf("failed to debit sender: %v", err)
	}

	// 6. Credit receiver balance
	_, err = s.walletClient.UpdateBalance(ctx, &walletPb.UpdateBalanceRequest{
		WalletId:      receiverWallet.WalletId,
		Amount:        req.Amount,
		TransactionId: txID,
	})
	if err != nil {
		// Compensation: Refund sender
		s.walletClient.UpdateBalance(ctx, &walletPb.UpdateBalanceRequest{
			WalletId:      senderWallet.WalletId,
			Amount:        req.Amount,
			TransactionId: txID,
		})
		s.repo.UpdateStatus(ctx, txID, "FAILED")
		return nil, fmt.Errorf("failed to credit receiver: %v", err)
	}

	// 7. Write Ledger Debit entry
	_, err = s.ledgerClient.RecordEntry(ctx, &ledgerPb.RecordEntryRequest{
		TransactionId: txID,
		WalletId:      senderWallet.WalletId,
		Type:          "debit",
		Amount:        req.Amount,
	})
	if err != nil {
		// Compensation: Refund receiver & sender
		s.walletClient.UpdateBalance(ctx, &walletPb.UpdateBalanceRequest{
			WalletId:      receiverWallet.WalletId,
			Amount:        -req.Amount,
			TransactionId: txID,
		})
		s.walletClient.UpdateBalance(ctx, &walletPb.UpdateBalanceRequest{
			WalletId:      senderWallet.WalletId,
			Amount:        req.Amount,
			TransactionId: txID,
		})
		s.repo.UpdateStatus(ctx, txID, "FAILED")
		return nil, fmt.Errorf("failed to write ledger entry for debit: %v", err)
	}

	// 8. Write Ledger Credit entry
	_, err = s.ledgerClient.RecordEntry(ctx, &ledgerPb.RecordEntryRequest{
		TransactionId: txID,
		WalletId:      receiverWallet.WalletId,
		Type:          "credit",
		Amount:        req.Amount,
	})
	if err != nil {
		// Compensation: Refund receiver & sender
		s.walletClient.UpdateBalance(ctx, &walletPb.UpdateBalanceRequest{
			WalletId:      receiverWallet.WalletId,
			Amount:        -req.Amount,
			TransactionId: txID,
		})
		s.walletClient.UpdateBalance(ctx, &walletPb.UpdateBalanceRequest{
			WalletId:      senderWallet.WalletId,
			Amount:        req.Amount,
			TransactionId: txID,
		})

		// Compensation: Counter ledger entry (CREDIT to balance original DEBIT)
		s.ledgerClient.RecordEntry(ctx, &ledgerPb.RecordEntryRequest{
			TransactionId: txID,
			WalletId:      senderWallet.WalletId,
			Type:          "credit",
			Amount:        req.Amount,
		})

		s.repo.UpdateStatus(ctx, txID, "FAILED")
		return nil, fmt.Errorf("failed to write ledger entry for credit: %v", err)
	}

	// 9. Mark status SUCCESS
	err = s.repo.UpdateStatus(ctx, txID, "SUCCESS")
	if err != nil {
		return nil, err
	}

	txRecord.Status = "SUCCESS"
	return txRecord, nil
}
