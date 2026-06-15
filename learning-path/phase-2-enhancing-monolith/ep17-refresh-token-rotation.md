# Episode 16: Refresh Token Rotation & Token Reuse Detection

## 🎯 Tujuan
* Menambahkan skema migrasi tabel `refresh_tokens` untuk menyimpan riwayat dan status token.
* Membuat REST API endpoint `/api/v1/auth/refresh` untuk memperbarui Access Token menggunakan Refresh Token.
* Mengamankan alur refresh dengan **Refresh Token Rotation (RTR)** (menerbitkan Refresh Token baru dan menonaktifkan Refresh Token lama pada setiap request refresh).
* Menerapkan **Token Reuse Detection** (jika token lama yang sudah dinonaktifkan digunakan kembali, batalkan seluruh sesi aktif pengguna tersebut demi keamanan).

---

## 📐 Konsep Refresh Token Rotation (RTR) & Reuse Detection
Secara standar, jika Refresh Token dicuri oleh peretas, peretas bisa memperbarui Access Token selamanya tanpa sepengetahuan pemilik asli. 
* **Dengan RTR:** Setiap kali user mengirimkan Refresh Token (RT-A) untuk meminta Access Token baru, kita **juga mengganti** RT-A dengan Refresh Token baru (RT-B). RT-A ditandai sebagai `revoked` (tidak aktif).
* **Dengan Reuse Detection:** Jika RT-A yang sudah tidak aktif dikirimkan lagi ke API, sistem mendeteksi ada anomali (kemungkinan RT-A telah dicuri dan coba digunakan ulang). Sistem akan langsung **menghapus semua Refresh Token aktif** milik user tersebut di database, sehingga peretas dan pemilik asli otomatis logout seketika (menjaga keamanan).

---

## 📦 Langkah-langkah

### Step 1: Membuat Skema Migrasi Tabel Refresh Tokens
Buat file migrasi baru `db/migrations/000004_create_refresh_tokens_table.up.sql`:

```sql
CREATE TABLE refresh_tokens (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL,
    token VARCHAR(500) NOT NULL UNIQUE,
    expires_at TIMESTAMP NOT NULL,
    revoked BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
```

Buat file down migrasi `db/migrations/000004_create_refresh_tokens_table.down.sql`:
```sql
DROP TABLE IF EXISTS refresh_tokens;
```

Jalankan perintah migrasi di terminal folder `monolith/`:
```bash
migrate -path db/migrations -database "mysql://gowallet_user:gowallet_password@tcp(localhost:3306)/gowallet" up
```

### Step 2: Membuat Model Refresh Token (`internal/user/model/refresh.go`)
Buat file di `internal/user/model/refresh.go`:

```go
package model

import "time"

type RefreshToken struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	Revoked   bool      `json:"revoked"`
	CreatedAt time.Time `json:"created_at"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}
```

### Step 3: Membuat Repository Refresh Token (`internal/user/repository/refresh_repo.go`)
Buat file repository baru di `internal/user/repository/refresh_repo.go`:

```go
package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/emzhofb/gowallet/monolith/internal/user/model"
)

type RefreshTokenRepository interface {
	Create(ctx context.Context, rt *model.RefreshToken) error
	GetByToken(ctx context.Context, token string) (*model.RefreshToken, error)
	Revoke(ctx context.Context, token string) error
	RevokeAllByUserID(ctx context.Context, userID string) error
}

type mysqlRefreshTokenRepository struct {
	db *sql.DB
}

func NewMySQLRefreshTokenRepository(db *sql.DB) RefreshTokenRepository {
	return &mysqlRefreshTokenRepository{db: db}
}

func (r *mysqlRefreshTokenRepository) Create(ctx context.Context, rt *model.RefreshToken) error {
	query := `INSERT INTO refresh_tokens (id, user_id, token, expires_at, revoked) VALUES (?, ?, ?, ?, ?)`
	_, err := r.db.ExecContext(ctx, query, rt.ID, rt.UserID, rt.Token, rt.ExpiresAt, rt.Revoked)
	return err
}

func (r *mysqlRefreshTokenRepository) GetByToken(ctx context.Context, token string) (*model.RefreshToken, error) {
	query := `SELECT id, user_id, token, expires_at, revoked, created_at FROM refresh_tokens WHERE token = ?`
	rt := &model.RefreshToken{}
	err := r.db.QueryRowContext(ctx, query, token).Scan(&rt.ID, &rt.UserID, &rt.Token, &rt.ExpiresAt, &rt.Revoked, &rt.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("token not found")
		}
		return nil, err
	}
	return rt, nil
}

func (r *mysqlRefreshTokenRepository) Revoke(ctx context.Context, token string) error {
	query := `UPDATE refresh_tokens SET revoked = 1 WHERE token = ?`
	_, err := r.db.ExecContext(ctx, query, token)
	return err
}

