# Episode 8: Ledger Service ⭐

## 🎯 Tujuan
- Mencatat semua pergerakan uang (immutable entries)
- Double-entry bookkeeping sederhana
- Riwayat mutasi per wallet
- Rekonsiliasi saldo (wallet balance vs ledger sum)
- gRPC API untuk dipanggil service lain

## 📝 Prerequisites
- Episode 7 selesai (Wallet Service)

---

## 📦 Langkah-langkah

### Step 1: Inisialisasi Service

```bash
mkdir -p ledger-service/cmd
mkdir -p ledger-service/internal/{handler,service,repository,model,dto,config,grpc}
mkdir -p ledger-service/db/migrations

cd ledger-service
go mod init github.com/emzhofb/gowallet/ledger-service
cd ..
go work use ./ledger-service
```

### Step 2: Konsep Ledger — Kenapa Penting?

```
Tanpa Ledger:
  Wallet saldo = 100.000
  Bagaimana bisa 100.000? Dari mana? Kapan? → TIDAK TAHU!

Dengan Ledger:
  Entry 1: Credit +200.000 (Top Up, 1 Jan)
  Entry 2: Debit  -50.000  (Transfer ke Bob, 2 Jan)
  Entry 3: Debit  -50.000  (Withdraw, 3 Jan)
  ────────────────────────────────────────
  Total:   100.000  ← Cocok dengan saldo wallet!

Ledger = Buku Kas. Setiap rupiah yang masuk/keluar tercatat.
```

> **Aturan emas:** Ledger entries TIDAK BOLEH di-update atau di-delete. Pernah salah? Buat entry baru untuk reversal. Ini prinsip **immutability**.

### Step 3: Database Migration

```bash
migrate create -ext sql -dir ledger-service/db/migrations -seq create_ledger_entries
```

**`000001_create_ledger_entries.up.sql`:**
```sql
CREATE TABLE IF NOT EXISTS ledger_entries (
    id CHAR(36) PRIMARY KEY,
    transaction_id CHAR(36) NOT NULL,
    wallet_id CHAR(36) NOT NULL,
    type ENUM('debit', 'credit') NOT NULL,
    amount DECIMAL(15,2) NOT NULL,
    balance_after DECIMAL(15,2) NOT NULL,
    description VARCHAR(500) NOT NULL DEFAULT '',
    reference_type VARCHAR(50) NOT NULL DEFAULT '',
    reference_id VARCHAR(36) NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    INDEX idx_wallet_id (wallet_id),
    INDEX idx_transaction_id (transaction_id),
    INDEX idx_created_at (created_at),
    INDEX idx_wallet_created (wallet_id, created_at)
    
    -- TIDAK ADA updated_at dan deleted_at → IMMUTABLE!
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

**Field Penjelasan:**
| Field | Keterangan |
|---|---|
| `transaction_id` | ID dari transaksi yang memicu entry ini |
| `wallet_id` | Wallet yang terpengaruh |
| `type` | `debit` = uang keluar, `credit` = uang masuk |
| `amount` | Jumlah uang (**selalu positif!**) |
| `balance_after` | Saldo wallet SETELAH entry ini |
| `description` | Keterangan (e.g., "Top up", "Transfer to John") |
| `reference_type` | Tipe referensi (e.g., "topup", "transfer", "withdraw") |
| `reference_id` | ID referensi (e.g., payment ID, transfer ID) |

### Step 4: Proto Definition

Buat `proto/ledger/ledger.proto`:

```protobuf
syntax = "proto3";
package ledger;
option go_package = "github.com/emzhofb/gowallet/proto/ledger";

