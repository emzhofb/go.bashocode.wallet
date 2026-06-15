# Episode 25: Payment Service (External Integration & Webhook HMAC)

## 🎯 Tujuan
* Membuat microservice baru bernama `payment-service` untuk menangani integrasi payment gateway luar (seperti Stripe atau Midtrans).
* Mendaftarkan service baru ke workspace `go.work`.
* Mengimplementasikan endpoint webhook `/api/v1/payments/callback` untuk menerima notifikasi pembayaran sukses dari payment gateway.
* Mengamankan webhook menggunakan **HMAC Signature Verification** (validasi tanda tangan biner) untuk mencegah peretas menembak API callback secara ilegal.
* Mem-publish event `payment.completed` ke RabbitMQ ketika pembayaran sukses terverifikasi.

---

## 📐 Konsep Keamanan Webhook dengan HMAC
Saat pengguna melakukan top-up, mereka membayar di halaman luar milik Payment Gateway. Setelah pembayaran selesai, Payment Gateway akan memanggil API kita secara otomatis (*Webhook HTTP Callback*) untuk mengabari bahwa uang sudah didepositkan.
* **Tantangan Keamanan:** Endpoint callback ini bersifat publik. Bagaimana jika peretas menembak endpoint kita langsung (`POST /api/v1/payments/callback`) dengan body palsu seolah-olah mereka telah mentransfer uang?
* **Solusinya (HMAC):** Payment Gateway dan server kita memiliki kunci rahasia bersama (*Shared Secret Key*). 
  1. Sebelum mengirim callback, Payment Gateway membuat tanda tangan dengan me-hash request body menggunakan kunci rahasia tersebut:
     `Signature = HMAC-SHA256(SecretKey, RequestBody)`
  2. Gateway mengirimkan tanda tangan ini di header HTTP (misal `X-Callback-Signature`).
  3. Saat menerima callback, server kita menghitung ulang tanda tangan tersebut menggunakan kunci rahasia yang kita simpan di `.env`.
  4. Jika hasil kalkulasi kita cocok dengan header dari Gateway, maka request dijamin 100% otentik dari Payment Gateway asli.

```
[ Payment Gateway ] ➔ (Kirim JSON + Signature Header) ➔ [ Payment Service (Webhook) ]
                                                                │
                                                         (Kalkulasi HMAC)
                                                        /              \
                                                  (Sama / Valid)   (Berbeda / Invalid)
                                                        /                  \
                                                       ▼                    ▼
                                              [ Publish Event ]       [ Return 401/403 ]
                                            (payment.completed)
```

---

## 📦 Langkah-langkah

### Step 1: Membuat Folder payment-service
Buat struktur folder untuk `payment-service`:

```bash
mkdir -p payment-service/cmd payment-service/internal/{config,database,handler,service}
```

Inisialisasi Go Module di dalam folder `payment-service`:
```bash
cd payment-service
go mod init github.com/emzhofb/gowallet/payment-service
cd ..
```

Daftarkan service baru ini ke Go Workspace di file `go.work`:
```bash
go work use ./payment-service
```

### Step 2: Salin Modul Database & Config
Salin file helper database (`rabbitmq.go`), structured logging (`logger.go`), config loader (`config.go`), serta middleware-middleware dari service sebelumnya ke folder `payment-service/internal/` yang sesuai.

### Step 3: Membuat Model Payment Callback DTO
Buat file baru di `payment-service/internal/handler/model.go`:

```go
package handler

type PaymentGatewayCallback struct {
	OrderID        string  `json:"order_id"`
	UserID         string  `json:"user_id"`
	Amount         float64 `json:"amount"`
	PaymentStatus  string  `json:"payment_status"` // settled, pending, deny
	PaymentGateway string  `json:"payment_gateway"` // midtrans, stripe
}
```

### Step 4: Membuat Verifikator HMAC & Webhook Handler
Kita gunakan standard package `crypto/hmac` dan `crypto/sha256` bawaan Go untuk memvalidasi tanda tangan webhook.

Buat file baru di `payment-service/internal/handler/webhook.go`:

