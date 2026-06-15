# Episode 28: Audit Logging dengan MongoDB

## 🎯 Tujuan
* Menambahkan database NoSQL **MongoDB** ke Docker Compose untuk menyimpan log audit yang besar dan dinamis.
* Membuat microservice baru bernama `audit-service`.
* Mengimplementasikan RabbitMQ Consumer yang berlangganan (*subscribe*) ke seluruh event (*wildcard* topic: `*` atau `#`).
* Menyimpan riwayat jejak audit (*audit trail*) ke dalam database MongoDB.

---

## 📐 Mengapa NoSQL MongoDB untuk Audit Log?
Jejak audit (*audit trail*) adalah catatan permanen tentang siapa melakukan apa di dalam sistem.
* Jenis data audit sangat dinamis (event `user.registered` memiliki struktur payload yang berbeda jauh dari `transfer.completed`).
* Jika menggunakan database relational SQL biasa, kita harus membuat kolom berbeda atau menyimpan teks mentah yang sulit di-query.
* MongoDB sangat ideal karena dapat menyimpan dokumen BSON/JSON dinamis tanpa skema kaku (*schemaless*), dan mendukung indexing pencarian yang sangat cepat.

```
[ RabbitMQ Exchange: wallet.events ]
           │ (Routing Key: *)
           ▼
[ Queue: audit.logs ]
           │
           ▼
[ audit-service (Consumer) ] ➔ (Simpan ke) ➔ [ MongoDB Collection: audit_logs ]
```

---

## 📦 Langkah-langkah

### Step 1: Tambahkan MongoDB ke Docker Compose
Buka file `docker-compose.yml` di folder root workspace, tambahkan service `mongodb`:

```yaml
# ... services mysql, redis, rabbitmq ...
  mongodb:
    image: mongo:7.0
    container_name: gowallet-mongodb
    ports:
      - "27017:27017"
    volumes:
      - mongo_data:/data/db
    restart: always

volumes:
  mysql_data:
  mongo_data:
```
Jalankan container: `docker compose up -d`.

### Step 2: Membuat Folder audit-service
Buat struktur folder untuk `audit-service`:

```bash
mkdir -p audit-service/cmd audit-service/internal/{config,database,consumer}
```

Inisialisasi Go Module di dalam folder `audit-service`:
```bash
cd audit-service
go mod init github.com/emzhofb/gowallet/audit-service
cd ..
```

Daftarkan service baru ini ke Go Workspace di file `go.work`:
```bash
go work use ./audit-service
```

### Step 3: Install Driver MongoDB & AMQP Client
Unduh official driver MongoDB untuk Go:
```bash
go get go.mongodb.org/mongo-driver/mongo
```

### Step 4: Membuat MongoDB Connection Helper (`internal/database/mongodb.go`)
Buat file helper koneksi MongoDB di `audit-service/internal/database/mongodb.go`:

```go
package database

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func ConnectMongoDB(uri string) (*mongo.Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientOpts := options.Client().ApplyURI(uri)
	client, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		return nil, err
	}

	// Ping database untuk memastikan koneksi aktif
	err = client.Ping(ctx, nil)
	if err != nil {
		return nil, err
	}

	log.Println("Successfully connected to MongoDB!")
	return client, nil
}
```

### Step 5: Membuat Audit Consumer (`internal/consumer/audit_consumer.go`)
Consumer ini akan mendengarkan Exchange `wallet.events` menggunakan wildcard routing key `#` (yang artinya mendengarkan **semua event** yang ter-publish di exchange tersebut).

Buat file baru di `audit-service/internal/consumer/audit_consumer.go`:

