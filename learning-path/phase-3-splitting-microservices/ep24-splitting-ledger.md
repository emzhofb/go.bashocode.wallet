# Episode 24: Memecah Ledger Service

## 🎯 Tujuan
* Membuat folder microservice baru bernama `ledger-service` di dalam monorepo kita.
* Mendaftarkan service baru ke dalam workspace `go.work`.
* Memindahkan domain **Ledger** dari monolith lama agar berjalan sebagai service mandiri di port gRPC `:50054`.

---

## 📐 Konsep Pemecahan Ledger Service
**Ledger Service** bertindak sebagai buku kas besar akuntansi finansial yang mencatat setiap aliran debit/kredit secara permanen (immutable):
1. **Tabel MySQL Dedikasi:** Service ini memegang kontrol eksklusif atas tabel `ledger_entries`. Semua catatan riwayat mutasi berada di sini.
2. **gRPC Server (Port 50054):** Menyediakan RPC method agar Transaction Service dapat mencatat transaksi kredit dan debit setelah proses transfer divalidasi.

```
/Users/ikhda/Documents/coding/golang/wallet-microservice/
├── go.work
├── api-gateway/         (Port 8080)
├── auth-service/        (Port 8081)
├── user-service/        (Port 8084)
├── wallet-service/      (Port 8082, gRPC 50053)
└── ledger-service/      (gRPC Port 50054)  <-- Service Baru!
```

---

## 📦 Langkah-langkah

### Step 1: Membuat Folder ledger-service
Buat struktur folder untuk `ledger-service`:

```bash
mkdir -p ledger-service/cmd ledger-service/internal/{config,database,ledger}
```

Inisialisasi Go Module di dalam folder `ledger-service`:
```bash
cd ledger-service
go mod init github.com/emzhofb/gowallet/ledger-service
cd ..
```

Daftarkan service baru ini ke Go Workspace di file `go.work`:
```bash
go work use ./ledger-service
```

### Step 2: Membuat Definisi Protobuf (`proto/ledger/ledger.proto`)
Buat folder baru bernama `proto/ledger/` di root folder workspace:
```bash
mkdir -p proto/ledger
```

Buat file `proto/ledger/ledger.proto`:

```protobuf
syntax = "proto3";

package ledger;

option go_package = "github.com/emzhofb/gowallet/ledger-service/proto/ledger";

service LedgerService {
  rpc RecordLedgerEntry (RecordEntryRequest) returns (RecordEntryResponse);
  rpc GetBalanceFromLedger (GetBalanceRequest) returns (BalanceResponse);
}

message RecordEntryRequest {
  string transaction_id = 1;
  string wallet_id = 2;
  string type = 3;       // DEBIT atau CREDIT
  double amount = 4;
}

message RecordEntryResponse {
  string entry_id = 1;
  bool success = 2;
}

message GetBalanceRequest {
  string wallet_id = 1;
}

message BalanceResponse {
  double calculated_balance = 1;
}
```

Compile file proto:
```bash
protoc --go_out=. --go-grpc_out=. proto/ledger/ledger.proto
```
Perintah ini akan men-generate folder target `ledger-service/proto/ledger/`.

### Step 3: Implementasi gRPC Server di Ledger Service
Buat file baru di `ledger-service/internal/ledger/grpc/server.go`:

```go
package grpc

import (
	"context"

	"github.com/emzhofb/gowallet/ledger-service/internal/ledger/model"
	"github.com/emzhofb/gowallet/ledger-service/internal/ledger/repository"
	pb "github.com/emzhofb/gowallet/ledger-service/proto/ledger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ledgerGRPCServer struct {
	pb.UnimplementedLedgerServiceServer
	repo repository.LedgerRepository
}

func NewLedgerGRPCServer(repo repository.LedgerRepository) pb.LedgerServiceServer {
	return &ledgerGRPCServer{repo: repo}
}

func (s *ledgerGRPCServer) RecordLedgerEntry(ctx context.Context, req *pb.RecordEntryRequest) (*pb.RecordEntryResponse, error) {
	entry := &model.LedgerEntry{
		TransactionID: req.GetTransactionId(),
		WalletID:      req.GetWalletId(),
		Type:          req.GetType(),
		Amount:         req.GetAmount(),
	}

	err := s.repo.Create(ctx, entry)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to record ledger entry: %v", err)
	}

	return &pb.RecordEntryResponse{
		EntryId: entry.ID,
		Success: true,
	}, nil
}

func (s *ledgerGRPCServer) GetBalanceFromLedger(ctx context.Context, req *pb.GetBalanceRequest) (*pb.BalanceResponse, error) {
	balance, err := s.repo.GetBalanceByWalletID(ctx, req.GetWalletId())
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "failed to calculate balance: %v", err)
	}

	return &pb.BalanceResponse{
		CalculatedBalance: balance,
	}, nil
}
```

### Step 4: Setup Entry Point `ledger-service/cmd/main.go`
Jalankan gRPC server di port `:50054`.

```go
package main

import (
	"log"
	"net"

	"github.com/emzhofb/gowallet/ledger-service/internal/config"
	"github.com/emzhofb/gowallet/ledger-service/internal/database"
	"github.com/emzhofb/gowallet/ledger-service/internal/logger"
	ledgerGRPC "github.com/emzhofb/gowallet/ledger-service/internal/ledger/grpc"
	ledgerRepository "github.com/emzhofb/gowallet/ledger-service/internal/ledger/repository"
	pb "github.com/emzhofb/gowallet/ledger-service/proto/ledger"
	"google.golang.org/grpc"
)

func main() {
	logger.InitLogger()
	logger.Log.Info("Starting Ledger Microservice...")

	cfg := config.LoadConfig()

	db, err := database.ConnectWithRetry(cfg.DBDSN)
	if err != nil {
		log.Fatalf("Could not connect to MySQL: %v", err)
	}
	defer db.Close()

	lRepo := ledgerRepository.NewMySQLLedgerRepository(db)

	// Setup gRPC Server
	lis, err := net.Listen("tcp", ":50054")
	if err != nil {
		log.Fatalf("Failed to listen gRPC: %v", err)
	}
	
	grpcServer := grpc.NewServer()
	pb.RegisterLedgerServiceServer(grpcServer, ledgerGRPC.NewLedgerGRPCServer(lRepo))

	logger.Log.Info("Ledger gRPC Server running on port 50054...")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve gRPC: %v", err)
	}
}
```

---

## ✅ Acceptance Criteria
* [ ] Folder `ledger-service` terdaftar di workspace `go.work`.
* [ ] gRPC server `ledger-service` aktif mendengarkan port `50054`.
* [ ] Method `RecordLedgerEntry` dapat diakses dan berhasil merekam entry debit/kredit ke tabel MySQL `ledger_entries`.

---

## 💡 Tips untuk Junior
* **Immutable Ledger Entries:** Ledger entries didesain *read-only* setelah ditulis. Data ledger yang sudah disimpan tidak boleh diupdate atau di-soft delete agar jejak audit transaksi tetap otentik.
