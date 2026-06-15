# Episode 33: RabbitMQ Resiliency (DLQ & Retries)

## 🎯 Tujuan
* Memahami ancaman **Poison Message** (pesan rusak yang gagal diproses terus-menerus dan memblokir antrean).
* Mengonfigurasi **Dead Letter Exchange (DLX)** dan **Dead Letter Queue (DLQ)** di RabbitMQ.
* Membuat alur **Automatic Retry** menggunakan konsep *Message TTL* (Time-To-Live) dan *Exchange Binding*.

---

## 📐 Konsep Poison Message & DLQ
Apa yang terjadi jika consumer kita crash saat membaca suatu message tertentu karena payload JSON-nya rusak/corrupt?
* Jika kita menggunakan `auto-ack = false` dan memanggil `msg.Nack(false, true)` (requeue = true), RabbitMQ akan langsung mengembalikan message tersebut ke barisan paling depan antrean.
* Consumer akan membaca message tersebut lagi, crash lagi, me-requeue lagi, membaca lagi... Terjadi **looping tanpa henti (infinite loop)** yang membakar CPU server kita dan memblokir message sehat lainnya di belakang. Ini disebut **Poison Message**.

### Solusi Resiliensi: DLQ & Retry Queue
Kita membuat alur penanganan error bertahap:
1. **Normal Queue (`notification.emails`):** Mendengarkan event normal. Jika terjadi error sistem (misal SMTP email mati), kita tidak membuang pesan, tapi memindahkan pesan ke **Retry Queue**.
2. **Retry Queue (`notification.emails.retry`):** Queue khusus yang disetel memiliki TTL (misal 10 detik). Tidak ada consumer aktif di queue ini. Pesan akan didiamkan selama 10 detik. Begitu waktu tunggu habis, pesan otomatis diteruskan kembali ke Exchange utama untuk dicoba kembali (*retry*).
3. **Dead Letter Queue (DLQ) (`notification.emails.dlq`):** Jika pesan sudah dicoba berkali-kali (misal 3 kali) dan tetap gagal, kita memindahkannya ke DLQ. Pesan di DLQ disimpan permanen agar tim developer bisa melakukan investigasi manual secara offline.

```
                  [ RabbitMQ Exchange: wallet.events ]
                               │
                      (Normal Publish)
                               ▼
            [ Queue: notification.emails (Consumer) ] ➔ (Sukses ➔ ACK)
                               │
                        (Gagal diproses)
                               ▼
            [ Queue: notification.emails.retry ] (Tunggu 10s TTL)
                               │
                       (Waktu tunggu habis)
                               ▼
            [ Exchange utama / Redelivery ] (Coba lagi)
                               │
                        (Gagal > 3 kali)
                               ▼
            [ Queue: notification.emails.dlq ] (Disimpan untuk investigasi)
```

---

## 📦 Langkah-langkah

### Step 1: Modifikasi Deklarasi Queue di Consumer Go
Buka file `notification-service/internal/consumer/email_consumer.go`. Kita akan mengonfigurasi arguments saat men-declare queue utama agar mendefinisikan DLX bawaan.

```go
func (c *EmailConsumer) Start(ctx context.Context) {
	// 1. Declare Exchange khusus untuk Dead Letter (DLX)
	err := c.channel.ExchangeDeclare(
		"wallet.events.dlx", // name
		"topic",             // type
		true,                // durable
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		log.Fatalf("Failed to declare DLX exchange: %v", err)
	}

	// 2. Declare Queue DLQ untuk menyimpan pesan gagal
	dlq, err := c.channel.QueueDeclare(
		"notification.emails.dlq",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		log.Fatalf("Failed to declare DLQ: %v", err)
	}

	// Bind DLQ ke DLX
	err = c.channel.QueueBind(dlq.Name, "#", "wallet.events.dlx", false, nil)
	if err != nil {
		log.Fatalf("Failed to bind DLQ to DLX: %v", err)
	}

	// 3. Declare Queue Utama dengan argumen x-dead-letter-exchange
	args := amqp.Table{
		"x-dead-letter-exchange": "wallet.events.dlx", // Jika Nack(requeue=false), buang pesan ke exchange ini
	}

	queue, err := c.channel.QueueDeclare(
		"notification.emails", // queue name
		true,                  // durable
		false,
		false,
		false,
		false,
		args, // Kirim argumen resiliensi
	)
	if err != nil {
		log.Fatalf("Failed to declare main queue: %v", err)
	}

	// ... bind queue & start consume ...
```

