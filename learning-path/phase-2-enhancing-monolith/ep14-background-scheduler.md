# Episode 14: Background Schedulers (Cron)

## 🎯 Tujuan
* Mengenalkan konsep **Asynchronous Background Processing** menggunakan library cron scheduler.
* Mengotomatiskan tugas-tugas pemeliharaan database secara berkala:
  * Menghapus OTP / token kedaluwarsa secara terjadwal.
* Menjalankan tugas **Audit Rekonsiliasi Saldo** harian secara otomatis untuk mendeteksi perbedaan saldo wallet vs total ledger.

---

## 🕒 Kenapa Butuh Scheduler Terpisah?
Banyak aktivitas backend yang tidak boleh memblokir request pengguna (misal, menghapus data sampah di DB). Tugas-tugas ini lebih baik dijalankan di latar belakang (*background worker*) secara terjadwal (misal setiap malam jam 2 pagi, atau setiap 30 menit).

Di Go, kita menggunakan library `robfig/cron` yang berjalan sebagai goroutine di latar belakang aplikasi monolith kita.

---

## 📦 Langkah-langkah

### Step 1: Install Library Cron
Unduh library `robfig/cron` versi 3:
```bash
go get github.com/robfig/cron/v3
```

### Step 2: Membuat Scheduler Module (`internal/scheduler/scheduler.go`)
Kita akan mendefinisikan scheduler utama yang mengumpulkan semua cron jobs.

Buat file baru di `internal/scheduler/scheduler.go`:

```go
package scheduler

import (
	"context"
	"database/sql"
	"time"

	"github.com/emzhofb/gowallet/monolith/internal/logger"
	ledgerRepo "github.com/emzhofb/gowallet/monolith/internal/ledger/repository"
	walletRepo "github.com/emzhofb/gowallet/monolith/internal/wallet/repository"
	"github.com/robfig/cron/v3"
)

type Scheduler struct {
	cron       *cron.Cron
	db         *sql.DB
	walletRepo walletRepo.WalletRepository
	ledgerRepo ledgerRepo.LedgerRepository
}

func NewScheduler(db *sql.DB, wRepo walletRepo.WalletRepository, lRepo ledgerRepo.LedgerRepository) *Scheduler {
	// Buat instance cron baru dengan seconds parser (opsional)
	c := cron.New(cron.WithSeconds())
	return &Scheduler{
		cron:       c,
		db:         db,
		walletRepo: wRepo,
		ledgerRepo: lRepo,
	}
}

func (s *Scheduler) Start() {
	// 1. Job 1: Bersihkan token OTP expired setiap 30 menit
	// Format cron: "detik menit jam hari_bulan bulan hari_minggu"
	s.cron.AddFunc("0 */30 * * * *", s.CleanupExpiredOTPs)

	// 2. Job 2: Rekonsiliasi Saldo Harian setiap jam 02:00 pagi
	s.cron.AddFunc("0 0 2 * * *", s.ReconcileAllBalances)

	// 3. Job 3: Bersihkan Refresh Token expired setiap hari pada jam 03:00 pagi
	s.cron.AddFunc("0 0 3 * * *", s.CleanupExpiredRefreshTokens)

	s.cron.Start()
	logger.Log.Info("Background scheduler successfully started!")
}

func (s *Scheduler) Stop() {
	s.cron.Stop()
	logger.Log.Info("Background scheduler stopped.")
}
```

### Step 3: Membuat Job 1 — Cleanup OTPs & Job 3 — Cleanup Refresh Tokens
Buat file baru `internal/scheduler/jobs.go` untuk menyimpan detail fungsi-fungsi job:

```go
package scheduler
import (
	"context"
	"time"

	"github.com/emzhofb/gowallet/monolith/internal/logger"
)

func (s *Scheduler) CleanupExpiredOTPs() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logger.Log.InfoContext(ctx, "[Cron Job] Starting expired OTP cleanup...")

	// Hapus OTP yang sudah expired lebih dari 1 jam untuk menghemat disk
	query := `DELETE FROM otp_codes WHERE expires_at < NOW()`
	
	// Catatan: kita akan membuat tabel otp_codes di Episode 14. 
	// Untuk saat ini, kita log dan bypass query jika tabel belum ada.
	result, err := s.db.ExecContext(ctx, query)
	if err != nil {
		logger.Warn(ctx, "[Cron Job] Bypass: Table otp_codes not created yet. Skipped.", "error", err.Error())
		return
	}

	rowsAffected, _ := result.RowsAffected()
	logger.Log.InfoContext(ctx, "[Cron Job] Expired OTP cleanup finished successfully.", "deleted_rows", rowsAffected)
}

func (s *Scheduler) CleanupExpiredRefreshTokens() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	logger.Log.InfoContext(ctx, "[Cron Job] Starting expired refresh token cleanup...")

	// Hapus refresh token yang sudah expired dari database
	query := `DELETE FROM refresh_tokens WHERE expires_at < NOW()`

	result, err := s.db.ExecContext(ctx, query)
	if err != nil {
		logger.Warn(ctx, "[Cron Job] Bypass: Table refresh_tokens not created yet. Skipped.", "error", err.Error())
		return
	}

	rowsAffected, _ := result.RowsAffected()
	logger.Log.InfoContext(ctx, "[Cron Job] Expired refresh token cleanup finished successfully.", "deleted_rows", rowsAffected)
}
```

