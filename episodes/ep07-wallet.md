# Episode 7: Wallet Service

## 🎯 Tujuan
- Auto-create wallet saat user register
- Cek saldo, top up, withdraw
- Freeze/unfreeze wallet (admin)
- Optimistic locking untuk concurrent updates
- Redis cache untuk saldo
- Idempotency key

## 📝 Prerequisites
- Episode 6 selesai (User Service + gRPC)

---

## 📦 Langkah-langkah

### Step 1: Inisialisasi Service

```bash
mkdir -p wallet-service/cmd
mkdir -p wallet-service/internal/{handler,service,repository,model,dto,config,grpc}
mkdir -p wallet-service/db/migrations

cd wallet-service
go mod init github.com/emzhofb/gowallet/wallet-service
cd ..
go work use ./wallet-service

# Install dependencies
cd wallet-service
go get github.com/gin-gonic/gin
go get github.com/go-sql-driver/mysql
go get github.com/google/uuid
go get github.com/redis/go-redis/v9
go get go.uber.org/zap
go get google.golang.org/grpc
cd ..
```

### Step 2: Database Migration

```bash
migrate create -ext sql -dir wallet-service/db/migrations -seq create_wallets
```

**`000001_create_wallets.up.sql`:**
```sql
CREATE TABLE IF NOT EXISTS wallets (
    id CHAR(36) PRIMARY KEY,
    user_id CHAR(36) UNIQUE NOT NULL,
    balance DECIMAL(15,2) NOT NULL DEFAULT 0.00,
    currency VARCHAR(3) NOT NULL DEFAULT 'IDR',
    status ENUM('active', 'frozen') NOT NULL DEFAULT 'active',
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    INDEX idx_user_id (user_id),
    INDEX idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Idempotency keys table
CREATE TABLE IF NOT EXISTS idempotency_keys (
    idempotency_key VARCHAR(255) PRIMARY KEY,
    wallet_id CHAR(36) NOT NULL,
    operation VARCHAR(50) NOT NULL,
    status ENUM('processing', 'completed', 'failed') NOT NULL DEFAULT 'processing',
    response_code INT,
    response_body JSON,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL,
    
    INDEX idx_wallet_id (wallet_id),
    INDEX idx_expires_at (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

**`000001_create_wallets.down.sql`:**
```sql
DROP TABLE IF EXISTS idempotency_keys;
DROP TABLE IF EXISTS wallets;
```

### Step 3: Model

```go
type Wallet struct {
    ID        string          `json:"id"`
    UserID    string          `json:"user_id"`
    Balance   decimal.Decimal `json:"balance"`     // Gunakan github.com/shopspring/decimal
    Currency  string          `json:"currency"`
    Status    string          `json:"status"`      // "active" atau "frozen"
    Version   int             `json:"version"`     // Untuk optimistic locking
    CreatedAt time.Time       `json:"created_at"`
    UpdatedAt time.Time       `json:"updated_at"`
}

type IdempotencyKey struct {
    Key          string `json:"key"`
    WalletID     string `json:"wallet_id"`
    Operation    string `json:"operation"`
    Status       string `json:"status"`
    ResponseCode *int   `json:"response_code"`
    ResponseBody []byte `json:"response_body"`
    ExpiresAt    time.Time
}
```

> **Catatan tentang decimal:** Untuk uang, **JANGAN gunakan `float64`!** Floating point tidak akurat untuk kalkulasi keuangan. Gunakan `DECIMAL(15,2)` di MySQL dan `github.com/shopspring/decimal` di Go. Alternatif: simpan dalam integer (satuan terkecil, misalnya Rupiah tanpa desimal).

### Step 4: Endpoints

| Method | Path | Auth | RBAC | Description |
|---|---|---|---|---|
| GET | `/api/v1/wallets/me` | ✅ | Any | Cek saldo |
| POST | `/api/v1/wallets/topup` | ✅ | Any | Top up saldo |
| POST | `/api/v1/wallets/withdraw` | ✅ | Any | Withdraw saldo |
| PUT | `/api/v1/wallets/:id/freeze` | ✅ | Admin | Freeze wallet |
| PUT | `/api/v1/wallets/:id/unfreeze` | ✅ | Admin | Unfreeze wallet |

### Step 5: DTOs

```go
type TopUpRequest struct {
    Amount         float64 `json:"amount" binding:"required,gt=0"`
    IdempotencyKey string  `json:"idempotency_key" binding:"required,uuid"`
}

