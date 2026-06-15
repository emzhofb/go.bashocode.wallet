# Episode 20: RabbitMQ & Event Publishing

## 🎯 Tujuan
* Menambahkan **RabbitMQ** ke Docker Compose sebagai Message Broker utama.
* Membuat program publisher (*outbox worker*) yang berjalan secara periodik untuk mengambil event pending di tabel `outbox_events`, mem-publish ke RabbitMQ, dan mengubah status event menjadi `"processed"`.
* Memahami konsep **At-Least-Once Delivery** (pesan dijamin terkirim minimal sekali).

---

## 📐 Konsep Outbox Polling Publisher
Di Episode 19, kita sukses menyimpan event di tabel `outbox_events`.
Sekarang kita membuat sebuah background worker di dalam `wallet-service` yang berjalan setiap 5 detik:
1. Membaca baris tabel `SELECT * FROM outbox_events WHERE status = 'pending' LIMIT 50`.
2. Untuk setiap event, kirim payload JSON ke RabbitMQ Exchange.
3. Jika pengiriman ke RabbitMQ sukses, update status event di database menjadi `"processed"` (atau hapus baris tersebut agar database tidak membengkak).
4. Jika RabbitMQ mati, status tetap `"pending"` dan akan di-retry pada putaran berikutnya.

```
[ outbox_events table ] ➔ (Poll every 5s) ➔ [ Go Outbox Worker ] ➔ (Publish) ➔ [ RabbitMQ ]
       ▲                                            │
       └────────────── (Update status: processed) ──┘
```

---

## 📦 Langkah-langkah

### Step 1: Tambahkan RabbitMQ ke Docker Compose
Buka file `docker-compose.yml` di folder root workspace, tambahkan service `rabbitmq`:

```yaml
# ... services mysql & redis ...
  rabbitmq:
    image: rabbitmq:3-management-alpine
    container_name: gowallet-rabbitmq
    ports:
      - "5672:5672"     # Port AMQP untuk koneksi aplikasi
      - "15672:15672"   # Port Web UI Management Console
    restart: always
```
Jalankan container: `docker compose up -d`. 
Buka dashboard UI RabbitMQ di browser `http://localhost:15672` (username/password default: `guest`/`guest`).

### Step 2: Install Library AMQP Go Client
Unduh official library client RabbitMQ untuk Go:
```bash
go get github.com/rabbitmq/amqp091-go
```

### Step 3: Update `.env` & Config Loader
Tambahkan URL koneksi RabbitMQ di file `.env`:
```env
RABBITMQ_URL=amqp://guest:guest@localhost:5672/
```

Update config loader `internal/config/config.go` untuk membaca `RABBITMQ_URL`.

### Step 4: Membuat RabbitMQ Connection Wrapper (`internal/database/rabbitmq.go`)
Buat file helper koneksi RabbitMQ di `internal/database/rabbitmq.go`:

```go
package database

import (
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

func ConnectRabbitMQ(url string) (*amqp.Connection, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, err
	}
	log.Println("Successfully connected to RabbitMQ!")
	return conn, nil
}
```

### Step 5: Membuat Outbox Publisher Worker (`internal/transaction/worker/outbox.go`)
Buat folder baru `internal/transaction/worker/` dan buat file `outbox.go`. Worker ini akan melakukan polling database dan menembakkan event ke RabbitMQ.

