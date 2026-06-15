# Episode 18: Memecah Wallet & Ledger Service

## 🎯 Tujuan
* Membuat folder microservice baru bernama `wallet-service` di dalam monorepo.
* Mendaftarkan service baru ke dalam workspace `go.work`.
* Memindahkan domain `wallet` dan `ledger` dari monolith lama agar berjalan sebagai service mandiri di port `:8082`.
* Mengintegrasikan gRPC Client agar `wallet-service` bisa memanggil `auth-service` (port `:50051`) untuk memvalidasi user ID.

---

## 📐 Konsep Pemisahan Wallet & Ledger Service
Sekarang kita memisahkan fungsionalitas finansial (Wallet & Buku Besar Ledger) ke service tersendiri agar skalabilitasnya terpisah dari autentikasi user.

```
/Users/ikhda/Documents/coding/golang/wallet-microservice/
├── go.work
├── api-gateway/         (Port 8080)
├── auth-service/        (Port 8081, gRPC: 50051)
└── wallet-service/      (Port 8082)  <-- Service Baru!
```

---

## 📦 Langkah-langkah

### Step 1: Membuat Folder wallet-service
Buat struktur folder untuk `wallet-service`:

```bash
mkdir -p wallet-service/cmd wallet-service/internal/{config,database,wallet,ledger,transaction}
```

Inisialisasi Go Module di dalam folder `wallet-service`:
```bash
cd wallet-service
go mod init github.com/emzhofb/gowallet/wallet-service
cd ..
```

Daftarkan service baru ini ke Go Workspace di file `go.work`:
```bash
go work use ./wallet-service
```

### Step 2: Salin Modul Database & Config
Salin file helper database (`database.go` & `redis.go`), structured logging (`logger.go`), config loader (`config.go`), serta middleware-middleware dari monolith lama ke folder `wallet-service/internal/` yang sesuai.

### Step 3: Pindahkan Domain Wallet & Ledger & Transaction
Pindahkan folder domain `wallet`, `ledger`, dan `transaction` dari monolith lama ke `wallet-service/internal/`.

Perbarui seluruh package import di dalam file-file tersebut agar menggunakan nama module yang baru:
```go
// Ganti import lama
import "github.com/emzhofb/gowallet/monolith/internal/wallet/model"

// Menjadi import baru
import "github.com/emzhofb/gowallet/wallet-service/internal/wallet/model"
```

### Step 4: Menghubungkan gRPC Client ke Auth Service
Di dalam `wallet-service`, kita tidak memiliki database tabel `users` untuk memvalidasi email penerima transfer. Kita harus memanggil gRPC Server milik `auth-service`.

Buka file `wallet-service/internal/transaction/service/service.go`. 
1. Hapus ketergantungan `userRepo` dari parameter struct `transactionService`.
2. Ganti dengan gRPC Client interface `pb.UserServiceClient`:

```go
package service

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	pb "github.com/emzhofb/gowallet/auth-service/proto/user" // Import proto generated file
	customErr "github.com/emzhofb/gowallet/wallet-service/internal/errors"
	ledgerModel "github.com/emzhofb/gowallet/wallet-service/internal/ledger/model"
	ledgerRepo "github.com/emzhofb/gowallet/wallet-service/internal/ledger/repository"
	"github.com/emzhofb/gowallet/wallet-service/internal/transaction/model"
	"github.com/emzhofb/gowallet/wallet-service/internal/transaction/repository"
	walletRepo "github.com/emzhofb/gowallet/wallet-service/internal/wallet/repository"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type transactionService struct {
	db          *sql.DB
	txRepo      repository.TransactionRepository
	userClient  pb.UserServiceClient // gRPC Client
	walletRepo  walletRepo.WalletRepository
	ledgerRepo  ledgerRepo.LedgerRepository
	rdb         *redis.Client
}

func NewTransactionService(
	db *sql.DB,
	txRepo repository.TransactionRepository,
	userClient pb.UserServiceClient, // Suntik gRPC Client
	wRepo walletRepo.WalletRepository,
	lRepo ledgerRepo.LedgerRepository,
	rdb *redis.Client,
) TransactionService {
	return &transactionService{
		db:          db,
		txRepo:      txRepo,
		userClient:  userClient,
		walletRepo:  wRepo,
		ledgerRepo:  lRepo,
		rdb:         rdb,
	}
}

func (s *transactionService) Transfer(ctx context.Context, senderUserID string, req model.TransferRequest) (*model.Transaction, error) {
	// ... cek idempotency ...

	// 2. Hubungi Auth Service via gRPC untuk mencari User Penerima berdasarkan Email
	receiverUser, err := s.userClient.GetUserByEmail(ctx, &pb.GetUserByEmailRequest{Email: req.ReceiverEmail})
	if err != nil {
		// Jika gRPC me-return error NotFound
		return nil, customErr.NewAppError(http.StatusNotFound, "RECEIVER_NOT_FOUND", "Email penerima tidak ditemukan.")
	}

	// 3. Mulai Transaksi Database ...
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, customErr.ErrInternalServer
	}
	defer tx.Rollback()

	// ... (proses persis sama, cukup gunakan receiverUser.GetId() untuk menggantikan receiverUser.ID)
```

