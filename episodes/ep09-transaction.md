# Episode 9: Transfer & Transaction Service

## 🎯 Tujuan
- Transfer antar pengguna
- Idempotency key untuk mencegah duplikasi
- Transaction history & detail (paginated)
- Outbox Pattern untuk event publishing

## 📝 Prerequisites
- Episode 7 & 8 selesai (Wallet + Ledger Service)

---

## 📦 Langkah-langkah

### Step 1: Inisialisasi Service

```bash
mkdir -p transaction-service/cmd
mkdir -p transaction-service/internal/{handler,service,repository,model,dto,config}
mkdir -p transaction-service/db/migrations

cd transaction-service
go mod init github.com/emzhofb/gowallet/transaction-service
cd ..
go work use ./transaction-service
```

### Step 2: Database Migration

**`000001_create_transactions.up.sql`:**
```sql
CREATE TABLE IF NOT EXISTS transactions (
    id CHAR(36) PRIMARY KEY,
    idempotency_key VARCHAR(255) UNIQUE NOT NULL,
    type ENUM('transfer', 'topup', 'withdraw', 'payment') NOT NULL,
    sender_wallet_id CHAR(36) NULL,
    receiver_wallet_id CHAR(36) NULL,
    amount DECIMAL(15,2) NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'IDR',
    status ENUM('pending', 'completed', 'failed', 'reversed') NOT NULL DEFAULT 'pending',
    description VARCHAR(500) DEFAULT '',
    failure_reason VARCHAR(500) DEFAULT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    INDEX idx_sender (sender_wallet_id),
    INDEX idx_receiver (receiver_wallet_id),
    INDEX idx_idempotency (idempotency_key),
    INDEX idx_status (status),
    INDEX idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

**`000002_create_outbox_events.up.sql`:**
```sql
CREATE TABLE IF NOT EXISTS outbox_events (
    id CHAR(36) PRIMARY KEY,
    aggregate_type VARCHAR(100) NOT NULL,
    aggregate_id CHAR(36) NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    payload JSON NOT NULL,
    published BOOLEAN NOT NULL DEFAULT FALSE,
    published_at TIMESTAMP NULL DEFAULT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    INDEX idx_unpublished (published, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

### Step 3: Endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/api/v1/transactions/transfer` | ✅ | Transfer saldo ke user lain |
| GET | `/api/v1/transactions` | ✅ | Transaction history |
| GET | `/api/v1/transactions/:id` | ✅ | Transaction detail |

### Step 4: DTOs

```go
type TransferRequest struct {
    ReceiverID     string  `json:"receiver_id" binding:"required,uuid"`
    Amount         float64 `json:"amount" binding:"required,gt=0"`
    IdempotencyKey string  `json:"idempotency_key" binding:"required,uuid"`
    Description    string  `json:"description" binding:"max=500"`
}

type TransactionResponse struct {
    ID               string  `json:"id"`
    Type             string  `json:"type"`
    SenderWalletID   *string `json:"sender_wallet_id"`
    ReceiverWalletID *string `json:"receiver_wallet_id"`
    Amount           float64 `json:"amount"`
    Currency         string  `json:"currency"`
    Status           string  `json:"status"`
    Description      string  `json:"description"`
    CreatedAt        string  `json:"created_at"`
}

type TransactionListQuery struct {
    Page  int    `form:"page,default=1"`
    Limit int    `form:"limit,default=10"`
    Type  string `form:"type"`     // filter: transfer, topup, withdraw
    Sort  string `form:"sort,default=created_at"`
    Order string `form:"order,default=desc"`
}
```

### Step 5: Transfer Flow (DETAIL)

Ini adalah bagian paling kompleks. Ikuti step-by-step:

```
Client mengirim:
POST /api/v1/transactions/transfer
{
  "receiver_id": "user-uuid-receiver",
  "amount": 50000,
  "idempotency_key": "unique-uuid",
  "description": "Bayar makan siang"
}
```

```go
func (s *transactionService) Transfer(ctx context.Context, senderUserID string, req dto.TransferRequest) (*dto.TransactionResponse, error) {
    
    // ═══════════════════════════════════════
    // STEP 1: IDEMPOTENCY CHECK
    // ═══════════════════════════════════════
    existing, _ := s.txRepo.GetByIdempotencyKey(ctx, req.IdempotencyKey)
    if existing != nil {
        // Sudah pernah diproses → return result sebelumnya
        return toResponse(existing), nil
    }
    
    // ═══════════════════════════════════════
    // STEP 2: VALIDASI
    // ═══════════════════════════════════════
    // 2a. Sender ≠ Receiver
    if senderUserID == req.ReceiverID {
        return nil, ErrCannotTransferToSelf
    }
    
    // 2b. Get sender wallet (via gRPC ke Wallet Service)
    senderWallet, err := s.walletClient.GetByUserID(ctx, senderUserID)
    if err != nil || senderWallet == nil {
        return nil, ErrWalletNotFound
    }
    
    // 2c. Cek sender wallet active
    if senderWallet.Status != "active" {
        return nil, ErrWalletFrozen
    }
    
    // 2d. Get receiver wallet
    receiverWallet, err := s.walletClient.GetByUserID(ctx, req.ReceiverID)
    if err != nil || receiverWallet == nil {
        return nil, ErrReceiverNotFound
    }
    
    // 2e. Cek receiver wallet active
    if receiverWallet.Status != "active" {
        return nil, ErrReceiverWalletFrozen
    }
    
    // 2f. Cek saldo cukup
    if senderWallet.Balance < req.Amount {
        return nil, ErrInsufficientBalance
    }
    
    // ═══════════════════════════════════════
    // STEP 3: EXECUTE (dalam 1 DB transaction)
    // ═══════════════════════════════════════
    tx, _ := s.db.BeginTx(ctx, nil)
    defer tx.Rollback()
    
    transactionID := uuid.New().String()
    
    // 3a. Create transaction record (status: pending)
    transaction := &model.Transaction{
        ID:               transactionID,
        IdempotencyKey:   req.IdempotencyKey,
        Type:             "transfer",
        SenderWalletID:   &senderWallet.ID,
        ReceiverWalletID: &receiverWallet.ID,
        Amount:           req.Amount,
        Status:           "pending",
        Description:      req.Description,
    }
    s.txRepo.CreateTx(ctx, tx, transaction)
    
    // 3b. Debit sender wallet (via gRPC, optimistic locking)
    err = s.walletClient.UpdateBalance(ctx, senderWallet.ID, -req.Amount, senderWallet.Version)
    if err != nil {
        // Rollback, update status → failed
        transaction.Status = "failed"
        transaction.FailureReason = err.Error()
        s.txRepo.UpdateStatusTx(ctx, tx, transaction)
        tx.Commit()
        return nil, err
    }
    
    // 3c. Credit receiver wallet
    err = s.walletClient.UpdateBalance(ctx, receiverWallet.ID, req.Amount, receiverWallet.Version)
    if err != nil {
        // PERLU ROLLBACK sender juga!
        s.walletClient.UpdateBalance(ctx, senderWallet.ID, req.Amount, senderWallet.Version+1)
        transaction.Status = "failed"
        s.txRepo.UpdateStatusTx(ctx, tx, transaction)
        tx.Commit()
        return nil, err
    }
    
    // 3d. Create ledger entries via gRPC
    s.ledgerClient.CreateTransferEntries(ctx, &ledgerpb.CreateTransferRequest{
        TransactionId:    transactionID,
        SenderWalletId:   senderWallet.ID,
        ReceiverWalletId: receiverWallet.ID,
        Amount:           req.Amount,
        Description:      req.Description,
    })
    
    // 3e. Update transaction status → completed
    transaction.Status = "completed"
    s.txRepo.UpdateStatusTx(ctx, tx, transaction)
    
    // 3f. Insert outbox event
    eventPayload, _ := json.Marshal(map[string]interface{}{
        "transaction_id":    transactionID,
        "sender_user_id":    senderUserID,
        "receiver_user_id":  req.ReceiverID,
        "amount":            req.Amount,
        "description":       req.Description,
    })
    
    s.outboxRepo.CreateTx(ctx, tx, &model.OutboxEvent{
        ID:            uuid.New().String(),
        AggregateType: "transaction",
        AggregateID:   transactionID,
        EventType:     "transfer.completed",
        Payload:       eventPayload,
    })
    
    // 3g. COMMIT transaction
    tx.Commit()
    
    return toResponse(transaction), nil
}
```

### Step 6: Outbox Pattern — Background Worker

```go
// Outbox worker berjalan di background goroutine
// Membaca event yang belum di-publish dan publish ke RabbitMQ

func (w *OutboxWorker) Start(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            w.processOutbox(ctx)
        }
    }
}

func (w *OutboxWorker) processOutbox(ctx context.Context) {
    // 1. Query unpublished events (batch 10)
    events, _ := w.outboxRepo.GetUnpublished(ctx, 10)
    
    for _, event := range events {
        // 2. Publish ke RabbitMQ
        err := w.publisher.Publish(ctx, event.EventType, event.Payload)
        if err != nil {
            w.logger.Error("failed to publish event",
                zap.String("event_id", event.ID),
                zap.Error(err))
            continue // Skip, akan di-retry di iterasi berikutnya
        }
        
        // 3. Mark as published
        w.outboxRepo.MarkPublished(ctx, event.ID)
    }
}
```

> **Kenapa Outbox Pattern?**
> ```
> TANPA Outbox:
>   DB commit ✅ → RabbitMQ publish ❌ (network error)
>   → Data tersimpan tapi event hilang!
>
> DENGAN Outbox:
>   DB commit (data + event) ✅ → Worker baca event → RabbitMQ publish
>   → Jika publish gagal, worker retry di iterasi berikutnya
>   → Event PASTI ter-publish (at least once delivery)
> ```

### Step 7: Transaction History

```go
// GET /api/v1/transactions?page=1&limit=10&type=transfer

func (r *txRepo) ListByUserID(ctx context.Context, userID string, query dto.TransactionListQuery) ([]model.Transaction, int, error) {
    // User bisa melihat transaksi dimana dia sebagai sender ATAU receiver
    
    countQuery := `SELECT COUNT(*) FROM transactions t
        JOIN wallets sw ON t.sender_wallet_id = sw.id
        JOIN wallets rw ON t.receiver_wallet_id = rw.id
        WHERE (sw.user_id = ? OR rw.user_id = ?)`
    
    listQuery := `SELECT t.* FROM transactions t
        JOIN wallets sw ON t.sender_wallet_id = sw.id
        JOIN wallets rw ON t.receiver_wallet_id = rw.id
        WHERE (sw.user_id = ? OR rw.user_id = ?)
        ORDER BY t.created_at DESC
        LIMIT ? OFFSET ?`
    
    // Add type filter if specified
    // Add pagination
}
```

### Step 8: Test Manual

```bash
# Setup: Register 2 users, top up user 1

# 1. Transfer
curl -X POST -H "Authorization: Bearer $TOKEN_USER1" \
  -H "Content-Type: application/json" \
  http://localhost:8085/api/v1/transactions/transfer \
  -d '{
    "receiver_id": "'$USER2_ID'",
    "amount": 50000,
    "idempotency_key": "'$(uuidgen)'",
    "description": "Bayar makan"
  }'

# 2. Cek saldo user 1 (seharusnya berkurang)
curl -H "Authorization: Bearer $TOKEN_USER1" http://localhost:8083/api/v1/wallets/me

# 3. Cek saldo user 2 (seharusnya bertambah)
curl -H "Authorization: Bearer $TOKEN_USER2" http://localhost:8083/api/v1/wallets/me

# 4. Transaction history
curl -H "Authorization: Bearer $TOKEN_USER1" \
  "http://localhost:8085/api/v1/transactions?page=1&limit=10"

# 5. Transaction detail
curl -H "Authorization: Bearer $TOKEN_USER1" \
  http://localhost:8085/api/v1/transactions/$TX_ID

# 6. Test idempotency (kirim transfer sama 2x)
# Request kedua harus return response yang sama tanpa proses ulang

# 7. Test saldo kurang
# User 2 coba transfer lebih dari saldonya → expected error

# 8. Test transfer ke diri sendiri → expected error
```

---

## ✅ Acceptance Criteria

- [ ] Transfer berhasil: saldo sender berkurang, saldo receiver bertambah
- [ ] Ledger entries tercatat (debit + credit)
- [ ] Idempotency key mencegah duplikasi transfer
- [ ] Transfer gagal jika saldo kurang → status "failed"
- [ ] Transfer gagal jika wallet frozen
- [ ] Transfer ke diri sendiri ditolak
- [ ] Transaction history paginated, bisa filter by type
- [ ] Outbox event ter-insert saat transfer completed
- [ ] Outbox worker publish event ke RabbitMQ (jika RabbitMQ sudah running)
- [ ] Unit test untuk transfer flow

---

## 💡 Tips & Common Pitfalls

1. **Consistency is KING** — Transfer melibatkan 2 wallet + ledger + outbox. Semua harus dalam satu DB transaction sebisa mungkin.

2. **Rollback yang benar** — Jika credit receiver gagal, jangan lupa kembalikan saldo sender!

3. **Outbox worker jangan terlalu agresif** — Poll setiap 5 detik sudah cukup. Terlalu sering = waste resource.

4. **Idempotency key dari CLIENT** — Server tidak generate, client harus kirim UUID unik. Ini pattern standard (Stripe, PayPal pakai ini).

5. **At-least-once delivery** — Event mungkin ter-publish lebih dari sekali (jika worker crash setelah publish tapi sebelum mark). Consumer harus handle duplikasi.

---

## 📚 Referensi Belajar

- [Outbox Pattern](https://microservices.io/patterns/data/transactional-outbox.html)
- [Idempotency Key (Stripe)](https://stripe.com/docs/api/idempotent_requests)
- [Saga Pattern](https://microservices.io/patterns/data/saga.html)
