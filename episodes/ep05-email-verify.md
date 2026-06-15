# Episode 5: Email Verification & Reset Password

## 🎯 Tujuan
- Email verification saat register (OTP via email)
- Forgot password flow
- Reset password flow
- Setup email sender (development: MailHog)

## 📝 Prerequisites
- Episode 3 & 4 selesai (Auth Service sudah jalan)

---

## 📦 Langkah-langkah

### Step 1: Tambahkan MailHog ke Docker Compose

MailHog adalah fake SMTP server untuk development — email tidak benar-benar terkirim, tapi bisa dilihat di UI.

Tambahkan di `docker-compose.yml`:
```yaml
  mailhog:
    image: mailhog/mailhog:latest
    container_name: gowallet-mailhog
    ports:
      - "1025:1025"    # SMTP server
      - "8025:8025"    # Web UI
    networks:
      - gowallet-network
```

```bash
docker compose up -d mailhog
# Buka http://localhost:8025 untuk melihat email yang terkirim
```

### Step 2: Database Migration

```bash
migrate create -ext sql -dir auth-service/db/migrations -seq create_otp_codes
```

**`000003_create_otp_codes.up.sql`:**
```sql
CREATE TABLE IF NOT EXISTS otp_codes (
    id CHAR(36) PRIMARY KEY,
    user_id CHAR(36) NOT NULL,
    code VARCHAR(6) NOT NULL,
    type ENUM('email_verification', 'password_reset') NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    used BOOLEAN NOT NULL DEFAULT FALSE,
    used_at TIMESTAMP NULL DEFAULT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    INDEX idx_user_id (user_id),
    INDEX idx_code (code),
    INDEX idx_type_user (type, user_id),
    INDEX idx_expires_at (expires_at),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

**`000003_create_otp_codes.down.sql`:**
```sql
DROP TABLE IF EXISTS otp_codes;
```

```bash
make migrate-up s=auth
```

### Step 3: Email Sender Package

Buat `pkg/email/email.go` atau langsung di auth-service:

```go
// Menggunakan standard library net/smtp
// atau github.com/jordan-wright/email untuk lebih mudah

type EmailSender interface {
    SendOTP(to string, otp string, otpType string) error
    SendWelcome(to string, name string) error
    SendPasswordReset(to string, otp string) error
}

type smtpSender struct {
    host     string  // localhost (MailHog) atau smtp.gmail.com (production)
    port     string  // 1025 (MailHog) atau 587 (production)
    user     string
    password string
    from     string  // noreply@gowallet.com
}
```

### Step 4: OTP Repository

```go
type OTPRepository interface {
    Create(ctx context.Context, otp *model.OTPCode) error
    GetByCode(ctx context.Context, code string, otpType string) (*model.OTPCode, error)
    GetLatestByUserID(ctx context.Context, userID string, otpType string) (*model.OTPCode, error)
    MarkUsed(ctx context.Context, id string) error
    DeleteExpired(ctx context.Context) (int64, error)
}
```

### Step 5: OTP Generation

```go
import "crypto/rand"

func GenerateOTP() string {
    // Generate 6 digit random number
    max := 999999
    min := 100000
    n, _ := rand.Int(rand.Reader, big.NewInt(int64(max-min)))
    return fmt.Sprintf("%06d", n.Int64()+int64(min))
}
```

### Step 6: Endpoints

| Method | Path | Auth | Request Body | Description |
|---|---|---|---|---|
| POST | `/api/v1/auth/send-verification` | ✅ | (none) | Kirim OTP verifikasi ke email user |
| POST | `/api/v1/auth/verify-email` | ❌ | `{email, code}` | Verifikasi email dengan OTP |
| POST | `/api/v1/auth/forgot-password` | ❌ | `{email}` | Kirim OTP reset password |
| POST | `/api/v1/auth/reset-password` | ❌ | `{email, code, new_password}` | Reset password |

### Step 7: Service Implementation

**Send Verification:**
```
1. Get user dari context (sudah login)
2. Cek apakah sudah verified → jika ya, return error
3. Cek apakah ada OTP aktif (belum expired) → jika ya, return "OTP sudah dikirim"
4. Generate OTP 6 digit
5. Simpan OTP ke database (expires_at = now + 5 menit)
6. Kirim email dengan OTP
7. Return success
```

**Verify Email:**
```
1. Cari user by email
2. Cari OTP by code + type='email_verification'
3. Validasi:
   - OTP ada?
   - OTP belum expired?
   - OTP belum digunakan?
   - OTP milik user yang benar?