```go
package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/emzhofb/gowallet/wallet-service/internal/logger"
	"github.com/emzhofb/gowallet/wallet-service/internal/transaction/model"
	amqp "github.com/rabbitmq/amqp091-go"
)

type OutboxWorker struct {
	db       *sql.DB
	amqpConn *amqp.Connection
	channel  *amqp.Channel
}

func NewOutboxWorker(db *sql.DB, conn *amqp.Connection) *OutboxWorker {
	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to open channel: %v", err)
	}

	// Declare Exchange utama untuk transaksi wallet
	err = ch.ExchangeDeclare(
		"wallet.events", // exchange name
		"topic",         // exchange type
		true,            // durable
		false,           // auto-deleted
		false,           // internal
		false,           // no-wait
		nil,             // arguments
	)
	if err != nil {
		log.Fatalf("Failed to declare exchange: %v", err)
	}

	return &OutboxWorker{
		db:       db,
		amqpConn: conn,
		channel:  ch,
	}
}

func (w *OutboxWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	logger.Log.Info("Outbox Publisher Worker started...")

	for {
		select {
		case <-ctx.Done():
			w.channel.Close()
			return
		case <-ticker.C:
			w.processPendingEvents(ctx)
		}
	}
}

func (w *OutboxWorker) processPendingEvents(ctx context.Context) {
	// 1. Ambil event pending terlama
	query := `SELECT id, event_type, payload FROM outbox_events WHERE status = 'pending' ORDER BY created_at ASC LIMIT 20`
	rows, err := w.db.QueryContext(ctx, query)
	if err != nil {
		logger.Error(ctx, "Failed to query pending outbox events", "error", err.Error())
		return
	}
	defer rows.Close()

	var events []model.OutboxEvent
	for rows.Next() {
		var e model.OutboxEvent
		if err := rows.Scan(&e.ID, &e.EventType, &e.Payload); err != nil {
			continue
		}
		events = append(events, e)
	}

	if len(events) == 0 {
		return
	}

	logger.Info(ctx, "Publishing pending outbox events", "count", len(events))

	// 2. Publish satu per satu ke RabbitMQ
	for _, event := range events {
		err = w.channel.PublishWithContext(
			ctx,
			"wallet.events", // exchange
			event.EventType, // routing key (misal: "transfer.completed")
			false,           // mandatory
			false,           // immediate
			amqp.Publishing{
				ContentType: "application/json",
				Body:        []byte(event.Payload),
				MessageId:   event.ID,
			},
		)

		if err != nil {
			logger.Error(ctx, "Failed to publish event to RabbitMQ", "event_id", event.ID, "error", err.Error())
			// Naikkan jumlah attempts di database
			_, _ = w.db.ExecContext(ctx, "UPDATE outbox_events SET attempts = attempts + 1 WHERE id = ?", event.ID)
			continue
		}

		// 3. Update status menjadi processed jika sukses terkirim ke RabbitMQ
		_, err = w.db.ExecContext(ctx, "UPDATE outbox_events SET status = 'processed' WHERE id = ?", event.ID)
		if err != nil {
			logger.Error(ctx, "Failed to update outbox event status", "event_id", event.ID, "error", err.Error())
		}
	}
}
```

### Step 6: Jalankan Worker di `cmd/main.go`
Koneksikan RabbitMQ di `main.go` dan aktifkan outbox worker di latar belakang.

```go
// Di dalam main.go wallet-service:
	
	// ... koneksi database & Redis ...
	
	// Connect to RabbitMQ
	amqpConn, err := database.ConnectRabbitMQ(cfg.RabbitMQURL)
	if err != nil {
		log.Fatalf("Could not connect to RabbitMQ: %v", err)
	}
	defer amqpConn.Close()

	// Inisialisasi & Start Outbox Worker
	outboxWorker := worker.NewOutboxWorker(db, amqpConn)
	
	bgCtx, cancelWorker := context.WithCancel(context.Background())
	defer cancelWorker()
	
	go outboxWorker.Start(bgCtx) // Jalankan asinkronus
```

---

## ✅ Acceptance Criteria
* [ ] Container RabbitMQ berjalan lancar dan Web Management UI bisa diakses di port `15672`.
* [ ] Menjalankan `wallet-service` memicu log koneksi sukses ke RabbitMQ.
* [ ] Setelah melakukan transaksi transfer, log outbox worker menampilkan data `Publishing pending outbox events` dan record di tabel `outbox_events` berubah status menjadi `"processed"`.
* [ ] Di dashboard RabbitMQ Management, jumlah message masuk di exchange `wallet.events` bertambah.

---

## 💡 Tips untuk Junior
* **Exchange Type - Topic:** Kita menggunakan exchange berjenis `topic` karena ini adalah tipe fleksibel yang memungkinkan message disalurkan berdasarkan wildcard routing key (seperti `transfer.completed`, `payment.failed`, dll), mempermudah service-service lain yang ingin berlangganan sebagian event saja.

---

## 📚 Referensi Belajar
* [RabbitMQ Official Go Tutorial](https://www.rabbitmq.com/tutorials/tutorial-one-go.html)
* [Outbox Polling vs Transaction Log Tailing](https://debezium.io/blog/2019/02/19/outbox-pattern-with-debezium/)
