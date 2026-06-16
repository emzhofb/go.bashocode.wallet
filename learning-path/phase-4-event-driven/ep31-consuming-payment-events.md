# Episode 31: Consuming Payment Events in Wallet Service

## 🎯 Tujuan
* Membuat **RabbitMQ Consumer** di dalam `wallet-service` untuk mendengarkan (*subscribe*) event `"payment.completed"`.
* Mengimplementasikan pengisian saldo (*Top Up*) secara otomatis saat event pembayaran sukses diterima.
* Menjamin konsistensi data dengan mencatatkan mutasi saldo di tabel `wallets` dan tabel `ledger_entries` di dalam satu transaksi database lokal (`sql.Tx`).

---

## 📐 Alur Event-Driven Top Up
Berikut adalah siklus aliran data saat pengguna melakukan Top Up:
1. `payment-service` menerima webhook dari gateway luar ➔ Memvalidasi HMAC ➔ Mem-publish event `payment.completed`.
2. `wallet-service` (Consumer) mendengarkan event tersebut dari Queue `wallet.payments`.
3. Consumer meng-up saldo wallet target di MySQL (menggunakan transaction + optimistic locking).
4. Consumer mencatatkan baris ledger bertipe `credit` (+) sebagai bukti audit mutasi uang.
5. Consumer memanggil `msg.Ack(false)` untuk menandakan saldo sukses diperbarui.

---

## 📦 Langkah-langkah

### Step 1: Membuat Queue & Consumer Baru di `wallet-service`
Buka file `wallet-service/internal/wallet/consumer/payment_consumer.go`. Kita akan membuat struct consumer untuk mengolah event pembayaran.

```go
package consumer

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"

	"github.com/emzhofb/gowallet/wallet-service/internal/logger"
	walletRepo "github.com/emzhofb/gowallet/wallet-service/internal/wallet/repository"
	ledgerPb "github.com/emzhofb/gowallet/ledger-service/proto/ledger"
	amqp "github.com/rabbitmq/amqp091-go"
)

type PaymentConsumer struct {
	db           *sql.DB
	amqpConn     *amqp.Connection
	channel      *amqp.Channel
	walletRepo   walletRepo.WalletRepository
	ledgerClient ledgerPb.LedgerServiceClient
}

type PaymentCompletedEvent struct {
	UserID  string  `json:"user_id"`
	Amount  float64 `json:"amount"`
	OrderID string  `json:"order_id"`
	Gateway string  `json:"gateway"`
}

func NewPaymentConsumer(
	db *sql.DB,
	conn *amqp.Connection,
	wRepo walletRepo.WalletRepository,
	lClient ledgerPb.LedgerServiceClient,
) *PaymentConsumer {
	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to open channel: %v", err)
	}

	return &PaymentConsumer{
		db:           db,
		amqpConn:     conn,
		channel:      ch,
		walletRepo:   wRepo,
		ledgerClient: lClient,
	}
}

func (c *PaymentConsumer) Start(ctx context.Context) {
	// 1. Declare Queue khusus untuk konsumsi event pembayaran
	queue, err := c.channel.QueueDeclare(
		"wallet.payments", // queue name
		true,              // durable
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		log.Fatalf("Failed to declare queue: %v", err)
	}

	// 2. Bind Queue ke Exchange utama dengan routing key "payment.completed"
	err = c.channel.QueueBind(
		queue.Name,
		"payment.completed",
		"wallet.events",
		false,
		nil,
	)
	if err != nil {
		log.Fatalf("Failed to bind queue: %v", err)
	}

	messages, err := c.channel.Consume(
		queue.Name,
		"",
		false, // manual ack
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		log.Fatalf("Failed to start consume: %v", err)
	}

	logger.Log.Info("Wallet Payment Consumer listening for payment events...")

	go func() {
		for {
			select {
			case <-ctx.Done():
				c.channel.Close()
				return
			case msg, ok := <-messages:
				if !ok {
					return
				}
				c.processPayment(ctx, msg)
			}
		}
	}()
}

func (c *PaymentConsumer) processPayment(ctx context.Context, msg amqp.Delivery) {
	logger.Info(ctx, "Received payment completed message", "message_id", msg.MessageId)

	// 1. Decode JSON Payload
	var event PaymentCompletedEvent
	err := json.Unmarshal(msg.Body, &event)
	if err != nil {
		logger.Error(ctx, "Failed to unmarshal JSON payload", "error", err.Error())
		msg.Nack(false, false) // Poison Message -> Langsung buang ke DLQ (jangan requeue)
		return
	}

	// 2. Buka Transaksi Database Lokal MySQL (hanya untuk tabel wallets)
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		logger.Error(ctx, "Failed to begin database transaction", "error", err.Error())
		msg.Nack(false, true) // Requeue untuk coba lagi
		return
	}
	defer tx.Rollback()

	// 3. Ambil data wallet user
	wallet, err := c.walletRepo.GetByUserID(ctx, event.UserID)
	if err != nil {
		logger.Error(ctx, "Wallet not found for user ID", "user_id", event.UserID, "error", err.Error())
		msg.Nack(false, false) // Jangan requeue karena datanya tidak valid
		return
	}

	// 4. Update saldo dengan Optimistic Locking
	newBalance := wallet.Balance + event.Amount
	err = c.walletRepo.UpdateBalanceTx(ctx, tx, wallet.ID, newBalance, wallet.Version)
	if err != nil {
		logger.Warn(ctx, "Concurrency conflict during balance update. Retrying...", "wallet_id", wallet.ID)
		msg.Nack(false, true) // Requeue agar dicoba lagi
		return
	}

	// 5. Commit Transaksi Lokal untuk melepaskan lock database
	if err := tx.Commit(); err != nil {
		logger.Error(ctx, "Failed to commit database transaction", "error", err.Error())
		msg.Nack(false, true)
		return
	}

	// 6. Catat Ledger Entry bertipe CREDIT (+) via gRPC ke Ledger Service (Database-per-Service)
	// Kita lakukan ini di luar transaksi lokal agar tidak menahan koneksi DB
	_, err = c.ledgerClient.RecordLedgerEntry(ctx, &ledgerPb.RecordEntryRequest{
		TransactionId: event.OrderID, // Gunakan order ID PG sebagai transaction ID reference
		WalletId:      wallet.ID,
		Type:          "CREDIT",
		Amount:        event.Amount,
	})
	if err != nil {
		logger.Error(ctx, "Failed to write credit ledger entry via gRPC", "error", err.Error())
		// Jika gRPC gagal, kita Nack agar message dikirim ulang oleh RabbitMQ.
		// Idempotency check di awal/di ledger-service akan mencegah data ganda.
		msg.Nack(false, true)
		return
	}

	logger.Info(ctx, "Wallet balance successfully credited via gRPC from payment webhook!", "user_id", event.UserID, "amount", event.Amount)
	msg.Ack(false)
}
```

