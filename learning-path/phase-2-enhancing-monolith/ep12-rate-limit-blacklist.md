# Episode 12: Rate Limiting & JWT Blacklisting

## 🎯 Tujuan
* Mengamankan API dari serangan *brute force* / abuse menggunakan **Rate Limiting** berbasis Redis.
* Membuat fitur **Logout** yang aman dengan mendaftarkan token aktif ke **JWT Blacklist** di Redis.
* Memodifikasi `AuthMiddleware` agar menolak request yang menggunakan token ter-blacklist.

---

## 📐 Konsep Rate Limiting & Blacklist

### 1. Sliding Window Counter Rate Limiter
Kita membatasi setiap IP address / User ID maksimal hanya boleh mengirim request sejumlah tertentu (misal: 60 request per menit). 
Kita menyimpan jumlah request di Redis menggunakan key `rate_limit:<identifier>:<minute_timestamp>` dengan TTL 1 menit. Jika nilai counter di Redis melebihi limit, kita langsung mengembalikan status `429 Too Many Requests`.

### 2. JWT Blacklist
Secara default, token JWT bersifat *stateless* (sekali dibuat, token akan selalu valid sampai waktu `exp` habis). Masalahnya: bagaimana jika user menekan tombol **Logout**? Token yang tersimpan di browser/device masih bisa dipakai orang lain jika dicuri!
* **Solusinya:** Saat logout, kita ambil token tersebut, baca waktu kedaluwarsanya, dan simpan signature/ID token tersebut ke Redis dengan key `blacklist:<token_signature>` dengan TTL sisa waktu hidup token.
* Di middleware auth, setiap ada request masuk, kita cek apakah token tersebut terdaftar di Redis blacklist. Jika ada, akses ditolak (`410 Gone / 401 Unauthorized`).

---

## 📦 Langkah-langkah

### Step 1: Membuat Rate Limiter Middleware (`internal/middleware/rate_limit.go`)
Kita akan membatasi request per IP address menggunakan Redis.

Buat file baru di `internal/middleware/rate_limit.go`:

```go
package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	customErr "github.com/emzhofb/gowallet/monolith/internal/errors"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func RateLimiter(rdb *redis.Client, limit int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		
		// Key format: rate_limit:IP:timestamp_menit
		currentTime := time.Now().Unix() / int64(window.Seconds())
		key := fmt.Sprintf("rate_limit:%s:%d", ip, currentTime)

		ctx := c.Request.Context()

		// Gunakan MULTI/EXEC untuk menaikkan counter & set TTL secara atomik
		pipe := rdb.Pipeline()
		incr := pipe.Incr(ctx, key)
		pipe.Expire(ctx, key, window*2) // Simpan sedikit lebih lama untuk keamanan margin

		_, err := pipe.Exec(ctx)
		if err != nil {
			c.Error(customErr.ErrInternalServer)
			c.Abort()
			return
		}

		count := incr.Val()
		if count > int64(limit) {
			c.Error(customErr.NewAppError(http.StatusTooManyRequests, "RATE_LIMIT_EXCEEDED", "Request terlalu padat. Silakan coba lagi nanti."))
			c.Abort()
			return
		}

		c.Next()
	}
}
```

### Step 2: Implementasi Blacklist di AuthMiddleware
Edit `internal/middleware/auth.go` untuk menyuntikkan `*redis.Client` dan memeriksa status blacklist:

```go
package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/emzhofb/gowallet/monolith/internal/auth"
	customErr "github.com/emzhofb/gowallet/monolith/internal/errors"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func AuthMiddleware(rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.Error(customErr.NewAppError(http.StatusUnauthorized, "MISSING_TOKEN", "Token otorisasi diperlukan."))
			c.Abort()
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.Error(customErr.NewAppError(http.StatusUnauthorized, "INVALID_TOKEN_FORMAT", "Format token harus Bearer <token>."))
			c.Abort()
			return
		}

		tokenString := parts[1]

		// 1. Cek apakah token masuk dalam blacklist di Redis
		blacklistKey := fmt.Sprintf("blacklist:%s", tokenString)
		exists, err := rdb.Exists(c.Request.Context(), blacklistKey).Result()
		if err == nil && exists > 0 {
			c.Error(customErr.NewAppError(http.StatusUnauthorized, "TOKEN_REVOKED", "Sesi login telah berakhir. Silakan login kembali."))
			c.Abort()
			return
		}

		// 2. Validasi Token
		claims, err := auth.ValidateToken(tokenString)
		if err != nil {
			c.Error(customErr.NewAppError(http.StatusUnauthorized, "EXPIRED_OR_INVALID_TOKEN", "Token kedaluwarsa atau tidak valid."))
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("email", claims.Email)
		c.Set("token_string", tokenString) // Simpan token string untuk kebutuhan logout

		c.Next()
	}
}
```