service LedgerService {
    // Buat entry baru (dipanggil oleh Wallet/Transaction Service)
    rpc CreateEntry(CreateEntryRequest) returns (LedgerEntryResponse);
    
    // Buat 2 entries sekaligus (untuk transfer: debit sender + credit receiver)
    rpc CreateTransferEntries(CreateTransferRequest) returns (TransferEntriesResponse);
    
    // Riwayat mutasi per wallet
    rpc GetEntriesByWallet(GetEntriesRequest) returns (GetEntriesResponse);
    
    // Hitung saldo dari ledger
    rpc GetBalanceByWallet(GetBalanceRequest) returns (BalanceResponse);
    
    // Rekonsiliasi: bandingkan saldo wallet vs ledger
    rpc Reconcile(ReconcileRequest) returns (ReconcileResponse);
}

message CreateEntryRequest {
    string transaction_id = 1;
    string wallet_id = 2;
    string type = 3;        // "debit" atau "credit"
    double amount = 4;
    string description = 5;
    string reference_type = 6;
    string reference_id = 7;
}

message CreateTransferRequest {
    string transaction_id = 1;
    string sender_wallet_id = 2;
    string receiver_wallet_id = 3;
    double amount = 4;
    string description = 5;
}

message GetEntriesRequest {
    string wallet_id = 1;
    int32 page = 2;
    int32 limit = 3;
    string start_date = 4;  // optional: filter by date range
    string end_date = 5;
}

message GetBalanceRequest {
    string wallet_id = 1;
}

message ReconcileRequest {
    string wallet_id = 1;
    double expected_balance = 2;  // Saldo dari wallet table
}

message LedgerEntryResponse {
    string id = 1;
    string transaction_id = 2;
    string wallet_id = 3;
    string type = 4;
    double amount = 5;
    double balance_after = 6;
    string description = 7;
    string created_at = 8;
}

message GetEntriesResponse {
    repeated LedgerEntryResponse entries = 1;
    int32 total = 2;
    int32 page = 3;
    int32 limit = 4;
}

message BalanceResponse {
    string wallet_id = 1;
    double total_credit = 2;
    double total_debit = 3;
    double calculated_balance = 4;  // credit - debit
}

message ReconcileResponse {
    bool is_consistent = 1;
    double wallet_balance = 2;
    double ledger_balance = 3;
    double difference = 4;
}

message TransferEntriesResponse {
    LedgerEntryResponse sender_entry = 1;
    LedgerEntryResponse receiver_entry = 2;
}
```

Generate:
```bash
protoc --go_out=. --go-grpc_out=. proto/ledger/ledger.proto
```

### Step 5: Service Implementation

**Create Entry:**
```
1. Hitung balance_after:
   - Query saldo terakhir dari ledger: 
     SELECT balance_after FROM ledger_entries 
     WHERE wallet_id = ? ORDER BY created_at DESC LIMIT 1
   - Jika belum ada entry → balance_after = 0
   - Jika type = 'credit': balance_after = last_balance + amount
   - Jika type = 'debit': balance_after = last_balance - amount
   - VALIDASI: balance_after tidak boleh negatif!
2. Insert entry ke database
3. Return entry
```

**Create Transfer Entries:**
```
1. Dalam satu database transaction:
   a. Create debit entry untuk sender
   b. Create credit entry untuk receiver
   c. Jika ada error → ROLLBACK keduanya
2. Return kedua entries
```

**Get Balance from Ledger:**
```sql
SELECT 
    COALESCE(SUM(CASE WHEN type = 'credit' THEN amount ELSE 0 END), 0) as total_credit,
    COALESCE(SUM(CASE WHEN type = 'debit' THEN amount ELSE 0 END), 0) as total_debit,
    COALESCE(SUM(CASE WHEN type = 'credit' THEN amount ELSE -amount END), 0) as calculated_balance
FROM ledger_entries
WHERE wallet_id = ?;
```

**Reconcile:**
```
1. Hitung saldo dari ledger (query di atas)
2. Bandingkan dengan expected_balance (dari wallet table)
3. Jika sama → consistent ✅
4. Jika berbeda → inconsistent ❌ + log warning!
```

### Step 6: Integrasi dengan Wallet Service

Setelah Ledger Service siap, update Wallet Service:

```go
// wallet-service/internal/service/wallet_service.go

