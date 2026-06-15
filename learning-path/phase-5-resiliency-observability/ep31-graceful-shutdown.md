# Episode 31: Graceful Shutdown di Microservices

## 🎯 Tujuan
* Memahami bahaya mematikan aplikasi secara paksa (*abrupt termination* / `kill -9`).
* Menangkap sinyal sistem operasi **SIGINT** (Ctrl+C) dan **SIGTERM** (sinyal stop dari Docker/Kubernetes).
* Menghentikan server HTTP Gin, gRPC server, koneksi database, dan consumer RabbitMQ secara teratur (*Graceful Shutdown*).

---

## 📐 Mengapa Butuh Graceful Shutdown?
Jika server kita sedang sibuk memproses transaksi transfer uang, lalu kita langsung mematikan aplikasi dengan memutus daya atau menghentikan container Docker secara kasar:
* Transaksi yang sedang berlangsung terputus di tengah jalan, berisiko merusak integritas saldo.
* Koneksi TCP ke database MySQL, Redis, dan RabbitMQ terputus tanpa penutupan (*flushing*) yang bersih, menyebabkan database mendeteksi koneksi menggantung (*zombie connections*).

**Graceful Shutdown** memastikan:
1. Aplikasi berhenti menerima request HTTP/gRPC baru.
2. Aplikasi menyelesaikan sisa request yang sedang berjalan (*in-flight requests*).
3. Background workers (seperti outbox worker) menyelesaikan tugas putaran terakhirnya.
4. Menutup koneksi database dan Redis secara bersih.
5. Mematikan aplikasi secara aman.

---

## 📦 Langkah-langkah

### Step 1: Menangkap Sinyal OS
Kita menggunakan channel dan package `os/signal` bawaan Go untuk mendeteksi kapan aplikasi diminta mati oleh sistem operasi.

Pola dasar menangkap sinyal:
```go
quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

// Kode di sini akan memblokir eksekusi sampai ada sinyal masuk
<-quit
```

### Step 1.5: Membuat Health, Readiness, & Liveness Checks
Dalam arsitektur microservices modern (seperti saat dideploy di Kubernetes), orkestrator perlu tahu status internal service kita untuk merouting traffic secara pintar:
* **Liveness Check (`/live`):** Menunjukkan apakah kontainer aplikasi masih hidup. Jika endpoint ini mengembalikan error (misal karena program deadlock), Kubernetes akan merestart kontainer ini.
* **Readiness Check (`/ready`):** Menunjukkan apakah aplikasi siap menerima traffic (koneksi database, redis, dan RabbitMQ sudah tersambung). Jika down, gateway tidak akan mengirim traffic ke kontainer ini.
* **Health Check (`/health`):** Endpoint umum untuk melaporkan status kesehatan sistem secara keseluruhan.

### Step 2: Implementasi Graceful Shutdown & Health Checks pada Server HTTP Gin
Buka file `wallet-service/cmd/main.go`. Ubah cara menjalankan server HTTP Gin dari `r.Run()` biasa menjadi ter-kustomisasi menggunakan `http.Server` dan daftarkan endpoint monitoring di router.

```go
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/emzhofb/gowallet/wallet-service/internal/config"
	"github.com/emzhofb/gowallet/wallet-service/internal/database"
	"github.com/emzhofb/gowallet/wallet-service/internal/logger"
	// ... import lainnya ...
)

func main() {
	logger.InitLogger()
	cfg := config.LoadConfig()

	// ... inisialisasi koneksi db, redis, rabbitmq ...
    
	r := gin.New()
	
	// Daftarkan Endpoint Health, Readiness, & Liveness
	r.GET("/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "UP"})
	})

	r.GET("/ready", func(c *gin.Context) {
		// Pastikan MySQL terkoneksi dengan baik
		if err := db.PingContext(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "DOWN", "reason": "MySQL database not responding"})
			return
		}
		// Pastikan Redis terkoneksi dengan baik
		if err := rdb.Ping(c.Request.Context()).Err(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "DOWN", "reason": "Redis cache not responding"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "READY"})
	})

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "HEALTHY"})
	})

	// ... register business routes ...

	// 1. Definisikan http.Server secara custom
	srv := &http.Server{
		Addr:    ":8082",
		Handler: r,
	}

	// 2. Jalankan HTTP server di latar belakang (goroutine)
	go func() {
		logger.Log.Info("Server listening on port 8082...")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server listen failed: %v", err)
		}
	}()

	// 3. Menunggu sinyal shutdown dari OS (Ctrl+C atau Docker stop)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	
	logger.Log.Info("Shutdown signal received. Starting graceful shutdown...")

	// 4. Set batas waktu tunggu toleransi penyelesaian request (misal 15 detik)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Hentikan penerimaan request HTTP baru & tunggu request berjalan selesai
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error(ctx, "HTTP Server forced to shutdown", "error", err.Error())
	} else {
		logger.Log.Info("HTTP Server closed cleanly.")
	}

	// 5. Bersihkan seluruh koneksi resource lainnya
	logger.Log.Info("Closing database and message broker connections...")
	
	// Hentikan background worker
	cancelWorker() // Context cancel untuk outbox worker
	
	// Close RabbitMQ Connection
	if err := amqpConn.Close(); err != nil {
		logger.Error(ctx, "Failed to close RabbitMQ connection", "error", err.Error())
	}
	
	// Close Redis Client
	if err := rdb.Close(); err != nil {
		logger.Error(ctx, "Failed to close Redis client", "error", err.Error())
	}
	
	// Close MySQL Connection
	if err := db.Close(); err != nil {
		logger.Error(ctx, "Failed to close MySQL connection", "error", err.Error())
	}

	logger.Log.Info("Wallet Microservice successfully stopped.")
}
```

---

## ✅ Acceptance Criteria
* [ ] Menjalankan service lalu mengirim sinyal stop (menekan `Ctrl+C` di terminal) sukses memicu log `Shutdown signal received...`.
* [ ] Terminal menampilkan log penutupan bersih bertahap:
  ```
  HTTP Server closed cleanly.
  Closing database and message broker connections...
  Wallet Microservice successfully stopped.
  ```
* [ ] Sisa request HTTP yang sedang berjalan saat sinyal diterima tetap selesai diproses dan tidak terputus paksa.
* [ ] Endpoint `GET /health`, `GET /live`, dan `GET /ready` berhasil diakses lewat browser/Postman dan mengembalikan JSON status dengan HTTP Status 200 saat server MySQL & Redis dalam keadaan menyala.
* [ ] Mematikan MySQL/Redis container secara sengaja menyebabkan `GET /ready` mengembalikan respons `503 Service Unavailable` dengan penjelasan kegagalan koneksi.

---

## 💡 Tips untuk Junior
* **SIGKILL (-9) vs SIGTERM (-15):** Di terminal Linux, perintah `kill -15 <pid>` (SIGTERM) meminta aplikasi berhenti secara sopan, memberi waktu bagi program untuk melakukan pembersihan (menjalankan rutin graceful shutdown kita). Sedangkan `kill -9 <pid>` (SIGKILL) langsung memotong proses secara keras di level kernel OS tanpa memberi kesempatan program bernafas. Kubernetes dan Docker selalu mengirim `SIGTERM` terlebih dahulu saat mematikan container.

---

## 📚 Referensi Belajar
* [Graceful Shutdown in Go http.Server](https://pkg.go.dev/net/http#Server.Shutdown)
* [Handling OS Signals in Go (Example)](https://gobyexample.com/signals)