### Step 5: Setup Entry Point `wallet-service/cmd/main.go`
Koneksikan ke gRPC server `auth-service` di port `50051`, buat instance gRPC Client, lalu suntikkan ke `transactionService`.

```go
package main

import (
	"log"

	pb "github.com/emzhofb/gowallet/auth-service/proto/user" // Import proto generated file
	"github.com/emzhofb/gowallet/wallet-service/internal/config"
	"github.com/emzhofb/gowallet/wallet-service/internal/database"
	"github.com/emzhofb/gowallet/wallet-service/internal/logger"
	"github.com/emzhofb/gowallet/wallet-service/internal/middleware"
	ledgerRepository "github.com/emzhofb/gowallet/wallet-service/internal/ledger/repository"
	transactionHandler "github.com/emzhofb/gowallet/wallet-service/internal/transaction/handler"
	transactionRepository "github.com/emzhofb/gowallet/wallet-service/internal/transaction/repository"
	transactionService "github.com/emzhofb/gowallet/wallet-service/internal/transaction/service"
	walletHandler "github.com/emzhofb/gowallet/wallet-service/internal/wallet/handler"
	walletRepository "github.com/emzhofb/gowallet/wallet-service/internal/wallet/repository"
	walletService "github.com/emzhofb/gowallet/wallet-service/internal/wallet/service"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	logger.InitLogger()
	logger.Log.Info("Starting Wallet Microservice...")

	cfg := config.LoadConfig()

	db, err := database.ConnectWithRetry(cfg.DBDSN)
	if err != nil {
		log.Fatalf("Could not connect to MySQL: %v", err)
	}
	defer db.Close()

	rdb, err := database.ConnectRedis(cfg.RedisAddr)
	if err != nil {
		log.Fatalf("Could not connect to Redis: %v", err)
	}
	defer rdb.Close()

	// 1. Koneksi ke gRPC Server Auth Service
	conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Could not connect to gRPC server: %v", err)
	}
	defer conn.Close()
	userClient := pb.NewUserServiceClient(conn)

	// 2. Inisialisasi layer
	wRepo := walletRepository.NewMySQLWalletRepository(db)
	lRepo := ledgerRepository.NewMySQLLedgerRepository(db)
	txRepo := transactionRepository.NewMySQLTransactionRepository(db)

	wSvc := walletService.NewWalletService(wRepo, rdb)
	txSvc := transactionService.NewTransactionService(db, txRepo, userClient, wRepo, lRepo, rdb) // Inject gRPC Client

	wHandler := walletHandler.NewWalletHandler(wSvc)
	txHandler := transactionHandler.NewTransactionHandler(txSvc)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.ErrorHandler())
	
	// Middleware auth tetap digunakan di tingkat microservice untuk memvalidasi jwt token
	r.Use(middleware.AuthMiddleware(rdb))

	// Routes
	r.GET("/api/v1/wallets/me", wHandler.GetMyWallet)
	r.POST("/api/v1/transactions/transfer", txHandler.Transfer)
	r.GET("/api/v1/transactions/history", txHandler.GetHistory)

	logger.Log.Info("Wallet Service listening on port 8082...")
	if err := r.Run(":8082"); err != nil {
		log.Fatalf("Wallet Service failed: %v", err)
	}
}
```

---

## ✅ Acceptance Criteria
* [ ] Folder `wallet-service` terdaftar di workspace `go.work`.
* [ ] Menjalankan `go run wallet-service/cmd/main.go` sukses mendeteksi dan mengoneksikan gRPC client ke `auth-service` di port `50051`.
* [ ] Memanggil endpoint transfer uang `/api/v1/transactions/transfer` lewat API Gateway sukses memvalidasi email penerima via gRPC dan memproses mutasi saldo di database dengan benar.

---

## 💡 Tips untuk Junior
* **Shared Proto files:** Di Monorepo, jika domain proto yang digenerate diimport oleh service lain, pastikan modul dependensi terdaftar dengan benar di `go.work`. Kita mengimpor package generated `pb` dari `github.com/emzhofb/gowallet/auth-service/proto/user` secara langsung berkat Workspace mode yang menyatukan kompilasi file lokal.

---

## 📚 Referensi Belajar
* [Microservices communication patterns overview](https://learn.microsoft.com/en-us/azure/architecture/microservices/design/interservice-communication)
