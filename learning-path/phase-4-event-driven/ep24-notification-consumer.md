# Episode 21: Notification Service (Consumer)

## 🎯 Tujuan
* Membuat microservice baru bernama `notification-service`.
* Mendaftarkan service baru ke workspace `go.work`.
* Mengimplementasikan **RabbitMQ Consumer** yang mendengarkan (*subscribe*) event `"transfer.completed"`.
* Memproses message payload dan mengirimkan email simulasi notifikasi transfer ke pihak pengirim dan penerima.

---

## 📐 Konsep Asynchronous Message Consumption
Sekarang, alur notifikasi email berjalan di microservice terpisah.
1. `wallet-service` mem-publish event `transfer.completed` ke Exchange `wallet.events`.
2. `notification-service` membuat antrean (*Queue*) sendiri bernama `notification.emails` yang dibinding ke Exchange `wallet.events` dengan routing key `transfer.completed`.
3. RabbitMQ akan otomatis menyalurkan setiap message transfer ke Queue milik `notification-service`.
4. Consumer membaca message secara *real-time*, men-decode payload, dan mengeksekusi pengiriman email.

```
[ RabbitMQ Exchange: wallet.events ]
           │ (Routing Key: transfer.completed)
           ▼
[ Queue: notification.emails ]
           │
           ▼
[ notification-service (Consumer) ] ➔ (Kirim Email Notifikasi)
```

---

## 📦 Langkah-langkah

### Step 1: Membuat Folder notification-service
Buat struktur folder untuk `notification-service`:

```bash
mkdir -p notification-service/cmd notification-service/internal/{config,database,consumer}
```

Inisialisasi Go Module di dalam folder `notification-service`:
```bash
cd notification-service
go mod init github.com/emzhofb/gowallet/notification-service
cd ..
```

Daftarkan service baru ini ke Go Workspace di file `go.work`:
```bash
go work use ./notification-service
```

### Step 2: Salin Modul Database & Config
Salin file helper database (`rabbitmq.go`), structured logging (`logger.go`), dan config loader (`config.go`) dari service sebelumnya ke folder `notification-service/internal/` yang sesuai.

### Step 3: Membuat Consumer Logic (`internal/consumer/email_consumer.go`)
Consumer ini akan berjalan secara kontinu, membuat channel, men-declare queue, dan mendengarkan antrean dari RabbitMQ.

Buat file baru di `notification-service/internal/consumer/email_consumer.go`:

```go
package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/emzhofb/gowallet/notification-service/internal/logger"
	amqp "github.com/rabbitmq/amqp091-go"
)

type EmailConsumer struct {
	amqpConn *amqp.Connection
	channel  *amqp.Channel
}

type TransferCompletedPayload struct {
	TransactionID  string  `json:"transaction_id"`
	SenderUserID   string  `json:"sender_user_id"`
	ReceiverUserID string  `json:"receiver_user_id"`
	Amount         float64 `json:"amount"`
	Description    string  `json:"description"`
}

func NewEmailConsumer(conn *amqp.Connection) *EmailConsumer {
	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to open channel: %v", err)
	}
	return &EmailConsumer{
		amqpConn: conn,
		channel:  ch,
	}
}

func (c *EmailConsumer) Start(ctx context.Context) {
	// 1. Declare Queue khusus untuk notifikasi email
	queue, err := c.channel.QueueDeclare(
		"notification.emails", // queue name
		true,                  // durable
		false,                 // delete when unused
		false,                 // exclusive
		false,                 // no-wait
		nil,                   // arguments
	)
	if err != nil {
		log.Fatalf("Failed to declare queue: %v", err)
	}

	// 2. Bind Queue ke Exchange utama dengan routing key target
	err = c.channel.QueueBind(
		queue.Name,             // queue name
		"transfer.completed",   // routing key
		"wallet.events",        // exchange name
		false,
		nil,
	)
	if err != nil {
		log.Fatalf("Failed to bind queue to exchange: %v", err)
	}

	// 3. Daftarkan consumer ke Queue
	messages, err := c.channel.Consume(
		queue.Name, // queue name
		"",         // consumer identifier
		false,      // auto-ack (kita set false agar kita bisa mengontrol acknowledgment manual)
		false,      // exclusive
		false,      // no-local
		false,      // no-wait
		nil,        // args
	)
	if err != nil {
		log.Fatalf("Failed to register consumer: %v", err)
	}

	logger.Log.Info("Notification Email Consumer listening for messages...")

	// 4. Loop tak terbatas untuk memproses setiap message yang masuk
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
				c.processMessage(ctx, msg)
			}
		}
	}()
}

func (c *EmailConsumer) processMessage(ctx context.Context, msg amqp.Delivery) {
	logger.Info(ctx, "Received message from RabbitMQ queue", "message_id", msg.MessageId)

	// 1. Parse JSON Payload
	var payload TransferCompletedPayload
	err := json.Unmarshal(msg.Body, &payload)
	if err != nil {
		logger.Error(ctx, "Failed to unmarshal JSON payload", "error", err.Error())
		
		// Nack message (jangan kembalikan ke antrean karena format data rusak / Poison Message)
		msg.Nack(false, false)
		return
	}

	// 2. Simulasi Pengiriman Email (di Episode asli, panggil SMTP Helper)
	logger.Info(ctx, "Sending Notification Email...",
		"transaction_id", payload.TransactionID,
		"sender_id", payload.SenderUserID,
		"receiver_id", payload.ReceiverUserID,
		"amount", payload.Amount,
	)
	fmt.Printf("[EMAIL SIMULATION] Halo User %s, Anda berhasil mengirim saldo sebesar Rp %.2f ke User %s.\n", 
		payload.SenderUserID, payload.Amount, payload.ReceiverUserID)

	// 3. Kirim Acknowledgment ke RabbitMQ untuk menandakan message sukses diproses
	err = msg.Ack(false)
	if err != nil {
		logger.Error(ctx, "Failed to acknowledge message to RabbitMQ", "message_id", msg.MessageId, "error", err.Error())
	}
}
```

