# Episode 15: Email Verification & Forgot Password

## 🎯 Tujuan
* Membuat skema migrasi tabel baru `otp_codes` untuk menyimpan kode OTP keamanan.
* Mengimplementasikan flow **Email Verification** setelah registrasi user.
* Mengirim email secara asinkronus menggunakan **Go Goroutine** (`go sendEmail(...)`) agar waktu respons registrasi user tetap secepat kilat tanpa tertunda proses pengiriman email ke server SMTP.
* Membuat alur **Forgot Password** dan **Reset Password** menggunakan kode OTP.

---

## 📐 Kenapa Mengirim Email Harus Asinkronus?
Menghubungi server email (SMTP) eksternal seperti SendGrid/Mailgun membutuhkan koneksi jaringan TCP dan memakan waktu sekitar **1 hingga 3 detik**. 
Jika pengiriman email dijalankan langsung di dalam *main thread* registrasi user:
```
Client Request ➔ Simpan User ➔ Kirim Email (Tunggu 2s) ➔ Kembalikan Response (Total: 2.2 detik)
```
Ini sangat lambat dan membuat user experience buruk. Dengan Goroutine, kita memindahkan proses lambat tersebut ke thread latar belakang:
```
Client Request ➔ Simpan User ➔ Trigger Goroutine (go kirimEmail()) ➔ Kembalikan Response (Total: 0.1 detik!)
```

---

## 📦 Langkah-langkah

### Step 1: Membuat Skema Migrasi untuk Tabel OTP
Buat file migrasi baru `db/migrations/000002_create_otp_table.up.sql`:

```sql
CREATE TABLE otp_codes (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL,
    code VARCHAR(6) NOT NULL,
    type VARCHAR(30) NOT NULL, -- email_verification, password_reset
    expires_at TIMESTAMP NOT NULL,
    used BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
```

Buat juga file down migrasi `db/migrations/000002_create_otp_table.down.sql`:
```sql
DROP TABLE IF EXISTS otp_codes;
```

Jalankan command migrasi untuk membuat tabel baru:
```bash
migrate -path db/migrations -database "mysql://gowallet_user:gowallet_password@tcp(localhost:3306)/gowallet" up
```

### Step 2: Membuat SMTP Email Sender Helper (`internal/email/email.go`)
Untuk keperluan testing lokal, kita bisa menggunakan **MailHog** (SMTP Mock server) atau default server SMTP. Kita gunakan library standard `net/smtp`.

Buat file baru di `internal/email/email.go`:

```go
package email

import (
	"context"
	"fmt"
	"net/smtp"

	"github.com/emzhofb/gowallet/monolith/internal/logger"
)

type EmailSender interface {
	SendEmail(ctx context.Context, to string, subject string, body string) error
}

type smtpEmailSender struct {
	host string
	port string
	from string
}

func NewSMTPEmailSender(host string, port string, from string) EmailSender {
	return &smtpEmailSender{
		host: host,
		port: port,
		from: from,
	}
}

func (s *smtpEmailSender) SendEmail(ctx context.Context, to string, subject string, body string) error {
	msg := []byte(fmt.Sprintf("To: %s\r\nSubject: %s\r\n\r\n%s\r\n", to, subject, body))
	
	// Untuk local testing dengan MailHog, kita kirim tanpa auth
	addr := s.host + ":" + s.port
	err := smtp.SendMail(addr, nil, s.from, []string{to}, msg)
	if err != nil {
		logger.Error(ctx, "Failed to send email via SMTP", "to", to, "error", err.Error())
		return err
	}

	logger.Info(ctx, "Email sent successfully", "to", to)
	return nil
}
```

### Step 3: Membuat OTP Domain (`internal/otp/`)
Buat folder baru: `internal/otp/model` & `internal/otp/repository` & `internal/otp/service`

Buat model OTP di `internal/otp/model/otp.go`:
```go
package model

import "time"

type OTP struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Code      string    `json:"code"`
	Type      string    `json:"type"` // email_verification, password_reset
	ExpiresAt time.Time `json:"expires_at"`
	Used      bool      `json:"used"`
	CreatedAt time.Time `json:"created_at"`
}
```