type WithdrawRequest struct {
    Amount         float64 `json:"amount" binding:"required,gt=0"`
    IdempotencyKey string  `json:"idempotency_key" binding:"required,uuid"`
}

type WalletResponse struct {
    ID        string  `json:"id"`
    UserID    string  `json:"user_id"`
    Balance   float64 `json:"balance"`
    Currency  string  `json:"currency"`
    Status    string  `json:"status"`
    CreatedAt string  `json:"created_at"`
}
```

### Step 6: Optimistic Locking

```go
func (r *walletRepo) UpdateBalance(ctx context.Context, walletID string, amount float64, version int) error {
    query := `UPDATE wallets 
              SET balance = balance + ?, version = version + 1, updated_at = NOW()
              WHERE id = ? AND version = ? AND status = 'active'`
    
    result, err := r.db.ExecContext(ctx, query, amount, walletID, version)
    if err != nil {
        return err
    }
    
    rowsAffected, _ := result.RowsAffected()
    if rowsAffected == 0 {
        // Version mismatch → concurrent update detected!
        return ErrOptimisticLock
    }
    
    return nil
}

// Di service layer, retry jika optimistic lock conflict:
func (s *walletService) TopUp(ctx context.Context, userID string, req dto.TopUpRequest) error {
    maxRetries := 3
    for i := 0; i < maxRetries; i++ {
        wallet, _ := s.walletRepo.GetByUserID(ctx, userID)
        
        err := s.walletRepo.UpdateBalance(ctx, wallet.ID, req.Amount, wallet.Version)
        if err == ErrOptimisticLock {
            // Retry — ambil data terbaru dan coba lagi
            continue
        }
        return err
    }
    return ErrTooManyRetries
}
```

### Step 7: Idempotency Key

```go
func (s *walletService) TopUp(ctx context.Context, userID string, req dto.TopUpRequest) (*dto.WalletResponse, error) {
    // 1. Cek idempotency key
    existing, _ := s.idempotencyRepo.GetByKey(ctx, req.IdempotencyKey)
    if existing != nil {
        if existing.Status == "completed" {
            // Sudah pernah diproses → return response sebelumnya
            return parseResponse(existing.ResponseBody), nil
        }
        if existing.Status == "processing" {
            return nil, ErrRequestInProgress
        }
    }
    
    // 2. Create idempotency key (status: processing)
    s.idempotencyRepo.Create(ctx, &model.IdempotencyKey{
        Key:       req.IdempotencyKey,
        WalletID:  wallet.ID,
        Operation: "topup",
        Status:    "processing",
        ExpiresAt: time.Now().Add(24 * time.Hour),
    })
    
    // 3. Process top up...
    
    // 4. Update idempotency key (status: completed, response_body: ...)
    s.idempotencyRepo.Complete(ctx, req.IdempotencyKey, responseCode, responseBody)
    
    return response, nil
}
```

> **Kenapa idempotency key?** Client mungkin retry request (karena timeout/network error). Tanpa idempotency key, saldo bisa bertambah 2x. Dengan idempotency key, request kedua return response yang sama tanpa proses ulang.

### Step 8: Redis Cache

```go
func (s *walletService) GetBalance(ctx context.Context, userID string) (*dto.WalletResponse, error) {
    // 1. Cek Redis cache
    cacheKey := fmt.Sprintf("wallet:%s:balance", userID)
    cached, err := s.redis.Get(ctx, cacheKey).Result()
    if err == nil {
        // Cache hit → parse dan return
        var wallet dto.WalletResponse
        json.Unmarshal([]byte(cached), &wallet)
        return &wallet, nil
    }
    
    // 2. Cache miss → query database
    wallet, err := s.walletRepo.GetByUserID(ctx, userID)
    if err != nil {
        return nil, err
    }
    
    // 3. Set cache (TTL 5 menit)
    data, _ := json.Marshal(wallet)
    s.redis.Set(ctx, cacheKey, data, 5*time.Minute)
    
    return toResponse(wallet), nil
}

// Invalidate cache setelah mutasi:
func (s *walletService) TopUp(ctx context.Context, ...) {
    // ... proses top up ...
    
    // Invalidate cache
    cacheKey := fmt.Sprintf("wallet:%s:balance", userID)
    s.redis.Del(ctx, cacheKey)
}
```

### Step 9: Auto-Create Wallet

Ada 2 pendekatan. Pilih salah satu:

**Pendekatan A: Event-driven (Recommended)**
```
Auth Service → publish event "user.registered" ke RabbitMQ
Wallet Service → consume event → create wallet

