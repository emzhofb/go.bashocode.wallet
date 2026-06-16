package model

import "time"

type Transaction struct {
	ID               string    `json:"id"`
	SenderWalletID   string    `json:"sender_wallet_id"`
	ReceiverWalletID string    `json:"receiver_wallet_id"`
	Amount           int64     `json:"amount"`
	Status           string    `json:"status"` // "PENDING", "SUCCESS", "FAILED"
	IdempotencyKey   string    `json:"idempotency_key"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type OutboxEvent struct {
	ID        string    `json:"id"`
	EventType string    `json:"event_type"`
	Payload   string    `json:"payload"`
	Status    string    `json:"status"` // "pending", "processed"
	CreatedAt time.Time `json:"created_at"`
}

type TransferRequest struct {
	ReceiverEmail  string `json:"receiver_email" binding:"required,email"`
	Amount         int64  `json:"amount" binding:"required,gt=0"`
	IdempotencyKey string `json:"idempotency_key" binding:"required"`
}