func (s *walletService) TopUp(ctx context.Context, userID string, req dto.TopUpRequest) error {
    // ... idempotency check ...
    
    wallet, _ := s.walletRepo.GetByUserID(ctx, userID)
    
    // 1. Update wallet balance (optimistic locking)
    err := s.walletRepo.UpdateBalance(ctx, wallet.ID, req.Amount, wallet.Version)
    
    // 2. Create ledger entry via gRPC
    _, err = s.ledgerClient.CreateEntry(ctx, &ledgerpb.CreateEntryRequest{
        TransactionId: uuid.New().String(),
        WalletId:      wallet.ID,
        Type:          "credit",
        Amount:        req.Amount,
        Description:   "Top up",
        ReferenceType: "topup",
        ReferenceId:   req.IdempotencyKey,
    })
    
    // 3. Invalidate cache
    s.redis.Del(ctx, cacheKey)
    
    return nil
}
```

### Step 7: REST Endpoints (Optional — bisa hanya gRPC)

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/ledger/entries?wallet_id=xxx` | ✅ | Riwayat mutasi (user hanya bisa lihat miliknya) |
| GET | `/api/v1/ledger/balance?wallet_id=xxx` | ✅ | Saldo dari ledger |
| GET | `/api/v1/ledger/reconcile?wallet_id=xxx` | ✅ Admin | Rekonsiliasi |

### Step 8: Test

```bash
# 1. Top up via Wallet Service (akan create ledger entry otomatis)
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  http://localhost:8083/api/v1/wallets/topup \
  -d '{"amount": 100000, "idempotency_key": "'$(uuidgen)'"}'

# 2. Cek ledger entries
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8084/api/v1/ledger/entries?wallet_id=xxx"

# 3. Cek saldo dari ledger
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8084/api/v1/ledger/balance?wallet_id=xxx"

# 4. Reconcile (admin)
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
  "http://localhost:8084/api/v1/ledger/reconcile?wallet_id=xxx"

# 5. Test gRPC
grpcurl -plaintext -d '{"wallet_id":"xxx"}' \
  localhost:9084 ledger.LedgerService/GetBalanceByWallet
```

---

## ✅ Acceptance Criteria

- [ ] Setiap top up/withdraw membuat ledger entry
- [ ] Ledger entries immutable (tidak ada UPDATE/DELETE)
- [ ] `balance_after` dihitung dengan benar
- [ ] Riwayat mutasi bisa dilihat per wallet (paginated)
- [ ] Saldo dari ledger = sum(credit) - sum(debit)
- [ ] Rekonsiliasi: saldo wallet == saldo ledger
- [ ] Transfer membuat 2 entries (debit + credit)
- [ ] gRPC API berfungsi
- [ ] Unit test untuk kalkulasi saldo

---

## 💡 Tips & Common Pitfalls

1. **IMMUTABLE = tidak ada UPDATE/DELETE** — Jika salah catat, buat entry reversal (e.g., credit untuk membatalkan debit).

2. **balance_after harus sequential** — Pastikan tidak ada race condition saat menghitung balance_after. Gunakan database transaction + row-level locking.

3. **Decimal precision** — Gunakan `DECIMAL(15,2)` bukan `FLOAT`. Floating point error bisa menyebabkan saldo tidak balance.

4. **Ledger ini source of truth** — Saldo di wallet table hanya untuk performance/cache. Jika ada perbedaan, percayai ledger.

---

## 📚 Referensi Belajar

- [Double-Entry Bookkeeping](https://en.wikipedia.org/wiki/Double-entry_bookkeeping)
- [Building a Ledger System](https://www.moderntreasury.com/journal/how-to-build-a-ledger)
- [gRPC Go Tutorial](https://grpc.io/docs/languages/go/basics/)