Tapi RabbitMQ event-driven baru di Episode 11.
Jadi untuk sekarang, pakai Pendekatan B dulu.
```

**Pendekatan B: API call dari Auth Service (Sementara)**
```
Auth Service register success → panggil Wallet Service API
POST http://wallet-service:8083/internal/wallets/create
Body: { "user_id": "uuid" }

Endpoint /internal/* hanya bisa diakses dari internal network (bukan dari Gateway).
```

Di Episode 11 nanti kita ubah ke event-driven.

### Step 10: Freeze/Unfreeze

```go
// Admin only
func (s *walletService) FreezeWallet(ctx context.Context, walletID string) error {
    return s.walletRepo.UpdateStatus(ctx, walletID, "frozen")
}

func (s *walletService) UnfreezeWallet(ctx context.Context, walletID string) error {
    return s.walletRepo.UpdateStatus(ctx, walletID, "active")
}

// Semua operasi (top up, withdraw, transfer) harus cek status:
if wallet.Status == "frozen" {
    return ErrWalletFrozen
}
```

### Step 11: Test Manual

```bash
# 1. Cek saldo (seharusnya 0)
curl -H "Authorization: Bearer $TOKEN" http://localhost:8083/api/v1/wallets/me

# 2. Top up
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  http://localhost:8083/api/v1/wallets/topup \
  -d '{"amount": 100000, "idempotency_key": "'$(uuidgen)'"}'

# 3. Cek saldo (seharusnya 100000)
curl -H "Authorization: Bearer $TOKEN" http://localhost:8083/api/v1/wallets/me

# 4. Top up lagi dengan idempotency key YANG SAMA
# Seharusnya return response yang sama tanpa menambah saldo

# 5. Withdraw
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  http://localhost:8083/api/v1/wallets/withdraw \
  -d '{"amount": 30000, "idempotency_key": "'$(uuidgen)'"}'

# 6. Cek saldo (seharusnya 70000)

# 7. Withdraw lebih dari saldo
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  http://localhost:8083/api/v1/wallets/withdraw \
  -d '{"amount": 999999, "idempotency_key": "'$(uuidgen)'"}'
# Expected: 400 "Insufficient balance"

# 8. Freeze wallet (admin)
curl -X PUT -H "Authorization: Bearer $ADMIN_TOKEN" \
  http://localhost:8083/api/v1/wallets/{wallet-id}/freeze

# 9. Coba top up ke frozen wallet → expected error
```

---

## ✅ Acceptance Criteria

- [ ] Wallet dibuat otomatis saat user register
- [ ] `GET /wallets/me` return saldo yang benar
- [ ] Top up menambah saldo
- [ ] Withdraw mengurangi saldo
- [ ] Withdraw gagal jika saldo kurang → 400 "Insufficient balance"
- [ ] Idempotency key: request dengan key yang sama return response sama
- [ ] Optimistic locking: concurrent update di-retry
- [ ] Redis cache: saldo di-cache, invalidate saat mutasi
- [ ] Freeze: wallet frozen tidak bisa top up/withdraw
- [ ] Admin only bisa freeze/unfreeze
- [ ] Unit test untuk service layer (terutama optimistic locking)

---

## 💡 Tips & Common Pitfalls

1. **JANGAN pakai float64 untuk uang!** Gunakan `DECIMAL(15,2)` di MySQL dan `github.com/shopspring/decimal` di Go. Atau simpan sebagai integer cents.

2. **Idempotency key dari CLIENT** — Server tidak generate key. Client harus kirim UUID unik per request.

3. **Optimistic lock retry** — Max 3 retry biasanya cukup. Jika masih conflict, mungkin ada high contention.

4. **Cache invalidation** — "There are only two hard things in CS: cache invalidation and naming things." Pastikan SELALU invalidate cache saat data berubah.

5. **Race condition testing** — Test concurrent top up dengan `go test -race ./...`

---

## 📚 Referensi Belajar

- [Optimistic Locking Pattern](https://www.baeldung.com/cs/optimistic-vs-pessimistic-locking)
- [Idempotency Keys](https://stripe.com/docs/api/idempotent_requests)
- [Redis Go Client](https://redis.io/docs/connect/clients/go/)
- [shopspring/decimal](https://github.com/shopspring/decimal)
