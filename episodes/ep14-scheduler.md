# Episode 14: Scheduler/Worker Service

## 🎯 Tujuan
- Cleanup expired refresh tokens
- Cleanup expired OTPs
- Retry failed outbox events
- Scheduled balance reconciliation
- Dead letter queue monitoring

## 📝 Prerequisites
- Episode 1-13 selesai

---

## 📦 Langkah-langkah

### Step 1: Inisialisasi Service

```bash
mkdir -p scheduler-service/cmd
mkdir -p scheduler-service/internal/{jobs,config}

cd scheduler-service
go mod init github.com/emzhofb/gowallet/scheduler-service
go get github.com/robfig/cron/v3
cd ..
go work use ./scheduler-service
```

### Step 2: Job Definitions

| Job | Schedule | Description |
|---|---|---|
| `CleanupRefreshTokens` | Setiap 1 jam | Hapus token expired > 24 jam |
| `CleanupOTP` | Setiap 30 menit | Hapus OTP expired > 1 jam |
| `RetryOutbox` | Setiap 5 detik | Publish outbox events yang belum ter-publish |
| `ReconcileBalance` | Setiap hari jam 02:00 | Bandingkan saldo wallet vs ledger |
| `MonitorDLQ` | Setiap 15 menit | Log dead letter count |

### Step 3: Implementation

```go
func main() {
    // Setup config, logger, DB connections...
    
    c := cron.New(cron.WithSeconds()) // Support detik
    
    // Cleanup jobs
    c.AddFunc("0 0 * * * *", cleanupRefreshTokens)    // Setiap jam
    c.AddFunc("0 */30 * * * *", cleanupOTP)            // Setiap 30 menit
    c.AddFunc("*/5 * * * * *", retryOutbox)            // Setiap 5 detik
    c.AddFunc("0 0 2 * * *", reconcileBalance)          // Setiap hari jam 02:00
    c.AddFunc("0 */15 * * * *", monitorDLQ)            // Setiap 15 menit
    
    c.Start()
    
    // Graceful shutdown
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit
    c.Stop()
}
```

**Cleanup Refresh Tokens:**
```go
func cleanupRefreshTokens() {
    // DELETE FROM refresh_tokens WHERE expires_at < NOW() - INTERVAL 24 HOUR
    result, err := db.Exec(
        "DELETE FROM refresh_tokens WHERE expires_at < DATE_SUB(NOW(), INTERVAL 24 HOUR)")
    rows, _ := result.RowsAffected()
    logger.Info("cleanup refresh tokens", zap.Int64("deleted", rows))
}
```

**Reconcile Balance:**
```go
func reconcileBalance() {
    // 1. Get semua wallet
    // 2. Untuk setiap wallet, hitung saldo dari ledger
    // 3. Bandingkan dengan wallet.balance
    // 4. Jika berbeda → LOG WARNING + kirim alert
    
    wallets := walletRepo.GetAll(ctx)
    for _, w := range wallets {
        ledgerBalance := ledgerRepo.CalculateBalance(ctx, w.ID)
        if w.Balance != ledgerBalance {
            logger.Error("RECONCILIATION MISMATCH",
                zap.String("wallet_id", w.ID),
                zap.Float64("wallet_balance", w.Balance),
                zap.Float64("ledger_balance", ledgerBalance),
                zap.Float64("difference", w.Balance-ledgerBalance),
            )
        }
    }
}
```

### Step 4: Health Check

Scheduler juga perlu health check:
```
GET /health → service running
GET /ready → DB connections OK
```

---

## ✅ Acceptance Criteria

- [ ] Cleanup jobs berjalan sesuai jadwal
- [ ] Expired tokens dan OTP terhapus
- [ ] Outbox events yang gagal ter-retry
- [ ] Reconciliation mismatch ter-log
- [ ] DLQ count ter-monitor

---

## 📚 Referensi

- [robfig/cron](https://github.com/robfig/cron)
- [Cron Expression Generator](https://crontab.guru/)
