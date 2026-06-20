# Panduan Perbaikan GoWallet Monolith — Phase 1 (Ep 1–8)

Dokumen ini mencakup seluruh bug dan plot hole yang ditemukan setelah review codebase. Diurutkan berdasarkan **dependency** — fix model dulu, lalu repository, service, handler, dan terakhir wiring di `main.go`.

---

## Fix #1 — JSON Struct Tags di `Transaction` Model

**File:** [tx.go](file:///Users/ikhda/Documents/coding/bashocode/gowallet/monolith/internal/transaction/model/tx.go)

**Masalah:** Hampir semua JSON tag di struct `Transaction` menggunakan `=` bukan `:`. Akibatnya Go **tidak mengenali** tag tersebut — field tidak akan muncul di JSON response dan JSON request body tidak akan ter-parse.

**Sebelum:**
```go
type Transaction struct {
    ID               string    `json="id"`
    SenderWalletID   *string   `json="sender_wallet_id"`
    ReceiverWalletID string    `json="receiver_wallet_id"`
    Amount           float64   `json:"amount"`
    Description      string    `json:"description"`
    IdempotencyKey   string    `json:"idempotency_key"`
    Status           string    `json:"status"`
    CreatedAt        time.Time `json:"created_at"`
}
```

**Sesudah:**
```diff
 type Transaction struct {
-    ID               string    `json="id"`
-    SenderWalletID   *string   `json="sender_wallet_id"`
-    ReceiverWalletID string    `json="receiver_wallet_id"`
+    ID               string    `json:"id"`
+    SenderWalletID   *string   `json:"sender_wallet_id"`
+    ReceiverWalletID string    `json:"receiver_wallet_id"`
     Amount           float64   `json:"amount"`
     Description      string    `json:"description"`
     IdempotencyKey   string    `json:"idempotency_key"`
     Status           string    `json:"status"`
     CreatedAt        time.Time `json:"created_at"`
 }
```

> [!CAUTION]
> Tanpa fix ini, API transfer akan mengembalikan JSON dengan field `ID`, `SenderWalletID`, `ReceiverWalletID` bernilai kosong/zero-value meskipun datanya ada.

---

## Fix #2 — JSON & Binding Tags di `TransferRequest` Model

**File:** [tx.go](file:///Users/ikhda/Documents/coding/bashocode/gowallet/monolith/internal/transaction/model/tx.go)

**Masalah:** Ada 3 jenis error di struct ini:
1. JSON tags pakai `=` bukan `:`
2. Binding tag `binding:=` (ada `=` yang tidak seharusnya)
3. Spasi setelah koma di binding values (`"required, email"`) — Gin validator **tidak** men-toleransi spasi

**Sebelum:**
```go
type TransferRequest struct {
    ReceiverEmail  string  `json="receiver_email" binding:"required, email"`
    Amount         float64 `json="amount" binding:="required, gt=0"`
    Description    string  `json="description"`
    IdempotencyKey string  `json="idempotency_key" binding:"required"`
}
```

**Sesudah:**
```diff
 type TransferRequest struct {
-    ReceiverEmail  string  `json="receiver_email" binding:"required, email"`
-    Amount         float64 `json="amount" binding:="required, gt=0"`
-    Description    string  `json="description"`
-    IdempotencyKey string  `json="idempotency_key" binding:"required"`
+    ReceiverEmail  string  `json:"receiver_email" binding:"required,email"`
+    Amount         float64 `json:"amount" binding:"required,gt=0"`
+    Description    string  `json:"description"`
+    IdempotencyKey string  `json:"idempotency_key" binding:"required"`
 }
```

> [!WARNING]
> Tanpa fix ini, `ShouldBindJSON` di handler akan selalu gagal validasi atau tidak mem-parse field dari request body sama sekali.

---

## Fix #3 — Scan Bug di `GetByIdempotencyKey` Repository

**File:** [repository.go](file:///Users/ikhda/Documents/coding/bashocode/gowallet/monolith/internal/transaction/repository/repository.go#L29-L53)

**Masalah:** Variable `sender` (`sql.NullString`) dideklarasikan di line 32, tapi yang di-scan ke database adalah `&t.SenderWalletID` (pointer `*string`) di line 35. Akibatnya:
- `sender` **tidak pernah diisi** oleh scan
- Block `if sender.Valid` di line 48–50 adalah **dead code**
- Jika `sender_wallet_id` di DB bernilai `NULL`, scan ke `*string` akan **panic** atau menghasilkan error

**Sebelum:**
```go
func (r *mysqlTransactionRepository) GetByIdempotencyKey(ctx context.Context, key string) (*model.Transaction, error) {
    query := `SELECT id, sender_wallet_id, receiver_wallet_id, amount, description, idempotency_key, status, created_at FROM transactions WHERE idempotency_key = ?`
    t := &model.Transaction{}
    var sender sql.NullString
    err := r.db.QueryRowContext(ctx, query, key).Scan(
        &t.ID,
        &t.SenderWalletID,   // ← harusnya scan ke &sender
        &t.ReceiverWalletID,
        &t.Amount,
        &t.Description,
        &t.IdempotencyKey,
        &t.Status,
        &t.CreatedAt,
    )

    if err != nil {
        return nil, err
    }

    if sender.Valid {
        t.SenderWalletID = &sender.String
    }

    return t, nil
}
```

**Sesudah:**
```diff
 func (r *mysqlTransactionRepository) GetByIdempotencyKey(ctx context.Context, key string) (*model.Transaction, error) {
     query := `SELECT id, sender_wallet_id, receiver_wallet_id, amount, description, idempotency_key, status, created_at FROM transactions WHERE idempotency_key = ?`
     t := &model.Transaction{}
     var sender sql.NullString
     err := r.db.QueryRowContext(ctx, query, key).Scan(
         &t.ID,
-        &t.SenderWalletID,
+        &sender,
         &t.ReceiverWalletID,
         &t.Amount,
         &t.Description,
         &t.IdempotencyKey,
         &t.Status,
         &t.CreatedAt,
     )
 
     if err != nil {
         return nil, err
     }
 
     if sender.Valid {
         t.SenderWalletID = &sender.String
     }
 
     return t, nil
 }
```

> [!IMPORTANT]
> Fix ini memastikan `sender_wallet_id` yang `NULL` di database (misalnya pada kasus top-up) tidak menyebabkan scan error.

---

## Fix #4 — `CreatedAt` Assignment di Transfer Service

**File:** [service.go](file:///Users/ikhda/Documents/coding/bashocode/gowallet/monolith/internal/transaction/service/service.go#L146)

**Masalah:** Setelah commit, `transaction.CreatedAt` di-set dari `senderWallet.UpdatedAt`. Ini tidak logis karena:
- `UpdatedAt` milik wallet, bukan transaction
- Setelah optimistic lock update, `senderWallet.UpdatedAt` di memori **masih** nilai lama (sebelum update)
- Seharusnya pakai `time.Now()` atau biarkan database mengisi via `DEFAULT CURRENT_TIMESTAMP`

**Sebelum:**
```go
transaction.CreatedAt = senderWallet.UpdatedAt
return transaction, nil
```

**Sesudah:**
```diff
-    transaction.CreatedAt = senderWallet.UpdatedAt
+    transaction.CreatedAt = time.Now()
     return transaction, nil
```

> [!NOTE]
> Ini hanya untuk response ke client. Nilai `created_at` yang sebenarnya tetap diisi oleh database saat `INSERT`.

---

## Fix #5 — [NEW] Buat Transaction Handler

**File:** [handler.go](file:///Users/ikhda/Documents/coding/bashocode/gowallet/monolith/internal/transaction/handler/handler.go) `[NEW]`

**Masalah:** Folder `transaction/handler/` **kosong**. Tidak ada HTTP handler yang mengekspos logika `Transfer` ke dunia luar. Ini adalah **plot hole utama** — business logic sudah lengkap tapi tidak bisa diakses via API.

**Kode baru:**

```go
package handler

import (
    "net/http"

    customErr "github.com/bashocode/gowallet/monolith/internal/errors"
    "github.com/bashocode/gowallet/monolith/internal/transaction/model"
    "github.com/bashocode/gowallet/monolith/internal/transaction/service"
    "github.com/gin-gonic/gin"
)

type TransactionHandler struct {
    svc service.TransactionService
}

func NewTransactionHandler(s service.TransactionService) *TransactionHandler {
    return &TransactionHandler{svc: s}
}

func (h *TransactionHandler) Transfer(c *gin.Context) {
    // Ambil senderUserID dari middleware Auth (JWT claims)
    senderUserID, exists := c.Get("user_id")
    if !exists {
        c.Error(customErr.NewAppError(
            http.StatusUnauthorized, "UNAUTHORIZED", "User context not found",
        ))
        return
    }

    var req model.TransferRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.Error(customErr.NewAppError(
            http.StatusBadRequest, "BAD_REQUEST", err.Error(),
        ))
        return
    }

    tx, err := h.svc.Transfer(c.Request.Context(), senderUserID.(string), req)
    if err != nil {
        c.Error(err)
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Transfer successful",
        "data":    tx,
    })
}
```

> [!NOTE]
> Pattern ini mengikuti konvensi yang sudah ada di `user/handler` dan `wallet/handler` — pakai `c.Error()` untuk error agar ditangani oleh `ErrorHandler` middleware secara konsisten.

---

## Fix #6 — Wiring Transaction di `main.go`

**File:** [main.go](file:///Users/ikhda/Documents/coding/bashocode/gowallet/monolith/cmd/main.go)

**Masalah:** `main.go` belum menginisialisasi transaction repository, service, dan handler. Route `/transactions/transfer` juga belum terdaftar.

**Perubahan:**

### 6a. Tambah import

```diff
 import (
     "github.com/bashocode/gowallet/monolith/internal/config"
     "github.com/bashocode/gowallet/monolith/internal/database"
+    ledgerRepository "github.com/bashocode/gowallet/monolith/internal/ledger/repository"
     "github.com/bashocode/gowallet/monolith/internal/logger"
     "github.com/bashocode/gowallet/monolith/internal/middleware"
+    txHandler "github.com/bashocode/gowallet/monolith/internal/transaction/handler"
+    txRepository "github.com/bashocode/gowallet/monolith/internal/transaction/repository"
+    txService "github.com/bashocode/gowallet/monolith/internal/transaction/service"
     userHandler "github.com/bashocode/gowallet/monolith/internal/user/handler"
     userRepository "github.com/bashocode/gowallet/monolith/internal/user/repository"
     userService "github.com/bashocode/gowallet/monolith/internal/user/service"
     walletHandler "github.com/bashocode/gowallet/monolith/internal/wallet/handler"
     walletRepository "github.com/bashocode/gowallet/monolith/internal/wallet/repository"
     walletService "github.com/bashocode/gowallet/monolith/internal/wallet/service"
     "github.com/gin-gonic/gin"
 )
```

> [!WARNING]
> Perhatikan constructor ledger repository di kode asli adalah `NewMysqlLedgerRepository` (huruf kecil `sql`), **bukan** `NewMySQLLedgerRepository` seperti saran Gemini. Salah tulis akan menyebabkan compile error.

### 6b. Inisialisasi layer transaction

```diff
     // 1. initiate layer
     uRepo := userRepository.NewMySQLUserRepository(db)
     wRepo := walletRepository.NewMySQLWalletRepository(db)
+    tRepo := txRepository.NewMySQLTransactionRepository(db)
+    lRepo := ledgerRepository.NewMysqlLedgerRepository(db)

     // inject db to user service for transaction
     uSvc := userService.NewUserService(db, uRepo, wRepo)
     wSvc := walletService.NewWalletService(wRepo)
+    tSvc := txService.NewTransactionService(db, tRepo, uRepo, wRepo, lRepo)

     uHandler := userHandler.NewUserHandler(uSvc)
     wHandler := walletHandler.NewWalletHandler(wSvc)
+    tHandler := txHandler.NewTransactionHandler(tSvc)
```

### 6c. Daftarkan route transfer

```diff
     // Protected routes (requires valid JWT token)
     protected := v1.Group("")
     protected.Use(middleware.AuthMiddleware())
     {
         protected.GET("/users/me", uHandler.GetProfileMe)
         protected.GET("/wallets/me", wHandler.GetMyWallet)
+        protected.POST("/transactions/transfer", tHandler.Transfer)
     }
```

---

## Urutan Pengerjaan

| Urutan | Fix | File | Dampak |
|:---:|:---:|---|---|
| 1 | Fix #1 & #2 | `transaction/model/tx.go` | JSON serialization & request binding |
| 2 | Fix #3 | `transaction/repository/repository.go` | NULL handling saat scan |
| 3 | Fix #4 | `transaction/service/service.go` | Correct `CreatedAt` value |
| 4 | Fix #5 | `transaction/handler/handler.go` **[NEW]** | HTTP endpoint handler |
| 5 | Fix #6 | `cmd/main.go` | Wiring & route registration |

---

## Verifikasi

Setelah semua fix diterapkan, jalankan:

```bash
# 1. Pastikan compile berhasil
cd monolith && go build ./...

# 2. Jalankan database
docker-compose up -d

# 3. Jalankan server
go run cmd/main.go

# 4. Register 2 user
curl -X POST http://localhost:8080/api/v1/users/register \
  -H "Content-Type: application/json" \
  -d '{"full_name":"Sender","email":"sender@test.com","password":"password123"}'

curl -X POST http://localhost:8080/api/v1/users/register \
  -H "Content-Type: application/json" \
  -d '{"full_name":"Receiver","email":"receiver@test.com","password":"password123"}'

# 5. Login sebagai sender
curl -X POST http://localhost:8080/api/v1/users/login \
  -H "Content-Type: application/json" \
  -d '{"email":"sender@test.com","password":"password123"}'
# → Copy token dari response

# 6. Test transfer
curl -X POST http://localhost:8080/api/v1/transactions/transfer \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <TOKEN_SENDER>" \
  -d '{
    "receiver_email": "receiver@test.com",
    "amount": 50000.00,
    "description": "Test transfer",
    "idempotency_key": "test-key-001"
  }'
```

> [!IMPORTANT]
> Pastikan kedua user sudah punya wallet dengan balance yang cukup. Wallet dibuat otomatis saat registrasi (lihat `userService.Register`), tapi balance default kemungkinan `0`. Kamu mungkin perlu seed data atau buat endpoint top-up terlebih dahulu.