func (r *mysqlRefreshTokenRepository) RevokeAllByUserID(ctx context.Context, userID string) error {
	query := `UPDATE refresh_tokens SET revoked = 1 WHERE user_id = ?`
	_, err := r.db.ExecContext(ctx, query, userID)
	return err
}
```

### Step 4: Implementasi Logic Token Rotation di UserService
Buka `internal/user/service/service.go`. Suntikkan `RefreshTokenRepository` ke dalam `userService`, lalu tambahkan method `RefreshToken` ke interface dan implementasinya:

```go
// Tambah di interface UserService:
// RefreshToken(ctx context.Context, oldTokenString string) (*model.LoginResponse, error)

func (s *userService) RefreshToken(ctx context.Context, oldTokenString string) (*model.LoginResponse, error) {
	// 1. Cari token di database
	rt, err := s.rtRepo.GetByToken(ctx, oldTokenString)
	if err != nil {
		return nil, customErr.NewAppError(http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", "Refresh token tidak terdaftar.")
	}

	// 2. TOKEN REUSE DETECTION: Jika token sudah di-revoke sebelumnya -> HACKER DETECTED!
	if rt.Revoked {
		// Langsung revoke seluruh sesi aktif milik user ini demi keamanan
		_ = s.rtRepo.RevokeAllByUserID(ctx, rt.UserID)
		return nil, customErr.NewAppError(http.StatusUnauthorized, "TOKEN_BREACH_DETECTED", "Sesi telah di-revoke secara massal demi alasan keamanan. Silakan login kembali.")
	}

	// 3. Cek apakah token sudah expired
	if time.Now().After(rt.ExpiresAt) {
		return nil, customErr.NewAppError(http.StatusUnauthorized, "EXPIRED_REFRESH_TOKEN", "Sesi login telah kedaluwarsa. Silakan login kembali.")
	}

	// 4. Revoke token lama
	if err := s.rtRepo.Revoke(ctx, oldTokenString); err != nil {
		return nil, customErr.ErrInternalServer
	}

	// 5. Cari detail User untuk generate JWT baru
	user, err := s.repo.GetByID(ctx, rt.UserID)
	if err != nil {
		return nil, customErr.ErrInternalServer
	}

	// 6. Generate Access Token & Refresh Token baru (Rotation)
	newAccessToken, err := auth.GenerateToken(user.ID, user.Email, 15*time.Minute)
	if err != nil {
		return nil, customErr.ErrInternalServer
	}

	newRefreshTokenString, err := auth.GenerateToken(user.ID, user.Email, 7*24*time.Hour)
	if err != nil {
		return nil, customErr.ErrInternalServer
	}

	// 7. Simpan Refresh Token baru ke Database
	newRT := &model.RefreshToken{
		ID:        uuid.New().String(),
		UserID:    user.ID,
		Token:     newRefreshTokenString,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		Revoked:   false,
	}
	if err := s.rtRepo.Create(ctx, newRT); err != nil {
		return nil, customErr.ErrInternalServer
	}

	return &model.LoginResponse{
		AccessToken:  newAccessToken,
		RefreshToken: newRefreshTokenString,
	}, nil
}
```

*Jangan lupa: Ubah juga method `Login` reguler di `userService` agar menyimpan data `RefreshToken` yang baru dibuat ke database sebelum mengembalikannya ke client.*

### Step 5: Tambahkan Handler & Daftarkan Route
Buka `internal/user/handler/handler.go`, buat handler `RefreshToken`:

```go
func (h *UserHandler) RefreshToken(c *gin.Context) {
	var req model.RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(customErr.NewAppError(http.StatusBadRequest, "INVALID_INPUT", err.Error()))
		return
	}

	resp, err := h.svc.RefreshToken(c.Request.Context(), req.RefreshToken)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    resp,
	})
}
```

Daftarkan route public di `cmd/main.go`:
```go
	r.POST("/api/v1/auth/refresh", uHandler.RefreshToken)
```

---

## ✅ Acceptance Criteria
* [ ] Tabel `refresh_tokens` terbuat sukses di database.
* [ ] Mengirim `POST /api/v1/auth/refresh` sukses mengembalikan access token & refresh token baru yang valid.
* [ ] Setelah token digunakan sekali, status token tersebut di database berubah menjadi `revoked` (tidak aktif).
* [ ] Jika client mencoba menggunakan kembali refresh token yang sudah revoked, respons HTTP mengembalikan status `401 Unauthorized` dengan kode `TOKEN_BREACH_DETECTED`, dan semua baris token milik user tersebut di database ditandai sebagai `revoked` (sukses menghentikan potensi pencurian sesi).

---

## 💡 Tips untuk Junior
* **Refresh Token Rotation (RTR) vs Static Token:** Dengan RTR, kita meminimalkan masa aktif Refresh Token yang dicuri. Jika peretas mencuri RT-A, dan pemilik asli me-refresh session lebih dulu menggunakan RT-A, maka RT-A langsung nonaktif. Saat peretas mencoba menggunakan RT-A berikutnya, Reuse Detection ter-trigger dan langsung meng-kick peretas (serta memaksa pemilik asli login ulang demi perlindungan).

---

## 📚 Referensi Belajar
* [Auth0 - Refresh Token Rotation](https://auth0.com/docs/secure/tokens/refresh-tokens/refresh-token-rotation)
* [OWASP Token Management Cheatsheet](https://cheatsheetseries.owasp.org/cheatsheets/JSON_Web_Token_for_Java_Cheat_Sheet.html)