### Step 4: Membuat Job 2 — Rekonsiliasi Saldo Harian
Fungsi ini berjalan setiap malam untuk memeriksa seluruh wallet di database. Fungsi ini akan membandingkan kolom `balance` di tabel `wallets` dengan total sum `ledger_entries`. Jika ada perbedaan (*mismatch*), sistem akan mencatat log error tingkat tinggi (*high priority alert*).

Tambahkan method `ReconcileAllBalances` di `internal/scheduler/jobs.go`:

```go
func (s *Scheduler) ReconcileAllBalances() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	logger.Log.InfoContext(ctx, "[Cron Job] Starting daily balance reconciliation audit...")

	// 1. Ambil semua data wallet di database
	query := `SELECT id, user_id, balance FROM wallets`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		logger.Error(ctx, "[Cron Job] Reconciliation failed to query wallets", "error", err.Error())
		return
	}
	defer rows.Close()

	mismatchCount := 0

	for rows.Next() {
		var walletID string
		var userID string
		var currentBalance float64

		if err := rows.Scan(&walletID, &userID, &currentBalance); err != nil {
			continue
		}

		// 2. Hitung total sum dari ledger entries untuk wallet ini
		ledgerBalance, err := s.ledgerRepo.GetBalanceByWalletID(ctx, walletID)
		if err != nil {
			logger.Error(ctx, "[Cron Job] Reconciliation failed to get ledger balance", "wallet_id", walletID, "error", err.Error())
			continue
		}

		// 3. Bandingkan saldo
		if currentBalance != ledgerBalance {
			mismatchCount++
			logger.Error(ctx, "CRITICAL: BALANCE MISMATCH DETECTED DURING AUDIT!",
				"wallet_id", walletID,
				"user_id", userID,
				"wallet_table_balance", currentBalance,
				"ledger_calculated_balance", ledgerBalance,
				"difference", currentBalance-ledgerBalance,
			)
			
			// DI SINI: Di production, Anda bisa menembak API Slack Alert / Telegram Alert ke tim Developer
		}
	}

	logger.Log.InfoContext(ctx, "[Cron Job] Daily balance reconciliation finished.", "mismatch_wallets_count", mismatchCount)
}
```

### Step 5: Jalankan Scheduler di `cmd/main.go`
Buka `cmd/main.go`. Inisialisasi scheduler dan jalankan method `Start()`. Pastikan kita memanggil `Stop()` saat server shutdown agar semua job yang sedang berjalan diselesaikan secara rapi.

```go
    // ...
	// 1. Inisialisasi Layer
	uRepo := userRepository.NewMySQLUserRepository(db)
	wRepo := walletRepository.NewMySQLWalletRepository(db)
	lRepo := ledgerRepository.NewMySQLLedgerRepository(db)
	txRepo := transactionRepository.NewMySQLTransactionRepository(db)
    
	uSvc := userService.NewUserService(db, uRepo, wRepo) 
	wSvc := walletService.NewWalletService(wRepo, rdb)
	txSvc := transactionService.NewTransactionService(db, txRepo, uRepo, wRepo, lRepo, rdb)
    
	// 2. Setup Background Scheduler
	cronSched := scheduler.NewScheduler(db, wRepo, lRepo)
	cronSched.Start()
	defer cronSched.Stop() // Matikan secara rapi saat aplikasi mati
    
	r := gin.New()
	r.Use(gin.Recovery())
    // ...
```

---

## ✅ Acceptance Criteria
* [ ] Menjalankan aplikasi Go memicu log `Background scheduler successfully started!`.
* [ ] Schedulers berjalan secara non-blocking (server Gin tetap bisa melayani request HTTP port `8080` tanpa terganggu).
* [ ] Logika rekonsiliasi harian berjalan sesuai jadwal dan sukses mencatat log normal jika saldo sinkron, atau log `CRITICAL` jika kita sengaja memanipulasi saldo di tabel `wallets` secara ilegal via DB client GUI.

---

## 💡 Tips untuk Junior
* **Context Timeout:** Selalu set `context.WithTimeout` saat menjalankan cron job. Ini penting agar jika terjadi kemacetan query database, proses cron job tidak akan menggantung selamanya (*deadlock/goroutine leak*).
* **Gunakan Waktu Server yang Konsisten:** Di MySQL dan sistem operasi server, selalu gunakan zona waktu **UTC** agar tidak pusing mengonversi perbedaan jam antara komputer lokal Anda dengan server production cloud.

---

## 📚 Referensi Belajar
* [robfig/cron documentation](https://github.com/robfig/cron)
* [Linux Cron syntax generator](https://crontab.guru/)
