package model

import "time"

type LedgerEntry struct {
	ID            string    `json:"id"`
	TransactionID string    `json:"transaction_id"`
	WalletID      string    `json:"wallet_id"`
	Type          string    `json:"type"` // "debit" or "credit"
	Amount        int64     `json:"amount"`
	CreatedAt     time.Time `json:"created_at"`
}
