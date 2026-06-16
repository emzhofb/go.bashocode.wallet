# Episode 28: Outbox Pattern untuk Transaksi Terdistribusi

## 🎯 Tujuan
* Memahami tantangan **Transaksi Terdistribusi** (Distributed Transactions) dalam arsitektur microservices.
* Mengenalkan pola **Transactional Outbox Pattern** untuk memastikan data konsisten tanpa kehilangan pesan (*guaranteed message delivery*).
* Membuat migrasi tabel `outbox_events` di database **Transaction Service**.
* Mengubah logika `Transfer` agar menulis data event ke tabel outbox di dalam rangkaian transaksi database local yang sama.

---

## 📐 Kenapa Butuh Outbox Pattern?
Ketika Transaction Service memproses transfer dan statusnya diperbarui menjadi `SUCCESS`, kita ingin mem-publish event `transfer.completed` ke RabbitMQ agar Notification Service dapat mengirim email secara asinkron.
* **Tantangan:** Bagaimana jika publish ke RabbitMQ gagal di tengah jalan setelah database lokal meng-commit status transaksi menjadi SUCCESS? Uang terpotong, tapi email tidak terkirim selamanya.
* **Solusi (Outbox Pattern):** Di dalam transaksi SQL database lokal Transaction Service yang menyimpan status transaksi, kita **juga menyimpan baris event baru** ke tabel lokal `outbox_events`. Karena satu koneksi DB dan satu transaksi lokal, penyimpanan status transaksi dan data outbox event dijamin 100% konsisten secara atomik (jika salah satu gagal, keduanya akan rollback).

```
[ SQL Local Transaction Begin ]
 ├── 1. Simpan/Update status transaksi di tabel 'transactions'
 └── 2. Simpan data event baru ke tabel 'outbox_events'
[ SQL Local Transaction Commit ] (100% Atomik)
```

---

## 📦 Langkah-langkah

### Step 1: Membuat Skema Migrasi Tabel Outbox
Buat file migrasi baru `db/migrations/000002_create_outbox_table.up.sql` di `transaction-service/`:

```sql
CREATE TABLE outbox_events (
    id VARCHAR(36) PRIMARY KEY,
    event_type VARCHAR(100) NOT NULL,
    payload JSON NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending, processed, failed
    attempts INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);
```

Jalankan perintah migrasi di folder `transaction-service/`:
```bash
migrate -path db/migrations -database "mysql://gowallet_user:gowallet_password@tcp(localhost:3306)/gowallet_transactions" up
```

### Step 2: Membuat Model Outbox Event (`internal/transaction/model/outbox.go`)
Buat file baru di `transaction-service/internal/transaction/model/outbox.go`:

```go
package model

import "time"

type OutboxEvent struct {
	ID        string    `json:"id"`
	EventType string    `json:"event_type"`
	Payload   string    `json:"payload"` // JSON string
	Status    string    `json:"status"`  // pending, processed, failed
	Attempts  int       `json:"attempts"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
```

### Step 3: Membuat Fungsi Menyimpan Outbox di Repository
Buka `transaction-service/internal/transaction/repository/repository.go`. Tambahkan method `CreateOutboxTx` ke interface dan implementasinya:

```go
// Tambah di interface TransactionRepository:
// CreateOutboxTx(ctx context.Context, tx *sql.Tx, event *model.OutboxEvent) error

func (r *mysqlTransactionRepository) CreateOutboxTx(ctx context.Context, tx *sql.Tx, event *model.OutboxEvent) error {
	query := `INSERT INTO outbox_events (id, event_type, payload, status) VALUES (?, ?, ?, ?)`
	_, err := tx.ExecContext(ctx, query, event.ID, event.EventType, event.Payload, event.Status)
	return err
}
```

### Step 4: Integrasi Outbox ke Method `Transfer` di TransactionService
Buka `transaction-service/internal/transaction/service/service.go`. Ubah method `Transfer` agar membungkus penyimpanan status transaksi dan outbox event dalam satu database transaction lokal:

```go
func (s *transactionService) Transfer(ctx context.Context, senderUserID string, req model.TransferRequest) (*model.Transaction, error) {
	// ... (langkah 1 s.d. 3 sama: cek idempotency, cek user via gRPC, cek wallet via gRPC) ...

	// 4. Mulai Transaksi SQL lokal di Transaction Service DB
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, customErr.ErrInternalServer
	}
	defer tx.Rollback()

	// 5. Buat record transaksi di tabel 'transactions' dengan status PENDING
	txID := uuid.New().String()
	txRecord := &model.Transaction{
		ID:               txID,
		SenderWalletID:   &senderWallet.Id,
		ReceiverWalletID: receiverUser.GetId(),
		Amount:           req.Amount,
		Description:      req.Description,
		IdempotencyKey:   req.IdempotencyKey,
		Status:           "PENDING",
	}
	if err := s.txRepo.CreateTx(ctx, tx, txRecord); err != nil {
		return nil, customErr.ErrInternalServer
	}

	// 6. Hubungi Wallet Service & Ledger Service via gRPC untuk mutasi saldo ...
	// (Jika panggilan gRPC sukses:)
	txRecord.Status = "SUCCESS"
	if err := s.txRepo.UpdateStatusTx(ctx, tx, txID, "SUCCESS"); err != nil {
		return nil, customErr.ErrInternalServer
	}

	// 7. Rangkai Payload Event untuk Outbox
	eventPayload := fmt.Sprintf(`{
		"transaction_id": "%s",
		"sender_user_id": "%s",
		"receiver_user_id": "%s",
		"amount": %.2f,
		"description": "%s"
	}`, txID, senderUserID, receiverUser.GetId(), req.Amount, req.Description)

	outboxEvent := &model.OutboxEvent{
		ID:        uuid.New().String(),
		EventType: "transfer.completed",
		Payload:   eventPayload,
		Status:    "pending",
	}

	// 8. Simpan event ke tabel outbox dalam transaksi lokal yang sama
	if err := s.txRepo.CreateOutboxTx(ctx, tx, outboxEvent); err != nil {
		return nil, customErr.ErrInternalServer
	}

	// 9. Commit transaksi lokal
	if err := tx.Commit(); err != nil {
		return nil, customErr.ErrInternalServer
	}

	return txRecord, nil
}
```

---

## ✅ Acceptance Criteria
* [ ] Tabel `outbox_events` terbuat dengan sukses di database `gowallet_transactions` milik Transaction Service.
* [ ] Setiap transaksi sukses otomatis menghasilkan satu record event outbox dengan status `"pending"` dan `event_type` bernilai `"transfer.completed"`.
* [ ] Jika proses transfer gagal di pertengahan jalan (misal karena limit saldo/gRPC error), transaksi lokal di-rollback dan tidak ada data outbox palsu yang tertulis.

---

## 💡 Tips untuk Junior
* **Atomicity:** Dengan menyimpan outbox event dalam satu transaksi lokal yang sama dengan data bisnis utama (status transaksi), kita menjamin konsistensi mutlak tanpa dipengaruhi oleh kegagalan jaringan eksternal saat proses transfer berlangsung.