```go
package handler

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"

	customErr "github.com/emzhofb/gowallet/payment-service/internal/errors"
	"github.com/emzhofb/gowallet/payment-service/internal/logger"
	"github.com/gin-gonic/gin"
	amqp "github.com/rabbitmq/amqp091-go"
)

type PaymentHandler struct {
	amqpConn *amqp.Connection
	channel  *amqp.Channel
	secretKey []byte // HMAC shared secret
}

func NewPaymentHandler(conn *amqp.Connection, secret string) *PaymentHandler {
	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to open channel: %v", err)
	}

	return &PaymentHandler{
		amqpConn:  conn,
		channel:   ch,
		secretKey: []byte(secret),
	}
}

func (h *PaymentHandler) HandleWebhook(c *gin.Context) {
	// 1. Baca request body mentah (raw bytes) untuk kalkulasi HMAC
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.Error(customErr.ErrBadRequest)
		return
	}
	// Kembalikan body ke request context agar Gin bisa mem-bind JSON setelahnya
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// 2. Ambil signature dari header HTTP
	signatureHeader := c.GetHeader("X-Callback-Signature")
	if signatureHeader == "" {
		c.Error(customErr.NewAppError(http.StatusUnauthorized, "MISSING_SIGNATURE", "Signature header diperlukan."))
		c.Abort()
		return
	}

	// 3. Hitung signature HMAC-SHA256 lokal
	mac := hmac.New(sha256.New, h.secretKey)
	mac.Write(bodyBytes)
	expectedMac := mac.Sum(nil)
	expectedSignature := hex.EncodeToString(expectedMac)

	// 4. Bandingkan signature (Gunakan ConstantTimeCompare untuk menghindari timing attack)
	if !hmac.Equal([]byte(signatureHeader), []byte(expectedSignature)) {
		logger.Warn(c.Request.Context(), "Spoofed webhook request detected! Signature mismatch.")
		c.Error(customErr.NewAppError(http.StatusForbidden, "INVALID_SIGNATURE", "Tanda tangan request tidak valid."))
		c.Abort()
		return
	}

	// 5. Bind JSON ke model jika signature valid
	var callback PaymentGatewayCallback
	if err := c.ShouldBindJSON(&callback); err != nil {
		c.Error(customErr.NewAppError(http.StatusBadRequest, "INVALID_INPUT", err.Error()))
		return
	}

	// 6. Jika status pembayaran adalah "settled" (sukses terbayar)
	if callback.PaymentStatus == "settled" {
		logger.Info(c.Request.Context(), "Payment verified. Publishing event...", "order_id", callback.OrderID)
		
		// Rangkai event payload
		eventPayload, _ := json.Marshal(map[string]any{
			"user_id":   callback.UserID,
			"amount":    callback.Amount,
			"order_id":  callback.OrderID,
			"gateway":   callback.PaymentGateway,
		})

		// Publish event "payment.completed" ke RabbitMQ Exchange
		err = h.channel.PublishWithContext(
			c.Request.Context(),
			"wallet.events",      // exchange
			"payment.completed",  // routing key
			false,
			false,
			amqp.Publishing{
				ContentType: "application/json",
				Body:        eventPayload,
				MessageId:   callback.OrderID,
			},
		)
		if err != nil {
			logger.Error(c.Request.Context(), "Failed to publish payment.completed event", "error", err.Error())
			c.Error(customErr.ErrInternalServer)
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Webhook processed successfully.",
	})
}
```

### Step 5: Setup Entry Point `payment-service/cmd/main.go`
Koneksikan ke RabbitMQ, daftarkan handler webhook, dan jalankan server HTTP di port `:8083`.

```go
package main

import (
	"log"
	"os"

	"github.com/emzhofb/gowallet/payment-service/internal/config"
	"github.com/emzhofb/gowallet/payment-service/internal/database"
	"github.com/emzhofb/gowallet/payment-service/internal/handler"
	"github.com/emzhofb/gowallet/payment-service/internal/logger"
	"github.com/emzhofb/gowallet/payment-service/internal/middleware"
	"github.com/gin-gonic/gin"
)

func main() {
	logger.InitLogger()
	logger.Log.Info("Starting Payment Microservice...")

	cfg := config.LoadConfig()

	// Connect to RabbitMQ
	amqpConn, err := database.ConnectRabbitMQ(cfg.RabbitMQURL)
	if err != nil {
		log.Fatalf("Could not connect to RabbitMQ: %v", err)
	}
	defer amqpConn.Close()

	// Definisikan secret key webhook (baca dari env)
	webhookSecret := os.Getenv("WEBHOOK_SECRET_KEY")
	if webhookSecret == "" {
		webhookSecret = "super-secret-key-change-this"
	}

	paymentHandler := handler.NewPaymentHandler(amqpConn, webhookSecret)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.ErrorHandler())

	// Route Callback
	r.POST("/api/v1/payments/callback", paymentHandler.HandleWebhook)

	logger.Log.Info("Payment Service listening on port 8083...")
	if err := r.Run(":8083"); err != nil {
		log.Fatalf("Payment Service failed: %v", err)
	}
}
```

*Sesuaikan `api-gateway` agar merouting path `/api/v1/payments/*` ke port `:8083`.*

---

## ✅ Acceptance Criteria
* [ ] Folder `payment-service` terdaftar di workspace `go.work`.
* [ ] Menjalankan POST request ke `/api/v1/payments/callback` tanpa header `X-Callback-Signature` ditolak dengan status HTTP `401 Unauthorized`.
* [ ] Menjalankan request dengan payload dan signature HMAC yang sengaja dirusak mengembalikan HTTP `403 Forbidden` (`INVALID_SIGNATURE`).
* [ ] Menjalankan request dengan signature HMAC-SHA256 yang valid mengembalikan HTTP `200 OK` dan sukses mem-publish event `payment.completed` ke Exchange RabbitMQ.

---

## 💡 Tips untuk Junior
* **Constant Time Comparison (`hmac.Equal`):** Mengapa kita membandingkan signature menggunakan `hmac.Equal(a, b)` alih-alih operator string biasa `stringA == stringB`? Operator perbandingan string biasa di Go bersifat *fail-fast* (berhenti membandingkan karakter pada huruf pertama yang tidak cocok), sehingga durasi waktu proses perbandingan bisa dianalisis peretas untuk menebak token karakter demi karakter (**Timing Attack**). `hmac.Equal` menjamin waktu perbandingan selalu konstan berapa pun jumlah karakter yang cocok, menutup celah serangan tersebut.

---

## 📚 Referensi Belajar
* [What is HMAC and how it works](https://en.wikipedia.org/wiki/HMAC)
* [Preventing Webhook Timing Attacks](https://codahale.com/a-lesson-in-timing-attacks/)