### Step 4: Setup Entry Point `notification-service/cmd/main.go`
Buat file `main.go` untuk inisialisasi koneksi RabbitMQ dan menjalankan Email Consumer:

```go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/emzhofb/gowallet/notification-service/internal/config"
	"github.com/emzhofb/gowallet/notification-service/internal/consumer"
	"github.com/emzhofb/gowallet/notification-service/internal/database"
	"github.com/emzhofb/gowallet/notification-service/internal/logger"
)

func main() {
	logger.InitLogger()
	logger.Log.Info("Starting Notification Microservice...")

	cfg := config.LoadConfig()

	// Connect to RabbitMQ
	amqpConn, err := database.ConnectRabbitMQ(cfg.RabbitMQURL)
	if err != nil {
		log.Fatalf("Could not connect to RabbitMQ: %v", err)
	}
	defer amqpConn.Close()

	// Start Consumer
	emailConsumer := consumer.NewEmailConsumer(amqpConn)
	bgCtx, cancelConsumer := context.WithCancel(context.Background())
	defer cancelConsumer()

	emailConsumer.Start(bgCtx)

	// Handle graceful stop
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Log.Info("Shutting down Notification Service...")
}
```

---

## ✅ Acceptance Criteria
* [ ] Folder `notification-service` sukses terdaftar di workspace `go.work`.
* [ ] Menjalankan `notification-service` memicu log `Notification Email Consumer listening for messages...`.
* [ ] Setelah pengirim melakukan transfer di API Gateway, log `notification-service` menampilkan print email simulasi `[EMAIL SIMULATION] Halo User ...` secara asinkronus beberapa detik kemudian.
* [ ] Menghentikan `notification-service` menutup channel RabbitMQ dengan rapi.

---

## 💡 Tips untuk Junior
* **Manual Acknowledgment (`msg.Ack`):** Jangan gunakan `auto-ack = true` untuk transaksi finansial/kritis. Jika `auto-ack` aktif, RabbitMQ akan menghapus message dari antrean begitu message dikirim, tanpa peduli apakah service kita mati mendadak di tengah proses kirim email. Dengan `auto-ack = false`, RabbitMQ akan menyimpan ulang message ke antrean jika consumer mati sebelum memanggil `msg.Ack(false)`.

---

## 📚 Referensi Belajar
* [RabbitMQ Consumer Patterns](https://www.rabbitmq.com/consumers.html)
* [Message Acknowledgements tutorial](https://www.rabbitmq.com/confirms.html)