```go
package consumer

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/emzhofb/gowallet/audit-service/internal/logger"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.mongodb.org/mongo-driver/mongo"
)

type AuditConsumer struct {
	amqpConn *amqp.Connection
	channel  *amqp.Channel
	db       *mongo.Database
}

type AuditLog struct {
	ID        string    `bson:"_id"`
	EventType string    `bson:"event_type"`
	Payload   any       `bson:"payload"` // Payload dinamis (JSON Map)
	CreatedAt time.Time `bson:"created_at"`
}

func NewAuditConsumer(conn *amqp.Connection, mongoClient *mongo.Client) *AuditConsumer {
	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to open channel: %v", err)
	}

	db := mongoClient.Database("gowallet_audit")

	return &AuditConsumer{
		amqpConn: conn,
		channel:  ch,
		db:       db,
	}
}

func (c *AuditConsumer) Start(ctx context.Context) {
	queue, err := c.channel.QueueDeclare(
		"audit.logs", // queue name
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		log.Fatalf("Failed to declare queue: %v", err)
	}

	// Gunakan routing key "#" (match any routing key)
	err = c.channel.QueueBind(
		queue.Name,
		"#", // Wildcard: terima seluruh event
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

	logger.Log.Info("Audit Consumer listening for all events...")

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

func (c *AuditConsumer) processMessage(ctx context.Context, msg amqp.Delivery) {
	logger.Info(ctx, "Audit log received", "event_type", msg.RoutingKey)

	// 1. Decode payload dinamis ke map generic
	var jsonPayload map[string]any
	err := json.Unmarshal(msg.Body, &jsonPayload)
	if err != nil {
		logger.Error(ctx, "Failed to unmarshal audit payload", "error", err.Error())
		msg.Nack(false, false)
		return
	}

	// 2. Rangkai Dokumen Audit Log
	doc := AuditLog{
		ID:        msg.MessageId,
		EventType: msg.RoutingKey,
		Payload:   jsonPayload,
		CreatedAt: time.Now(),
	}

	// 3. Simpan ke MongoDB Collection "audit_logs"
	collection := c.db.Collection("audit_logs")
	_, err = collection.InsertOne(ctx, doc)
	if err != nil {
		logger.Error(ctx, "Failed to save document to MongoDB", "error", err.Error())
		// Kembalikan ke queue agar dicoba ulang nanti (requeue = true)
		msg.Nack(false, true)
		return
	}

	logger.Info(ctx, "Audit log saved to MongoDB", "id", msg.MessageId)
	msg.Ack(false)
}
```

### Step 6: Setup Entry Point `audit-service/cmd/main.go`
Koneksikan ke MongoDB dan RabbitMQ, lalu aktifkan consumer:

```go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/emzhofb/gowallet/audit-service/internal/config"
	"github.com/emzhofb/gowallet/audit-service/internal/consumer"
	"github.com/emzhofb/gowallet/audit-service/internal/database"
	"github.com/emzhofb/gowallet/audit-service/internal/logger"
)

func main() {
	logger.InitLogger()
	logger.Log.Info("Starting Audit Microservice...")

	cfg := config.LoadConfig()

	// 1. Connect MongoDB
	mongoClient, err := database.ConnectMongoDB("mongodb://localhost:27017")
	if err != nil {
		log.Fatalf("Could not connect to MongoDB: %v", err)
	}
	defer mongoClient.Disconnect(context.Background())

	// 2. Connect RabbitMQ
	amqpConn, err := database.ConnectRabbitMQ(cfg.RabbitMQURL)
	if err != nil {
		log.Fatalf("Could not connect to RabbitMQ: %v", err)
	}
	defer amqpConn.Close()

	// 3. Start Consumer
	auditConsumer := consumer.NewAuditConsumer(amqpConn, mongoClient)
	bgCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	auditConsumer.Start(bgCtx)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Log.Info("Shutting down Audit Service...")
}
```

---

## ✅ Acceptance Criteria
* [ ] Container MongoDB berjalan lancar di Docker Compose port `27017`.
* [ ] Menjalankan `audit-service` berhasil memunculkan log `Successfully connected to MongoDB!`.
* [ ] Setiap ada transaksi transfer sukses, dokumen baru terbuat di database MongoDB `gowallet_audit` collection `audit_logs` secara asinkronus (dapat divalidasi dengan Compass atau MongoDB shell CLI).

---

## 💡 Tips untuk Junior
* **Wildcard Routing Key:** Karakter `#` pada binding RabbitMQ bertindak sebagai wildcard multi-kata (menerima semua routing key). Sedangkan `*` bertindak sebagai wildcard satu kata saja (misal: `transfer.*` akan menerima `transfer.completed` tapi menolak `transfer.completed.success`). Gunakan `#` untuk menangkap seluruh log sistem tanpa terlewat.

---

## 📚 Referensi Belajar
* [MongoDB Go Driver official documentation](https://www.mongodb.com/docs/drivers/go/current/)
* [RabbitMQ Topic Exchanges (Wildcard routing keys)](https://www.rabbitmq.com/tutorials/tutorial-five-go.html)