4. Mark OTP as used
5. Update user.is_verified = true
6. Return success
```

**Forgot Password:**
```
1. Cari user by email
2. Jika user tidak ditemukan → JANGAN kasih error!
   Return "If the email exists, an OTP has been sent" (prevent email enumeration)
3. Jika user ada:
   a. Generate OTP
   b. Simpan ke database (type='password_reset', expires_at = now + 5 menit)
   c. Kirim email
4. Return generic success message
```

**Reset Password:**
```
1. Cari user by email
2. Cari OTP by code + type='password_reset' + user_id
3. Validasi OTP (sama seperti verify email)
4. Hash password baru (bcrypt)
5. Update user.password_hash
6. Mark OTP as used
7. REVOKE SEMUA refresh token user (force re-login)
8. Return success
```

### Step 8: Email Templates

Buat simple HTML email templates:

**Verification Email:**
```
Subject: GoWallet - Verifikasi Email Anda

Halo {name},

Kode verifikasi email Anda adalah:

{OTP_CODE}

Kode ini berlaku selama 5 menit.
Jika Anda tidak mendaftar di GoWallet, abaikan email ini.
```

**Reset Password Email:**
```
Subject: GoWallet - Reset Password

Halo {name},

Kode untuk reset password Anda adalah:

{OTP_CODE}

Kode ini berlaku selama 5 menit.
Jika Anda tidak meminta reset password, abaikan email ini dan pastikan akun Anda aman.
```

### Step 9: Test Manual

```bash
# 1. Register user baru
curl -X POST http://localhost:8081/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"verify@example.com","password":"password123","full_name":"Verify User"}'

# 2. Kirim verification OTP (perlu login dulu)
ACCESS_TOKEN="<dari register response>"
curl -X POST http://localhost:8081/api/v1/auth/send-verification \
  -H "Authorization: Bearer $ACCESS_TOKEN"

# 3. Cek MailHog UI → http://localhost:8025
# Lihat email yang masuk, copy OTP

# 4. Verify email
curl -X POST http://localhost:8081/api/v1/auth/verify-email \
  -H "Content-Type: application/json" \
  -d '{"email":"verify@example.com","code":"123456"}'

# 5. Forgot Password
curl -X POST http://localhost:8081/api/v1/auth/forgot-password \
  -H "Content-Type: application/json" \
  -d '{"email":"verify@example.com"}'

# 6. Cek MailHog untuk OTP reset password

# 7. Reset Password
curl -X POST http://localhost:8081/api/v1/auth/reset-password \
  -H "Content-Type: application/json" \
  -d '{"email":"verify@example.com","code":"654321","new_password":"newpassword123"}'

# 8. Login dengan password baru
curl -X POST http://localhost:8081/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"verify@example.com","password":"newpassword123"}'
```

---

## ✅ Acceptance Criteria

- [ ] OTP terkirim ke email (visible di MailHog)
- [ ] OTP valid → user.is_verified = true
- [ ] OTP expired (>5 menit) → error
- [ ] OTP hanya bisa dipakai sekali
- [ ] Forgot password → OTP terkirim
- [ ] Forgot password dengan email tidak ada → tetap return success (no leak)
- [ ] Reset password → password berubah, semua token di-revoke
- [ ] Login dengan password baru berhasil
- [ ] Unit test untuk OTP generation, validation, expiry

---

## 💡 Tips & Common Pitfalls

1. **Jangan leak email existence!** — `forgot-password` harus selalu return success, even jika email tidak terdaftar.
2. **OTP harus expire** — 5 menit adalah standard. Jangan terlalu lama.
3. **Rate limit OTP** — Jangan biarkan user spam OTP. Max 3 request per 15 menit.
4. **MailHog tidak butuh auth** — Di development, biarkan SMTP tanpa user/password.

---

## 📚 Referensi Belajar

- [MailHog Docker](https://github.com/mailhog/MailHog)
- [Go net/smtp](https://pkg.go.dev/net/smtp)
- [OWASP Forgot Password Cheatsheet](https://cheatsheetseries.owasp.org/cheatsheets/Forgot_Password_Cheat_Sheet.html)
