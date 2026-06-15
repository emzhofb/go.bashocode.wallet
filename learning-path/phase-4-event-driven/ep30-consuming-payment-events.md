# Episode 30: Consuming Payment Events in Wallet Service

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
	ledgerModel "github.com/emzhofb/gowallet/wallet-service/internal/ledger/model"
	ledgerRepo "github.com/emzhofb/gowallet/wallet-service/internal/ledger/repository"
	walletRepo "github.com/emzhofb/gowallet/wallet-service/internal/wallet/repository"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/google/uuid"
)

type PaymentConsumer struct {
	db         *sql.DB
	amqpConn   *amqp.Connection
	channel    *amqp.Channel
	walletRepo walletRepo.WalletRepository
	ledgerRepo ledgerRepo.LedgerRepository
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
	lRepo ledgerRepo.LedgerRepository,
) *PaymentConsumer {
	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to open channel: %v", err)
	}

	return &PaymentConsumer{
		db:         db,
		amqpConn:   conn,
		channel:    ch,
		walletRepo: wRepo,
		ledgerRepo: lRepo,
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
		msg.Nack(false, false) // Poison Message -> Langsung buang ke DLQ
		return
	}

	// 2. Buka Transaksi Database Lokal MySQL
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
		// Tidak ada wallet -> Nack (jangan requeue karena datanya tidak valid)
		msg.Nack(false, false)
		return
	}

	// 4. Update saldo dengan Optimistic Locking
	newBalance := wallet.Balance + event.Amount
	err = c.walletRepo.UpdateBalanceTx(ctx, tx, wallet.ID, newBalance, wallet.Version)
	if err != nil {
		logger.Warn(ctx, "Concurrency conflict during balance update. Retrying in next message read...", "wallet_id", wallet.ID)
		msg.Nack(false, true) // Requeue agar dibaca ulang (retry)
		return
	}

	// 5. Catat Ledger Entry bertipe Credit (+)
	creditEntry := &ledgerModel.LedgerEntry{
		ID:            uuid.New().String(),
		WalletID:      wallet.ID,
		TransactionID: event.OrderID, // Gunakan order ID dari PG sebagai transaction ID reference
		EntryType:     "credit",
		Amount:        event.Amount,
	}

	if err := c.ledgerRepo.CreateTx(ctx, tx, creditEntry); err != nil {
		logger.Error(ctx, "Failed to write credit ledger entry", "error", err.Error())
		msg.Nack(false, true)
		return
	}

	// 6. Commit Transaksi
	if err := tx.Commit(); err != nil {
		logger.Error(ctx, "Failed to commit database transaction", "error", err.Error())
		msg.Nack(false, true)
		return
	}

	logger.Info(ctx, "Wallet balance successfully credited from payment webhook!", "user_id", event.UserID, "amount", event.Amount)
	msg.Ack(false)
}
```

### Step 2: Aktifkan Consumer di `wallet-service/cmd/main.go`
Buka `wallet-service/cmd/main.go`. Inisialisasi dan jalankan `PaymentConsumer` di latar belakang bersamaan dengan worker lainnya:

```go
// Di dalam main.go wallet-service:
	
	// ... inisialisasi Repo ...
	
	// Inisialisasi Payment Consumer
	paymentConsumer := walletConsumer.NewPaymentConsumer(db, amqpConn, wRepo, lRepo)
	
	// Start Consumer
	go paymentConsumer.Start(bgCtx)
```

---

## ✅ Acceptance Criteria
* [ ] Antrean `wallet.payments` terbuat sukses di database RabbitMQ.
* [ ] Menjalankan `payment-service` webhook sukses mem-publish event, dan `wallet-service` (Consumer) sukses menangkap event tersebut secara otomatis di latar belakang.
* [ ] Saldo di tabel `wallets` bertambah dan satu baris ledger kredit baru tercipta di tabel `ledger_entries` MySQL secara instan dan aman.
* [ ] Kesalahan koneksi database memicu `Nack(requeue=true)` sehingga saldo tidak akan pernah hilang (*no message loss*).

---

## 💡 Tips untuk Junior
* **Idempotency Check di Consumer:** Di production, sangat mungkin message yang sama dikirim dua kali oleh RabbitMQ (*At-least-once delivery*). Untuk mencegah pengisian saldo ganda (*double top up*), pastikan Anda melakukan pengecekan di awal apakah ID pembayaran tersebut (`OrderID`) sudah pernah tercatat di ledger/transaksi database kita. Jika sudah ada, langsung panggil `msg.Ack(false)` dan abaikan message tersebut agar tidak terjadi duplikasi pengisian uang.

---

## 📚 Referensi Belajar
* [RabbitMQ Consumer Acknowledge Best Practices](https://www.rabbitmq.com/confirms.html#consumer-acknowledgements)
* [Data Consistency in Enterprise Systems](https://www.enterpriseintegrationpatterns.com/patterns/messaging/GuaranteedMessaging.html)
