# Episode 23: Memecah Wallet Service

## 🎯 Tujuan
* Membuat folder microservice baru bernama `wallet-service` di dalam monorepo kita.
* Mendaftarkan service baru ke dalam workspace `go.work`.
* Memindahkan domain **Wallet** dari monolith lama agar berjalan sebagai service mandiri di port HTTP `:8082` dan gRPC `:50053`.

---

## 📐 Konsep Pemecahan Wallet Service
**Wallet Service** memegang kendali penuh atas saldo dan informasi dompet digital pengguna:
1. **Tabel MySQL Dedikasi:** Mengelola tabel `wallets`. Semua query pembaruan saldo menggunakan optimistic locking dijalankan di sini.
2. **gRPC Server (Port 50053):** Menyediakan rpc method agar Transaction Service dapat memeriksa saldo pengirim sebelum transfer, dan memperbarui saldo pengirim/penerima secara terdistribusi.

```
/Users/ikhda/Documents/coding/golang/wallet-microservice/
├── go.work
├── api-gateway/         (Port 8080)
├── auth-service/        (Port 8081)
├── user-service/        (Port 8084)
└── wallet-service/      (Port 8082, gRPC 50053)  <-- Service Baru!
```

---

## 📦 Langkah-langkah

### Step 1: Membuat Folder wallet-service
Buat struktur folder untuk `wallet-service`:

```bash
mkdir -p wallet-service/cmd wallet-service/internal/{config,database,wallet}
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

### Step 2: Membuat Definisi Protobuf (`proto/wallet/wallet.proto`)
Buat folder baru bernama `proto/wallet/` di root folder workspace:
```bash
mkdir -p proto/wallet
```

Buat file `proto/wallet/wallet.proto`:

```protobuf
syntax = "proto3";

package wallet;

option go_package = "github.com/emzhofb/gowallet/wallet-service/proto/wallet";

service WalletService {
  rpc GetWalletByUserID (GetWalletRequest) returns (WalletResponse);
  rpc UpdateWalletBalance (UpdateBalanceRequest) returns (WalletResponse);
}

message GetWalletRequest {
  string user_id = 1;
}

message UpdateBalanceRequest {
  string user_id = 2;
  double amount = 3;       // Positif untuk kredit, Negatif untuk debit
  int32 expected_version = 4; // Optimistic locking version check
}

message WalletResponse {
  string id = 1;
  string user_id = 2;
  double balance = 3;
  int32 version = 4;
}
```

Compile file proto:
```bash
protoc --go_out=. --go-grpc_out=. proto/wallet/wallet.proto
```
Perintah ini akan men-generate folder target `wallet-service/proto/wallet/`.

### Step 3: Implementasi gRPC Server di Wallet Service
Buat file baru di `wallet-service/internal/wallet/grpc/server.go`:

```go
package grpc

import (
	"context"

	"github.com/emzhofb/gowallet/wallet-service/internal/wallet/repository"
	pb "github.com/emzhofb/gowallet/wallet-service/proto/wallet"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type walletGRPCServer struct {
	pb.UnimplementedWalletServiceServer
	repo repository.WalletRepository
}

func NewWalletGRPCServer(repo repository.WalletRepository) pb.WalletServiceServer {
	return &walletGRPCServer{repo: repo}
}

func (s *walletGRPCServer) GetWalletByUserID(ctx context.Context, req *pb.GetWalletRequest) (*pb.WalletResponse, error) {
	w, err := s.repo.GetByUserID(ctx, req.GetUserId())
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "wallet not found: %v", err)
	}

	return &pb.WalletResponse{
		Id:      w.ID,
		UserId:  w.UserID,
		Balance: w.Balance,
		Version: w.Version,
	}, nil
}

func (s *walletGRPCServer) UpdateWalletBalance(ctx context.Context, req *pb.UpdateBalanceRequest) (*pb.WalletResponse, error) {
	// Lakukan update saldo menggunakan Optimistic Locking di repository
	w, err := s.repo.UpdateBalanceWithOwnerCheck(ctx, req.GetUserId(), req.GetAmount(), req.GetExpectedVersion())
	if err != nil {
		return nil, status.Errorf(codes.Aborted, "concurrent update failure or insufficient balance: %v", err)
	}

	return &pb.WalletResponse{
		Id:      w.ID,
		UserId:  w.UserID,
		Balance: w.Balance,
		Version: w.Version,
	}, nil
}
```

### Step 4: Setup Entry Point `wallet-service/cmd/main.go`
Jalankan gRPC server di port `:50053` dan HTTP server di port `:8082`.

```go
package main

import (
	"log"
	"net"

	"github.com/emzhofb/gowallet/wallet-service/internal/config"
	"github.com/emzhofb/gowallet/wallet-service/internal/database"
	"github.com/emzhofb/gowallet/wallet-service/internal/logger"
	walletGRPC "github.com/emzhofb/gowallet/wallet-service/internal/wallet/grpc"
	walletRepository "github.com/emzhofb/gowallet/wallet-service/internal/wallet/repository"
	pb "github.com/emzhofb/gowallet/wallet-service/proto/wallet"
	"google.golang.org/grpc"
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

	wRepo := walletRepository.NewMySQLWalletRepository(db)

	// Setup gRPC Server
	lis, err := net.Listen("tcp", ":50053")
	if err != nil {
		log.Fatalf("Failed to listen gRPC: %v", err)
	}
	
	grpcServer := grpc.NewServer()
	pb.RegisterWalletServiceServer(grpcServer, walletGRPC.NewWalletGRPCServer(wRepo))

	go func() {
		logger.Log.Info("Wallet gRPC Server running on port 50053...")
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve gRPC: %v", err)
		}
	}()

	// (HTTP routes untuk check balance dari client luar diport 8082...)
}
```

---

## ✅ Acceptance Criteria
* [ ] Folder `wallet-service` terdaftar di workspace `go.work`.
* [ ] gRPC server `wallet-service` aktif mendengarkan port `50053`.
* [ ] Memanggil RPC `GetWalletByUserID` berhasil mengembalikan data wallet beserta kolom `version`.

---

## 💡 Tips untuk Junior
* **Optimistic Locking via gRPC:** Mengirimkan parameter `expected_version` lewat RPC memastikan verifikasi perlindungan perlombaan data (*race condition protection*) tetap terjaga saat transaksi dijalankan secara terdistribusi.