### Step 2: Aktifkan Consumer di `wallet-service/cmd/main.go`
Buka `wallet-service/cmd/main.go`. Setup dial koneksi gRPC ke `ledger-service`, lalu inisialisasi dan jalankan `PaymentConsumer` di latar belakang bersamaan dengan worker lainnya:

```go
// Di dalam main.go wallet-service:
	
	// ... inisialisasi Repo ...
	
	// Hubungkan gRPC Client ke Ledger Service (port 50054)
	ledgerConn, _ := grpc.Dial("localhost:50054", grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer ledgerConn.Close()
	ledgerClient := ledgerPb.NewLedgerServiceClient(ledgerConn)
	
	// Inisialisasi Payment Consumer dengan gRPC Client
	paymentConsumer := walletConsumer.NewPaymentConsumer(db, amqpConn, wRepo, ledgerClient)
	
	// Start Consumer
	go paymentConsumer.Start(bgCtx)
```

---

## ✅ Acceptance Criteria
* [ ] Antrean `wallet.payments` terbuat sukses di database RabbitMQ.
* [ ] Menjalankan `payment-service` webhook sukses mem-publish event, dan `wallet-service` (Consumer) sukses menangkap event tersebut secara otomatis di latar belakang.
* [ ] Saldo di tabel `wallets` bertambah di `wallet-service` dan baris ledger kredit baru tercipta di database `ledger-service` (melalui pemanggilan gRPC internal).
* [ ] Kesalahan koneksi database atau kegagalan gRPC memicu `Nack(requeue=true)` sehingga saldo tidak akan pernah hilang (*no message loss*).

---

## 💡 Tips untuk Junior
* **Database-per-Service Principle:** Di arsitektur monolith lama, kita bisa menggabungkan penulisan saldo wallet dan pencatatan ledger dalam satu transaksi database lokal menggunakan `sql.Tx`. Namun di microservices, `wallet-service` tidak boleh mengakses atau menulis ke tabel `ledger_entries` milik `ledger-service` secara langsung karena melanggar kepemilikan data. Komunikasi wajib dilakukan melalui jalur API/gRPC.
* **Idempotency Check di Consumer:** Di production, sangat mungkin message yang sama dikirim dua kali oleh RabbitMQ (*At-least-once delivery*). Untuk mencegah pengisian saldo ganda (*double top up*), pastikan Anda melakukan pengecekan di awal apakah ID pembayaran tersebut (`OrderID`) sudah pernah tercatat di ledger/transaksi database kita. Jika sudah ada, langsung panggil `msg.Ack(false)` dan abaikan message tersebut agar tidak terjadi duplikasi pengisian uang.

---

## 📚 Referensi Belajar
* [RabbitMQ Consumer Acknowledge Best Practices](https://www.rabbitmq.com/confirms.html#consumer-acknowledgements)
* [Microservice Database Patterns (Database-per-Service)](https://microservices.io/patterns/data/database-per-service.html)
* [Data Consistency in Enterprise Systems](https://www.enterpriseintegrationpatterns.com/patterns/messaging/GuaranteedMessaging.html)
