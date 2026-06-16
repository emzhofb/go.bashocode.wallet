# Episode 27: Memecah Scheduler Service

## 🎯 Tujuan
* Membuat folder microservice baru bernama `scheduler-service` di dalam monorepo kita.
* Mendaftarkan service baru ke dalam workspace `go.work`.
* Memisahkan logika background jobs (dari Episode 14) menjadi daemon service mandiri.
* Menerapkan prinsip **Database-per-Service Boundaries** dengan mengoordinasikan tugas cron melalui pemanggilan **gRPC internal** ke service pemilik domain, alih-alih melakukan query database langsung ke database service lain.

---

## 📐 Konsep Scheduler di Arsitektur Microservices
Di fase monolith (Episode 14), background scheduler berjalan di dalam aplikasi utama dan memiliki akses langsung ke database serta repository domain lain.
Dalam arsitektur microservices yang menggunakan prinsip **Database-per-Service**, ini adalah anti-pattern yang fatal. `scheduler-service` tidak boleh menyentuh database milik `auth-service`, `wallet-service`, atau `transaction-service` secara langsung.
* **Solusinya (gRPC Orchestrated Scheduler):** `scheduler-service` bertindak sebagai pemicu (*trigger*) mandiri yang ringan. Ketika waktu cron tercapai, ia memicu aksi di microservices lain melalui protokol gRPC yang aman dan cepat.

```
                  ┌───────────────┐
                  │   Scheduler   │
                  │    Service    │
                  └───────┬───────┘
          (Trigger Jobs via gRPC Protocols)
          /               │               \
         ▼                ▼                ▼
┌────────────────┐ ┌──────────────┐ ┌─────────────────────┐
│  Auth Service  │ │ Wallet /     │ │ Transaction Service │
│  (Cleanup OTP) │ │ Ledger Svc  │ │ (Generate CSV)      │
└────────────────┘ └──────────────┘ └─────────────────────┘
```

---

## 📦 Langkah-langkah

### Step 1: Membuat Folder & Modul `scheduler-service`
Buat struktur direktori untuk scheduler microservice:
```bash
mkdir -p scheduler-service/cmd scheduler-service/internal/{config,database,scheduler}
```

Inisialisasi Go Module di folder `scheduler-service`:
```bash
cd scheduler-service
go mod init github.com/emzhofb/gowallet/scheduler-service
cd ..
```

Daftarkan folder baru ini ke Go Workspace di file `go.work`:
```bash
go work use ./scheduler-service
```

### Step 2: Salin Logger & Config Helper
Salin file `logger.go` dan `config.go` dari service sebelumnya ke folder `scheduler-service/internal/` yang sesuai.

### Step 3: Membuat Scheduler Go Client (`internal/scheduler/scheduler.go`)
`scheduler-service` akan terhubung ke gRPC server dari microservices lain untuk memicu tugas-tugas maintenance.

Buat file baru di `scheduler-service/internal/scheduler/scheduler.go`:
```go
package scheduler

import (
	"context"
	"time"

	"github.com/emzhofb/gowallet/scheduler-service/internal/logger"
	authPb "github.com/emzhofb/gowallet/auth-service/proto/auth"
	walletPb "github.com/emzhofb/gowallet/wallet-service/proto/wallet"
	txPb "github.com/emzhofb/gowallet/transaction-service/proto/transaction"
	"github.com/robfig/cron/v3"
)

type Scheduler struct {
	cron       *cron.Cron
	authClient authPb.AuthServiceClient
	walletClient walletPb.WalletServiceClient
	txClient   txPb.TransactionServiceClient
}

func NewScheduler(
	authClient authPb.AuthServiceClient,
	walletClient walletPb.WalletServiceClient,
	txClient txPb.TransactionServiceClient,
) *Scheduler {
	c := cron.New(cron.WithSeconds())
	return &Scheduler{
		cron:         c,
		authClient:   authClient,
		walletClient: walletClient,
		txClient:     txClient,
	}
}

func (s *Scheduler) Start() {
	// 1. Job 1: Bersihkan token OTP expired setiap 30 menit
	s.cron.AddFunc("0 */30 * * * *", s.TriggerOTPCleanup)

	// 2. Job 2: Rekonsiliasi Saldo Harian setiap jam 02:00 pagi
	s.cron.AddFunc("0 0 2 * * *", s.TriggerBalanceReconciliation)

	// 3. Job 3: Bersihkan Refresh Token expired setiap hari pada jam 03:00 pagi
	s.cron.AddFunc("0 0 3 * * *", s.TriggerRefreshTokenCleanup)

	// 4. Job 4: Ekspor laporan transaksi harian setiap hari pada jam 23:59 malam
	s.cron.AddFunc("0 59 23 * * *", s.TriggerDailyReportGeneration)

	s.cron.Start()
	logger.Log.Info("Centralized Scheduler Service started successfully!")
}

func (s *Scheduler) Stop() {
	s.cron.Stop()
	logger.Log.Info("Centralized Scheduler Service stopped.")
}
```

### Step 4: Membuat Trigger Logic (`internal/scheduler/jobs.go`)
Fungsi trigger ini memanggil endpoint gRPC internal di masing-masing service.

