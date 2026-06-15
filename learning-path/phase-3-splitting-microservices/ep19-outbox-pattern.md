# Episode 19: Outbox Pattern untuk Transaksi Terdistribusi

## 🎯 Tujuan
* Memahami tantangan **Transaksi Terdistribusi** (Distributed Transactions) dalam arsitektur microservices.
* Mengenalkan pola **Outbox Pattern** untuk memastikan data konsisten tanpa kehilangan pesan (*guaranteed message delivery*).
* Mendesain skema migrasi tabel `outbox_events`.
* Mengubah logika `Transfer` agar menulis data event ke tabel outbox di dalam rangkaian transaksi database yang sama.

---

## 📐 Kenapa Butuh Outbox Pattern?
Bayangkan ketika transfer uang sukses, kita ingin mengirim email notifikasi ke penerima ("Anda menerima dana Rp 50.000").
* **Skenario Buruk 1 (Direct Network Call):** Setelah DB commit transfer, Wallet Service menembak API Notification Service secara langsung via HTTP. Bagaimana jika Notification Service sedang down, atau jaringan terputus? Transaksi di MySQL sukses (saldo terpotong), tapi email tidak terkirim selamanya.
* **Skenario Buruk 2 (Direct Queue Publish):** Bagaimana jika kita mem-publish event ke RabbitMQ langsung di tengah-tengah kode Go sebelum MySQL commit? Jika MySQL mendadak rollback (misal karena constraint error), event terlanjur dikirim ke RabbitMQ. Uang gagal dikirim, tapi email notifikasi terlanjur terkirim ke user!

### Solusi: Outbox Pattern
Daripada memanggil API eksternal atau mem-publish event ke RabbitMQ langsung dari thread transaksi:
1. Kita membuat tabel pembantu bernama `outbox_events` di database yang sama dengan tabel `wallets`.
2. Di dalam transaksi SQL transfer saldo, kita **juga menyimpan baris event baru** ke tabel `outbox_events` (misal: event `transfer.completed`).
3. Karena berada di transaksi database yang sama, penyimpanan event ke outbox dijamin 100% konsisten dengan perubahan saldo (jika transfer gagal, data outbox ikut ter-rollback).
4. Selesai! Tugas mem-publish event tersebut ke RabbitMQ diserahkan kepada background worker terpisah (yang dibahas di Fase 4).

```
[ SQL Transaction Begin ]
 ├── 1. Kurangi Saldo Pengirim
 ├── 2. Tambah Saldo Penerima
 ├── 3. Simpan Mutasi Ledger
 └── 4. Simpan Event ke outbox_events ("transfer.completed")
[ SQL Transaction Commit ] (Semua langkah di atas dijamin atomik)
```

---

## 📦 Langkah-langkah

### Step 1: Membuat Skema Migrasi Tabel Outbox
Buat file migrasi baru `db/migrations/000003_create_outbox_table.up.sql` di `wallet-service/`:

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

Buat file down migrasi `db/migrations/000003_create_outbox_table.down.sql`:
```sql
DROP TABLE IF EXISTS outbox_events;
```

Jalankan perintah migrasi di folder `wallet-service/`:
```bash
migrate -path db/migrations -database "mysql://gowallet_user:gowallet_password@tcp(localhost:3306)/gowallet" up
```

### Step 2: Membuat Model Outbox Event (`internal/transaction/model/outbox.go`)
Buat file baru di `wallet-service/internal/transaction/model/outbox.go`:

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
Buka `wallet-service/internal/transaction/repository/repository.go`. Tambahkan method `CreateOutboxTx` ke interface dan implementasinya:

```go
// Tambah di interface TransactionRepository:
// CreateOutboxTx(ctx context.Context, tx *sql.Tx, event *model.OutboxEvent) error

func (r *mysqlTransactionRepository) CreateOutboxTx(ctx context.Context, tx *sql.Tx, event *model.OutboxEvent) error {
	query := `INSERT INTO outbox_events (id, event_type, payload, status) VALUES (?, ?, ?, ?)`
	_, err := tx.ExecContext(ctx, query, event.ID, event.EventType, event.Payload, event.Status)
	return err
}
```

### Step 4: Modifikasi Logic Transfer di TransactionService
Buka `wallet-service/internal/transaction/service/service.go`. 

Di dalam method `Transfer`, tepat sebelum `tx.Commit()`, tambahkan kode untuk merangkai event payload JSON dan menyimpannya ke tabel outbox:

```go
// Di dalam method Transfer sebelum tx.Commit():

	// 8. Rangkai Payload Event (Detail transfer yang dibutuhkan oleh service lain)
	eventPayload := fmt.Sprintf(`{
		"transaction_id": "%s",
		"sender_user_id": "%s",
		"receiver_user_id": "%s",
		"amount": %.2f,
		"description": "%s"
	}`, transactionID, senderUserID, receiverUser.GetId(), req.Amount, req.Description)

	outboxEvent := &model.OutboxEvent{
		ID:        uuid.New().String(),
		EventType: "transfer.completed",
		Payload:   eventPayload,
		Status:    "pending",
	}

	// 9. Simpan ke tabel outbox menggunakan koneksi Tx transaksi yang sama
	if err := s.txRepo.CreateOutboxTx(ctx, tx, outboxEvent); err != nil {
		return nil, customErr.ErrInternalServer
	}

	// 10. Commit transaksi
	if err := tx.Commit(); err != nil {
		return nil, customErr.ErrInternalServer
	}
```

---

## ✅ Acceptance Criteria
* [ ] Tabel `outbox_events` terbuat dengan tipe kolom payload berupa `JSON` atau `TEXT`.
* [ ] Setiap kali transfer uang berhasil dilakukan, satu record baru otomatis tersimpan di tabel `outbox_events` dengan status `"pending"` dan `event_type` bernilai `"transfer.completed"`.
* [ ] Jika proses transfer dibatalkan (Rollback), tidak boleh ada record outbox baru yang tertinggal di database.

---

## 💡 Tips untuk Junior
* **Database as Message Queue:** Outbox pattern adalah jembatan terbaik untuk menjamin reliabilitas data (*At-least-once Delivery*) di microservices. Database SQL kita sementara bertindak sebagai antrean pesan yang aman dan tahan banting.

---

## 📚 Referensi Belajar
* [Transactional Outbox Pattern Pattern](https://microservices.io/patterns/data/transactional-outbox.html)
* [Distributed Systems Consistency Challenges](https://www.confluent.io/blog/transactional-outbox-pattern-confluent-platform/)