### Step 2: Konfigurasi Retry Queue dengan TTL (Opsional - Skema Sederhana)
Jika ingin mengaktifkan Retry Queue otomatis:
```go
	// Declare Retry Queue dengan TTL 10.000 ms (10 detik)
	retryArgs := amqp.Table{
		"x-dead-letter-exchange": "wallet.events",       // Lempar kembali ke exchange utama setelah TTL habis
		"x-message-ttl":          int32(10000),           // Waktu tunggu di queue retry (10s)
	}
    
	_, err = c.channel.QueueDeclare(
		"notification.emails.retry",
		true,
		false,
		false,
		false,
		false,
		retryArgs,
	)
```

### Step 3: Implementasi Pengecekan Retry Count di Go Code
Di dalam fungsi `processMessage`, kita mengecek header `x-death` bawaan RabbitMQ untuk mengetahui sudah berapa kali pesan ini mati dan masuk retry queue:

```go
func (c *EmailConsumer) processMessage(ctx context.Context, msg amqp.Delivery) {
	// Panggil helper untuk menghitung jumlah kegagalan (retry count)
	retryCount := getRetryCount(msg.Headers)
	
	logger.Info(ctx, "Processing message", "id", msg.MessageId, "retry_attempt", retryCount)

	// Decode JSON
	var payload TransferCompletedPayload
	err := json.Unmarshal(msg.Body, &payload)
	if err != nil {
		logger.Error(ctx, "JSON corrupt! Direct Nack without retry to DLQ.", "id", msg.MessageId)
		// requeue = false agar langsung terlempar ke DLQ karena datanya memang rusak permanen
		msg.Nack(false, false) 
		return
	}

	// Panggil logika bisnis (misal kirim email)
	err = c.sendEmailNotif(payload)
	if err != nil {
		logger.Warn(ctx, "Failed to send email", "error", err.Error())
		
		if retryCount >= 3 {
			logger.Error(ctx, "Max retries reached. Sending message to DLQ.", "id", msg.MessageId)
			// Pindahkan ke DLQ (requeue = false)
			msg.Nack(false, false) 
		} else {
			logger.Info(ctx, "Re-routing message to Retry Queue...", "id", msg.MessageId)
			// Publish ke Retry Queue secara manual atau Nack dengan skema DLX routing key khusus
			// Untuk versi simple tingkat junior: Nack(requeue=false) dengan DLX ter-binding ke retry queue
			msg.Nack(false, false)
		}
		return
	}

	msg.Ack(false)
}

// Helper untuk membaca headers x-death dari RabbitMQ
func getRetryCount(headers amqp.Table) int {
	if headers == nil {
		return 0
	}
	
	xDeath, exists := headers["x-death"]
	if !exists {
		return 0
	}
	
	deathList, ok := xDeath.([]any)
	if !ok || len(deathList) == 0 {
		return 0
	}
	
	// RabbitMQ menyimpan array objek x-death, panjang array mewakili total retry
	return len(deathList)
}
```

---

## ✅ Acceptance Criteria
* [ ] Antrean `notification.emails.dlq` terdaftar otomatis di dashboard RabbitMQ.
* [ ] Mengirimkan pesan JSON yang rusak/corrupt sukses membuat message langsung dipindahkan ke DLQ (antrean `notification.emails` tetap kosong dan bersih).
* [ ] Tim SysOps dapat memeriksa dan men-download isi payload message gagal di DLQ melalui RabbitMQ Web Management secara aman.

---

## 💡 Tips untuk Junior
* **Gunakan DLQ Secara Bijak:** Message di DLQ menumpuk tanpa batas karena tidak ada consumer yang membaca. Selalu set alert monitoring di production jika antrean DLQ memiliki jumlah pesan > 10, yang menandakan adanya bug software massal atau kegagalan pihak ketiga (SMTP provider mati total).

---

## 📚 Referensi Belajar
* [RabbitMQ Dead Letter Exchanges official guide](https://www.rabbitmq.com/dlx.html)
* [Designing retry queues with RabbitMQ TTL](https://www.cloudamqp.com/blog/rabbitmq-dead-letter-exchange-dlx.html)
