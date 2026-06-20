# Episode26: Memecah Transaction Service

## 🎯 Tujuan
* Membuat folder microservice baru bernama `transaction-service` di dalam monorepo kita.
* Mendaftarkan service baru ke dalam workspace `go.work`.
* Memindahkan domain **Transaction** dari monolith lama agar berjalan sebagai service mandiri di port HTTP `:8086`.

---

## 📐 Konsep Pemecahan Transaction Service
**Transaction Service** berfungsi sebagai orchestrator (sutradara) untuk memproses alur transfer dana:
1. **Tabel MySQL Dedikasi:** Mengelola tabel `transactions` untuk menyimpan status detail transaksi (pending, success, failed) dan melakukan validasi `idempotency_key` secara ketat.
2. **Koordinasi gRPC:** Saat menerima request transfer, service ini akan menghubungi:
   * **User Service** (via gRPC port 50052) untuk memvalidasi keberadaan email penerima.
   * **Wallet Service** (via gRPC port 50053) untuk mengecek dan mengupdate saldo.
   * **Ledger Service** (via gRPC port 50054) untuk mencatat pembukuan debit dan kredit.

```
/Users/ikhda/Documents/coding/golang/wallet-microservice/
├── go.work
├── api-gateway/         (Port 8080)
├── auth-service/        (Port 8081)
├── user-service/        (Port 8084)
├── wallet-service/      (Port 8082, gRPC 50053)
├── ledger-service/      (gRPC Port 50054)
└── transaction-service/ (Port 8086)  <-- Service Baru!
```

---

## 📦 Langkah-langkah

### Step 1: Membuat Folder transaction-service
Buat struktur folder untuk `transaction-service`:

```bash
mkdir -p transaction-service/cmd transaction-service/internal/{config,database,transaction}
```

Inisialisasi Go Module di dalam folder `transaction-service`:
```bash
cd transaction-service
go mod init github.com/emzhofb/gowallet/transaction-service
cd ..
```

Daftarkan service baru ini ke Go Workspace di file `go.work`:
```bash
go work use ./transaction-service
```

### Step 2: Salin Helper & Config
Salin file helper database (`database.go`), structured logging (`logger.go`), error handler (`errors.go`), config loader (`config.go`), serta middleware-middleware dari monolith ke folder `transaction-service/internal/` yang sesuai.

### Step 3: Pindahkan Domain Transaction
Pindahkan folder domain `transaction` (Model, Repository, dan Handler) dari monolith lama ke `transaction-service/internal/transaction/`.

Perbarui seluruh package import agar mengarah ke module path `transaction-service`:
```go
import "github.com/emzhofb/gowallet/transaction-service/internal/transaction/model"
```

### Step 4: Setup Entry Point `transaction-service/cmd/main.go`
Buat file `main.go` di `transaction-service/cmd/main.go` untuk menjalankan HTTP server di port `:8086`.

```go
package main

import (
	"log"

	"github.com/emzhofb/gowallet/transaction-service/internal/config"
	"github.com/emzhofb/gowallet/transaction-service/internal/database"
	"github.com/emzhofb/gowallet/transaction-service/internal/logger"
	"github.com/emzhofb/gowallet/transaction-service/internal/middleware"
	transactionHandler "github.com/emzhofb/gowallet/transaction-service/internal/transaction/handler"
	transactionRepository "github.com/emzhofb/gowallet/transaction-service/internal/transaction/repository"
	transactionService "github.com/emzhofb/gowallet/transaction-service/internal/transaction/service"
	"github.com/gin-gonic/gin"
)

func main() {
	logger.InitLogger()
	logger.Log.Info("Starting Transaction Microservice...")

	cfg := config.LoadConfig()

	db, err := database.ConnectWithRetry(cfg.DBDSN)
	if err != nil {
		log.Fatalf("Could not connect to MySQL: %v", err)
	}
	defer db.Close()

	txRepo := transactionRepository.NewMySQLTransactionRepository(db)
	
	// Service Layer (Akan disuntik gRPC clients di Episode27)
	txSvc := transactionService.NewTransactionService(db, txRepo)
	txHandler := transactionHandler.NewTransactionHandler(txSvc)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.ErrorHandler())

	// HTTP routes
	r.POST("/api/v1/transactions/transfer", txHandler.Transfer)
	r.GET("/api/v1/transactions/history", txHandler.GetHistory)

	logger.Log.Info("Transaction Service listening on port 8086...")
	if err := r.Run(":8086"); err != nil {
		log.Fatalf("Transaction Service failed: %v", err)
	}
}
```

---

## ✅ Acceptance Criteria
* [ ] Folder `transaction-service` terdaftar di workspace `go.work`.
* [ ] Service `transaction-service` berjalan lancar di port HTTP `8086`.
* [ ] Database `gowallet` MySQL terkoneksi dan siap merekam data transaksi.

---

## 💡 Tips untuk Junior
* **Idempotency Key di level Transaction Service:** Validasi `idempotency_key` wajib dilakukan pertama kali di level entry point transaksi sebelum memproses logika lainnya agar tidak terjadi double-spending.