### Step 3: Membuat Endpoint Logout
Tambahkan fungsi Logout di `UserService` yang akan mencatatkan token saat ini ke Redis dengan TTL sisa waktu kedaluwarsa token.

Buka `internal/user/service/service.go`. Tambahkan `*redis.Client` ke struct `userService` dan inisialisasinya, lalu tambahkan method `Logout`:

```go
// Tambahkan di interface UserService:
// Logout(ctx context.Context, tokenString string) error

func (s *userService) Logout(ctx context.Context, tokenString string) error {
	// 1. Validasi token untuk membaca claims (khususnya Expire Time)
	claims, err := auth.ValidateToken(tokenString)
	if err != nil {
		return errors.New("invalid token")
	}

	// 2. Hitung sisa waktu aktif token
	expirationTime := claims.ExpiresAt.Time
	timeLeft := time.Until(expirationTime)

	if timeLeft <= 0 {
		return nil // Token memang sudah expired, tidak perlu di-blacklist
	}

	// 3. Masukkan ke Redis Blacklist
	blacklistKey := fmt.Sprintf("blacklist:%s", tokenString)
	err = s.rdb.Set(ctx, blacklistKey, "logged_out", timeLeft).Err()
	if err != nil {
		return customErr.ErrInternalServer
	}

	return nil
}
```

Tambahkan handler logout di `internal/user/handler/handler.go`:

```go
func (h *UserHandler) Logout(c *gin.Context) {
	// Ambil token string dari context auth middleware
	tokenString, _ := c.Get("token_string")

	err := h.svc.Logout(c.Request.Context(), tokenString.(string))
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Berhasil logout. Sesi token telah dinonaktifkan.",
	})
}
```

### Step 4: Register Route di `cmd/main.go`
Suntikkan parameter `rdb` ke middleware-middleware baru:

```go
    // ...
	// 2. Setup Gin Router
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.ErrorHandler())
	
	// Terapkan Rate Limiter Global (Maksimal 60 request per menit per IP)
	r.Use(middleware.RateLimiter(rdb, 60, time.Minute))

	// Routes Public
	r.POST("/api/v1/users/register", uHandler.Register)
	r.POST("/api/v1/users/login", uHandler.Login)

	// Routes Protected
	protected := r.Group("/api/v1")
	protected.Use(middleware.AuthMiddleware(rdb)) // Inject Redis ke AuthMiddleware
	{
		// ...
		protected.POST("/users/logout", uHandler.Logout) // Endpoint logout baru
		protected.GET("/wallets/me", wHandler.GetMyWallet)
		protected.POST("/transactions/transfer", txHandler.Transfer)
		protected.GET("/transactions/history", txHandler.GetHistory)
	}
    // ...
```

---

## ✅ Acceptance Criteria
* [ ] Melakukan request berturut-turut melebihi limit (misal > 60 kali) menghasilkan HTTP `429 Too Many Requests`.
* [ ] Memanggil `POST /api/v1/users/logout` sukses mengembalikan pesan sukses.
* [ ] Mencoba mengakses `/api/v1/wallets/me` kembali menggunakan token yang baru saja di-logout mengembalikan status `401 Unauthorized` dengan kode error `TOKEN_REVOKED`.

---

## 💡 Tips untuk Junior
* **Redis TTL:** Sangat penting memberikan TTL (Time To Live) saat mem-blacklist token. Jika kita tidak memberikan TTL, data blacklist akan terus bertumpuk di memori Redis selamanya, menyebabkan RAM server membengkak (*memory leak*). Dengan menyetel TTL = sisa waktu expired token, Redis otomatis menghapus key tersebut setelah token kedaluwarsa secara alami.

---

## 📚 Referensi Belajar
* [Token Blacklisting Strategies](https://owasp.org/www-community/Source_Code_Analysis_Tools)
* [Redis Pipeline documentation](https://redis.io/docs/manual/pipelining/)
