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
Buka `transaction-service/internal/transaction/service/service.go`. Ubah method `Transfer` agar mencatat transaksi awal, melakukan panggilan gRPC di luar transaksi database, dan baru merekam status akhir beserta outbox event dalam satu transaksi lokal yang cepat:

```go
func (s *transactionService) Transfer(ctx context.Context, senderUserID string, req model.TransferRequest) (*model.Transaction, error) {
	// 1. Cek Idempotency Key (keamanan transaksi ganda)
	existing, _ := s.txRepo.GetByIdempotencyKey(ctx, req.IdempotencyKey)
	if existing != nil {
		return existing, nil
	}

	// 2. Cari & Validasi User Penerima via User Service gRPC
	receiverUser, err := s.userClient.GetUserByEmail(ctx, &userPb.GetUserByEmailRequest{Email: req.ReceiverEmail})
	if err != nil {
		return nil, customErr.NewAppError(http.StatusNotFound, "RECEIVER_NOT_FOUND", "Penerima tidak ditemukan.")
	}

	// 3. Ambil Detail Dompet Pengirim via Wallet Service gRPC
	senderWallet, err := s.walletClient.GetWalletByUserID(ctx, &walletPb.GetWalletRequest{UserId: senderUserID})
	if err != nil {
		return nil, customErr.NewAppError(http.StatusNotFound, "SENDER_WALLET_NOT_FOUND", "Dompet pengirim tidak ditemukan.")
	}
	if senderWallet.GetBalance() < req.Amount {
		return nil, customErr.NewAppError(http.StatusBadRequest, "INSUFFICIENT_BALANCE", "Saldo tidak mencukupi.")
	}

	// 4. Catat record transaksi PENDING ke database.
	// Kita lakukan ini dalam transaksi pendek tersendiri untuk segera melepaskan lock database.
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
	
	initTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, customErr.ErrInternalServer
	}
	defer initTx.Rollback()

	if err := s.txRepo.CreateTx(ctx, initTx, txRecord); err != nil {
		return nil, customErr.ErrInternalServer
	}
	if err := initTx.Commit(); err != nil {
		return nil, customErr.ErrInternalServer
	}

	// 5. Hubungi Wallet Service & Ledger Service via gRPC untuk mutasi saldo (DI LUAR TRANSAKSI DATABASE LOKAL)
	// Kita menerapkan Saga Orchestration dengan orkestrasi rollback manual jika ada step yang gagal.
	err = s.executeGrpcTransferChain(ctx, txID, senderUserID, receiverUser.GetId(), req.Amount, senderWallet, receiverWallet)
	if err != nil {
		// Jika gagal, perbarui status menjadi FAILED
		s.txRepo.UpdateStatus(ctx, txID, "FAILED")
		return nil, err
	}

	// 6. Jika rantai gRPC sukses: Mulai Transaksi SQL lokal baru yang super cepat
	// untuk memperbarui status transaksi menjadi SUCCESS dan menyisipkan event outbox.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, customErr.ErrInternalServer
	}
	defer tx.Rollback()

	txRecord.Status = "SUCCESS"
	if err := s.txRepo.UpdateStatusTx(ctx, tx, txID, "SUCCESS"); err != nil {
		return nil, customErr.ErrInternalServer
	}

	// Rangkai Payload Event untuk Outbox
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

	// Simpan event ke tabel outbox dalam transaksi lokal yang sama
	if err := s.txRepo.CreateOutboxTx(ctx, tx, outboxEvent); err != nil {
		return nil, customErr.ErrInternalServer
	}

	// Commit transaksi lokal (Lock dilepas dalam hitungan milidetik!)
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
* [ ] Panggilan gRPC eksternal dilakukan sepenuhnya di luar konteks transaksi database (`sql.Tx`).
* [ ] Jika proses transfer gagal di pertengahan jalan (misal karena limit saldo/gRPC error), transaksi lokal di-rollback dan tidak ada data outbox palsu yang tertulis.

---

## 💡 Tips untuk Junior
* **Anti-Pattern: Network Calls Inside DB Transactions:** Melakukan panggilan jaringan (seperti gRPC, HTTP, atau publish message broker) di dalam transaksi database lokal (`BeginTx` ... `Commit()`) adalah kesalahan arsitektur yang sangat fatal di production. Panggilan jaringan tidak bisa diprediksi kecepatannya (bisa memakan waktu beberapa detik jika ada gangguan latency). Selama pemanggilan tersebut, koneksi database ditahan (*held open*) dan tabel/baris database terkunci (*locked*), menyebabkan penumpukan antrean koneksi (*connection pool exhaustion*) dan membuat seluruh aplikasi crash.
* **Separation of Concerns & Eventual Consistency:** Dengan mencatat transaksi awal sebagai `PENDING` terlebih dahulu secara cepat, kita bebas melakukan panggilan gRPC eksternal di luar transaksi. Setelah semua gRPC sukses, kita membuka transaksi database pendek baru hanya untuk memperbarui status menjadi `SUCCESS` dan menyimpan Outbox Event. Cara ini meminimalkan durasi lock database hingga kurang dari 5 milidetik!
* **Handling Server Crash Between Steps:** Jika server crash persis setelah gRPC mutasi saldo sukses, namun sebelum status transaksi diubah menjadi `SUCCESS`: transaksi akan selamanya menggantung di status `PENDING`. Untuk mengatasinya, di production kita membuat program *Reconciliation/Compensation Worker* terjadwal yang bertugas mencari transaksi berstatus `PENDING` yang berusia lebih dari 5 menit, menanyakan status saldo/ledger ke Wallet/Ledger Service, lalu secara otomatis mem-finalize status transaksi tersebut ke `SUCCESS` atau `FAILED` demi tercapainya *eventual consistency*.
