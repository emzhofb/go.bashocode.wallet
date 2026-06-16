package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/emzhofb/gowallet/transaction-service/internal/transaction/model"
)

type TransactionRepository interface {
	GetByIdempotencyKey(ctx context.Context, key string) (*model.Transaction, error)
	CreateWithOutbox(ctx context.Context, tx *model.Transaction, event *model.OutboxEvent) error
	UpdateStatus(ctx context.Context, id string, status string) error
	GetPendingOutbox(ctx context.Context) ([]*model.OutboxEvent, error)
	MarkOutboxProcessed(ctx context.Context, id string) error
}

type mysqlTransactionRepository struct {
	db *sql.DB
}

func NewMySQLTransactionRepository(db *sql.DB) TransactionRepository {
	return &mysqlTransactionRepository{db: db}
}

func (r *mysqlTransactionRepository) GetByIdempotencyKey(ctx context.Context, key string) (*model.Transaction, error) {
	query := `SELECT id, sender_wallet_id, receiver_wallet_id, amount, status, idempotency_key, created_at, updated_at FROM transactions WHERE idempotency_key = ?`
	row := r.db.QueryRowContext(ctx, query, key)

	var tx model.Transaction
	err := row.Scan(&tx.ID, &tx.SenderWalletID, &tx.ReceiverWalletID, &tx.Amount, &tx.Status, &tx.IdempotencyKey, &tx.CreatedAt, &tx.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &tx, nil
}

func (r *mysqlTransactionRepository) CreateWithOutbox(ctx context.Context, tx *model.Transaction, event *model.OutboxEvent) error {
	dbTx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer dbTx.Rollback()

	// 1. Insert Transaction
	txQuery := `INSERT INTO transactions (id, sender_wallet_id, receiver_wallet_id, amount, status, idempotency_key) VALUES (?, ?, ?, ?, ?, ?)`
	_, err = dbTx.ExecContext(ctx, txQuery, tx.ID, tx.SenderWalletID, tx.ReceiverWalletID, tx.Amount, tx.Status, tx.IdempotencyKey)
	if err != nil {
		return err
	}

	// 2. Insert Outbox Event
	if event != nil {
		eventQuery := `INSERT INTO outbox_events (id, event_type, payload, status) VALUES (?, ?, ?, ?)`
		_, err = dbTx.ExecContext(ctx, eventQuery, event.ID, event.EventType, event.Payload, event.Status)
		if err != nil {
			return err
		}
	}

	return dbTx.Commit()
}

func (r *mysqlTransactionRepository) UpdateStatus(ctx context.Context, id string, status string) error {
	query := `UPDATE transactions SET status = ? WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, status, id)
	return err
}

func (r *mysqlTransactionRepository) GetPendingOutbox(ctx context.Context) ([]*model.OutboxEvent, error) {
	query := `SELECT id, event_type, payload, status, created_at FROM outbox_events WHERE status = 'pending' LIMIT 50`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*model.OutboxEvent
	for rows.Next() {
		var event model.OutboxEvent
		err := rows.Scan(&event.ID, &event.EventType, &event.Payload, &event.Status, &event.CreatedAt)
		if err != nil {
			return nil, err
		}
		events = append(events, &event)
	}
	return events, nil
}

func (r *mysqlTransactionRepository) MarkOutboxProcessed(ctx context.Context, id string) error {
	query := `UPDATE outbox_events SET status = 'processed' WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}