Buat repository OTP di `internal/otp/repository/repository.go`:
```go
package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/emzhofb/gowallet/monolith/internal/otp/model"
)

type OTPRepository interface {
	Create(ctx context.Context, o *model.OTP) error
	GetActiveOTP(ctx context.Context, userID string, code string, otpType string) (*model.OTP, error)
	MarkAsUsed(ctx context.Context, id string) error
}

type mysqlOTPRepository struct {
	db *sql.DB
}

func NewMySQLOTPRepository(db *sql.DB) OTPRepository {
	return &mysqlOTPRepository{db: db}
}

func (r *mysqlOTPRepository) Create(ctx context.Context, o *model.OTP) error {
	query := `INSERT INTO otp_codes (id, user_id, code, type, expires_at, used) VALUES (?, ?, ?, ?, ?, ?)`
	_, err := r.db.ExecContext(ctx, query, o.ID, o.UserID, o.Code, o.Type, o.ExpiresAt, o.Used)
	return err
}

func (r *mysqlOTPRepository) GetActiveOTP(ctx context.Context, userID string, code string, otpType string) (*model.OTP, error) {
	query := `SELECT id, user_id, code, type, expires_at, used FROM otp_codes WHERE user_id = ? AND code = ? AND type = ? AND used = 0 AND expires_at > NOW()`
	o := &model.OTP{}
	err := r.db.QueryRowContext(ctx, query, userID, code, otpType).Scan(&o.ID, &o.UserID, &o.Code, &o.Type, &o.ExpiresAt, &o.Used)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("active OTP not found or expired")
		}
		return nil, err
	}
	return o, nil
}

func (r *mysqlOTPRepository) MarkAsUsed(ctx context.Context, id string) error {
	query := `UPDATE otp_codes SET used = 1 WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}
```

### Step 4: Menambahkan Logic Asynchronous Email & OTP di Register
Buka `internal/user/service/service.go`. Kita suntikkan `OTPRepository` dan `EmailSender`.

Di dalam method `Register`, setelah user berhasil tersimpan di DB (setelah `tx.Commit()`), tambahkan pemanggilan goroutine asinkronus untuk membuat OTP dan mengirim email:

```go
// Di dalam method Register setelah commit sukses:

	// 5. Generate 6 digit OTP Code secara acak
	otpCode := "123456" // Untuk mempermudah, atau gunakan fungsi random string
	
	otpModel := &otpModel.OTP{
		ID:        uuid.New().String(),
		UserID:    user.ID,
		Code:      otpCode,
		Type:      "email_verification",
		ExpiresAt: time.Now().Add(15 * time.Minute), // Masa aktif 15 menit
		Used:      false,
	}
	
	// Simpan OTP ke database
	s.otpRepo.Create(ctx, otpModel)

	// 6. Jalankan goroutine untuk mengirim email secara asinkronus
	go func() {
		// Buat context baru yang independen dari siklus request HTTP
		bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		
		subject := "GoWallet - Verifikasi Email Anda"
		body := fmt.Sprintf("Halo %s,\n\nKode verifikasi Anda adalah: %s\nKode ini berlaku selama 15 menit.", user.FullName, otpCode)
		
		s.emailSender.SendEmail(bgCtx, user.Email, subject, body)
	}()
```

### Step 5: Membuat Handler Konfirmasi Verifikasi
Di `internal/user/handler/handler.go`, tambahkan endpoint konfirmasi verifikasi OTP:

```go
type VerifyOTPRequest struct {
	Code string `json:"code" binding:"required,len=6"`
}

func (h *UserHandler) VerifyEmail(c *gin.Context) {
	userID, _ := c.Get("user_id")
	var req VerifyOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(customErr.NewAppError(http.StatusBadRequest, "INVALID_INPUT", err.Error()))
		return
	}

	// Di service, panggil get active OTP & update status user menjadi verified
	// Untuk simplicity:
	// 1. Cek OTP di database
	// 2. Jika valid, update tabel users (is_verified = 1 / mark user as verified)
	// 3. Mark OTP as used
}
```

---

## ✅ Acceptance Criteria
* [ ] Tabel `otp_codes` berhasil dibuat melalui script migrasi database.
* [ ] Melakukan registrasi user baru langsung mengembalikan respons sukses instan (< 100ms) tanpa jeda loading pengiriman email.
* [ ] Server log memuat info `Email sent successfully` beberapa detik setelah registrasi sukses (menandakan goroutine berjalan mulus di background).
* [ ] Jika menggunakan MailHog di local, email verifikasi masuk ke dashboard inbox MailHog secara tepat waktu.

---

## 💡 Tips untuk Junior
* **Independen Context di Goroutine:** Perhatikan kode `bgCtx := context.Background()`. Kita **tidak boleh** mengoper parameter `ctx` asli bawaan request HTTP (seperti `c.Request.Context()`) ke dalam goroutine asinkronus. Mengapa? Karena ketika request HTTP selesai dikembalikan ke client, Gin Context asli tersebut akan langsung di-destory/closed. Goroutine kita yang masih berjalan akan gagal mengirim email karena context-nya tiba-tiba dibatalkan (*context canceled*).

---

## 📚 Referensi Belajar
* [Goroutines in Go (Concurrency)](https://go.dev/tour/concurrency/1)
* [Go net/smtp standard library](https://pkg.go.dev/net/smtp)
* [MailHog Local Testing Server](https://github.com/mailhog/MailHog)
