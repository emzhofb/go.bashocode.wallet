# Episode 12: Notification Service

## 🎯 Tujuan
- Consume event dari RabbitMQ
- Kirim email notifikasi untuk berbagai event
- HTML email templates
- Retry mechanism untuk email gagal

## 📝 Prerequisites
- Episode 11 selesai (RabbitMQ setup)
- MailHog running (dari Episode 5)

---

## 📦 Langkah-langkah

### Step 1: Inisialisasi Service

```bash
mkdir -p notification-service/cmd
mkdir -p notification-service/internal/{handler,service,consumer,templates,config}

cd notification-service
go mod init github.com/emzhofb/gowallet/notification-service
cd ..
go work use ./notification-service
```

### Step 2: Events yang Di-consume

| Queue | Event | Action |
|---|---|---|
| `notif.transfer` | `transfer.completed` | Email ke sender & receiver |
| `notif.payment` | `payment.completed` | Email konfirmasi payment |
| `notif.welcome` | `user.registered` | Email welcome |
| `notif.reset` | `auth.password_reset` | Email link reset password |
| `notif.topup` | `wallet.topup` | Email konfirmasi top up |

### Step 3: Consumer Setup

```go
func main() {
    // ... setup config, logger, rabbitmq connection ...
    
    emailSender := email.NewSMTPSender(cfg.SMTP)
    notifService := service.NewNotificationService(emailSender, logger)
    
    // Start consumers (each in its own goroutine)
    consumers := []Consumer{
        NewConsumer(ch, "notif.transfer", notifService.HandleTransferCompleted, 3, logger),
        NewConsumer(ch, "notif.payment", notifService.HandlePaymentCompleted, 3, logger),
        NewConsumer(ch, "notif.welcome", notifService.HandleUserRegistered, 3, logger),
        NewConsumer(ch, "notif.reset", notifService.HandlePasswordReset, 3, logger),
        NewConsumer(ch, "notif.topup", notifService.HandleTopUp, 3, logger),
    }
    
    ctx, cancel := context.WithCancel(context.Background())
    for _, c := range consumers {
        go c.Start(ctx)
    }
    
    // Graceful shutdown
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit
    cancel()
}
```

### Step 4: Notification Service

```go
func (s *NotificationService) HandleTransferCompleted(ctx context.Context, body []byte) error {
    var event TransferEvent
    json.Unmarshal(body, &event)
    
    // Email ke sender
    err := s.emailSender.Send(email.Email{
        To:       event.Data.SenderEmail,
        Subject:  "GoWallet - Transfer Berhasil",
        Template: "transfer_sent",
        Data: map[string]interface{}{
            "Name":     event.Data.SenderName,
            "Amount":   formatCurrency(event.Data.Amount),
            "Receiver": event.Data.ReceiverName,
            "Date":     event.Timestamp,
            "TxID":     event.Data.TransactionID,
        },
    })
    if err != nil {
        return fmt.Errorf("failed to send email to sender: %w", err)
    }
    
    // Email ke receiver
    err = s.emailSender.Send(email.Email{
        To:       event.Data.ReceiverEmail,
        Subject:  "GoWallet - Anda Menerima Transfer",
        Template: "transfer_received",
        Data:     map[string]interface{}{...},
    })
    
    return err
}
```

### Step 5: HTML Email Templates

Buat templates di `notification-service/internal/templates/`:

Gunakan `html/template` standard library Go.

Contoh `transfer_sent.html`:
```html
<!DOCTYPE html>
<html>
<body style="font-family: Arial, sans-serif; padding: 20px;">
  <h2 style="color: #4CAF50;">Transfer Berhasil ✅</h2>
  <p>Halo {{.Name}},</p>
  <p>Transfer Anda telah berhasil diproses:</p>
  <table style="border-collapse: collapse; width: 100%;">
    <tr><td style="padding: 8px; border: 1px solid #ddd;">Jumlah</td>
        <td style="padding: 8px; border: 1px solid #ddd;"><b>{{.Amount}}</b></td></tr>
    <tr><td style="padding: 8px; border: 1px solid #ddd;">Penerima</td>
        <td style="padding: 8px; border: 1px solid #ddd;">{{.Receiver}}</td></tr>
    <tr><td style="padding: 8px; border: 1px solid #ddd;">Tanggal</td>
        <td style="padding: 8px; border: 1px solid #ddd;">{{.Date}}</td></tr>
    <tr><td style="padding: 8px; border: 1px solid #ddd;">ID Transaksi</td>
        <td style="padding: 8px; border: 1px solid #ddd;">{{.TxID}}</td></tr>
  </table>
  <p style="color: #888; font-size: 12px;">GoWallet - Digital Wallet</p>
</body>
</html>
```

### Step 6: Test

```bash
# 1. Start Notification Service
cd notification-service && go run cmd/main.go

# 2. Trigger transfer (dari Transaction Service)
# Email akan otomatis terkirim

# 3. Cek MailHog: http://localhost:8025
# Verifikasi email terkirim dengan template yang benar
```

---

## ✅ Acceptance Criteria

- [ ] Consumer berjalan dan consume dari semua queue
- [ ] Email terkirim untuk setiap event type
- [ ] HTML template rendering benar
- [ ] Retry bekerja jika email gagal terkirim
- [ ] DLQ menangkap notification yang gagal permanen
- [ ] Email visible di MailHog

---

## 📚 Referensi

- [Go html/template](https://pkg.go.dev/html/template)
- [Go net/smtp](https://pkg.go.dev/net/smtp)
