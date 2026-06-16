package repository

import (
	"database/sql"

	"github.com/emzhofb/gowallet/ledger-service/internal/ledger/model"
)

type LedgerRepository interface {
	Create(entry *model.LedgerEntry) error
}

type mysqlLedgerRepository struct {
	db *sql.DB
}

func NewMySQLLedgerRepository(db *sql.DB) LedgerRepository {
	return &mysqlLedgerRepository{db: db}
}

func (r *mysqlLedgerRepository) Create(entry *model.LedgerEntry) error {
	query := `INSERT INTO ledger_entries (id, transaction_id, wallet_id, type, amount) VALUES (?, ?, ?, ?, ?)`
	_, err := r.db.Exec(query, entry.ID, entry.TransactionID, entry.WalletID, entry.Type, entry.Amount)
	return err
}
