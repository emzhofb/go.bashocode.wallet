package repository

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/emzhofb/gowallet/wallet-service/internal/wallet/model"
)

type WalletRepository interface {
	Create(wallet *model.Wallet) error
	GetByUserID(userID string) (*model.Wallet, error)
	GetByID(id string) (*model.Wallet, error)
	UpdateBalance(id string, amount int64, currentVersion int) (int64, error)
}

type mysqlWalletRepository struct {
	db *sql.DB
}

func NewMySQLWalletRepository(db *sql.DB) WalletRepository {
	return &mysqlWalletRepository{db: db}
}

func (r *mysqlWalletRepository) Create(wallet *model.Wallet) error {
	query := `INSERT INTO wallets (id, user_id, balance, version) VALUES (?, ?, ?, ?)`
	_, err := r.db.Exec(query, wallet.ID, wallet.UserID, wallet.Balance, wallet.Version)
	return err
}

func (r *mysqlWalletRepository) GetByUserID(userID string) (*model.Wallet, error) {
	query := `SELECT id, user_id, balance, version, created_at, updated_at FROM wallets WHERE user_id = ?`
	row := r.db.QueryRow(query, userID)

	var wallet model.Wallet
	err := row.Scan(&wallet.ID, &wallet.UserID, &wallet.Balance, &wallet.Version, &wallet.CreatedAt, &wallet.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &wallet, nil
}

func (r *mysqlWalletRepository) GetByID(id string) (*model.Wallet, error) {
	query := `SELECT id, user_id, balance, version, created_at, updated_at FROM wallets WHERE id = ?`
	row := r.db.QueryRow(query, id)

	var wallet model.Wallet
	err := row.Scan(&wallet.ID, &wallet.UserID, &wallet.Balance, &wallet.Version, &wallet.CreatedAt, &wallet.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &wallet, nil
}

func (r *mysqlWalletRepository) UpdateBalance(id string, amount int64, currentVersion int) (int64, error) {
	// Optimistic locking update query
	query := `UPDATE wallets SET balance = balance + ?, version = version + 1 WHERE id = ? AND version = ?`
	res, err := r.db.Exec(query, amount, id, currentVersion)
	if err != nil {
		return 0, err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}

	if rowsAffected == 0 {
		return 0, fmt.Errorf("optimistic locking failure: record modified by another transaction")
	}

	// Fetch new balance
	var newBalance int64
	err = r.db.QueryRow("SELECT balance FROM wallets WHERE id = ?", id).Scan(&newBalance)
	if err != nil {
		return 0, err
	}

	return newBalance, nil
}