Buat file baru di `scheduler-service/internal/scheduler/jobs.go`:
```go
package scheduler

import (
	"context"
	"time"

	"github.com/emzhofb/gowallet/scheduler-service/internal/logger"
	authPb "github.com/emzhofb/gowallet/auth-service/proto/auth"
	walletPb "github.com/emzhofb/gowallet/wallet-service/proto/wallet"
	txPb "github.com/emzhofb/gowallet/transaction-service/proto/transaction"
)

func (s *Scheduler) TriggerOTPCleanup() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger.Log.InfoContext(ctx, "[Cron Job] Triggering expired OTP cleanup via Auth gRPC...")
	_, err := s.authClient.CleanupExpiredOTPs(ctx, &authPb.CleanupRequest{})
	if err != nil {
		logger.Error(ctx, "Failed to cleanup expired OTPs", "error", err.Error())
		return
	}
	logger.Log.InfoContext(ctx, "[Cron Job] Expired OTP cleanup successfully triggered.")
}

func (s *Scheduler) TriggerRefreshTokenCleanup() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger.Log.InfoContext(ctx, "[Cron Job] Triggering expired Refresh Token cleanup via Auth gRPC...")
	_, err := s.authClient.CleanupExpiredRefreshTokens(ctx, &authPb.CleanupRequest{})
	if err != nil {
		logger.Error(ctx, "Failed to cleanup expired Refresh Tokens", "error", err.Error())
		return
	}
	logger.Log.InfoContext(ctx, "[Cron Job] Expired Refresh Token cleanup successfully triggered.")
}

func (s *Scheduler) TriggerBalanceReconciliation() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	logger.Log.InfoContext(ctx, "[Cron Job] Triggering Balance Reconciliation Audit via Wallet gRPC...")
	res, err := s.walletClient.ReconcileBalances(ctx, &walletPb.ReconcileRequest{})
	if err != nil {
		logger.Error(ctx, "Failed to reconcile balances", "error", err.Error())
		return
	}
	logger.Log.InfoContext(ctx, "[Cron Job] Balance Reconciliation completed.", "mismatches_found", res.MismatchCount)
}

func (s *Scheduler) TriggerDailyReportGeneration() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	logger.Log.InfoContext(ctx, "[Cron Job] Triggering Daily Report Generation via Transaction gRPC...")
	_, err := s.txClient.GenerateDailyReport(ctx, &txPb.ReportRequest{})
	if err != nil {
		logger.Error(ctx, "Failed to trigger daily report", "error", err.Error())
		return
	}
	logger.Log.InfoContext(ctx, "[Cron Job] Daily report generation successfully triggered.")
}
```

### Step 5: Setup Entry Point `scheduler-service/cmd/main.go`
Buat file `main.go` di `scheduler-service/cmd/main.go` untuk menyambungkan semua gRPC clients dan menjalankan scheduler:

```go
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/emzhofb/gowallet/scheduler-service/internal/config"
	"github.com/emzhofb/gowallet/scheduler-service/internal/logger"
	"github.com/emzhofb/gowallet/scheduler-service/internal/scheduler"
	authPb "github.com/emzhofb/gowallet/auth-service/proto/auth"
	walletPb "github.com/emzhofb/gowallet/wallet-service/proto/wallet"
	txPb "github.com/emzhofb/gowallet/transaction-service/proto/transaction"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	logger.InitLogger()
	logger.Log.Info("Starting Centralized Scheduler Service...")

	cfg := config.LoadConfig()

	// 1. Koneksi gRPC ke Auth Service
	authConn, err := grpc.Dial(cfg.AuthGRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Could not connect to Auth gRPC: %v", err)
	}
	defer authConn.Close()
	authClient := authPb.NewAuthServiceClient(authConn)

	// 2. Koneksi gRPC ke Wallet Service
	walletConn, err := grpc.Dial(cfg.WalletGRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Could not connect to Wallet gRPC: %v", err)
	}
	defer walletConn.Close()
	walletClient := walletPb.NewWalletServiceClient(walletConn)

	// 3. Koneksi gRPC ke Transaction Service
	txConn, err := grpc.Dial(cfg.TransactionGRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Could not connect to Transaction gRPC: %v", err)
	}
	defer txConn.Close()
	txClient := txPb.NewTransactionServiceClient(txConn)

	// 4. Inisialisasi & Mulai Scheduler
	sched := scheduler.NewScheduler(authClient, walletClient, txClient)
	sched.Start()

	// Menunggu signal shutdown (graceful shutdown)
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)
	<-stopChan

	sched.Stop()
}
```

---

## ✅ Acceptance Criteria
* [ ] Folder `scheduler-service` terdaftar dengan benar di `go.work`.
* [ ] Menjalankan daemon `scheduler-service` tidak menampilkan error koneksi gRPC.
* [ ] Log `Centralized Scheduler Service started successfully!` tercetak dengan sukses di terminal.
* [ ] Eksekusi task maintenance (seperti `ReconcileBalances`) terpicu otomatis sesuai jadwal dan memicu logika terkait di service tujuannya tanpa akses database langsung dari scheduler-service.

---

## 💡 Tips untuk Junior
* **No Database Connection:** Kelebihan utama arsitektur ini adalah `scheduler-service` tidak memerlukan koneksi DB sama sekali. Ini mengurangi jumlah pool koneksi DB yang menganggur dan mengisolasi logika bisnis domain tetap berada di service asalnya.
* **gRPC Server Readiness:** Karena scheduler bergantung penuh pada gRPC server dari service lain, pastikan Anda menambahkan policy health-checking dan restart delay pada container scheduler jika menggunakan orkestrasi seperti Docker Compose.
