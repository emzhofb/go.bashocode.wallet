# Episode 17: Role-Based Access Control (RBAC)

## 🎯 Tujuan
* Menambahkan kolom `role` pada tabel `users` untuk membedakan antara pengguna biasa (`user`) dan administrator (`admin`).
* Menyisipkan klaim `role` ke dalam JWT payload.
* Membuat middleware otorisasi **`RequireRole`** untuk melindungi endpoint administratif (seperti melihat log audit, menonaktifkan akun user, dll).

---

## 📐 Konsep RBAC (Role-Based Access Control)
Setelah user berhasil diautentikasi (kita tahu *siapa* mereka lewat token JWT), langkah selanjutnya adalah **Otorisasi** (kita mengecek *apa saja* hak akses yang mereka miliki).
* **Role-Based:** Kita menempelkan peran (*role*) tertentu pada identitas user.
  * Role `user` ➔ hanya boleh melihat wallet sendiri dan melakukan transfer.
  * Role `admin` ➔ boleh melihat semua wallet user, mem-freeze akun, dan melihat log sistem.
* **Claims-Based:** Memasukkan informasi role di dalam token JWT (`role: "admin"`) mempermudah API Gateway atau Middleware mengecek hak akses tanpa perlu melakukan query database berulang-ulang (*stateless authorization*).

---

## 📦 Langkah-langkah

### Step 1: Membuat Skema Migrasi untuk Role
Buat file migrasi baru `db/migrations/000005_add_role_to_users.up.sql`:

```sql
ALTER TABLE users ADD COLUMN role VARCHAR(20) NOT NULL DEFAULT 'user';
```

Buat file down migrasi `db/migrations/000005_add_role_to_users.down.sql`:
```sql
ALTER TABLE users DROP COLUMN role;
```

Jalankan migrasi di terminal:
```bash
migrate -path db/migrations -database "mysql://gowallet_user:gowallet_password@tcp(localhost:3306)/gowallet" up
```

### Step 2: Update Model & JWT Claims
Buka file `internal/user/model/user.go`, tambahkan kolom `Role` ke dalam struct `User`:

```go
type User struct {
	ID           string     `json:"id"`
	FullName     string     `json:"full_name"`
	Email        string     `json:"email"`
	Role         string     `json:"role"` // user, admin
	PasswordHash string     `json:"-"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
}
```

Buka `internal/auth/jwt.go`, tambahkan field `Role` ke dalam struct `JWTClaims`:

```go
type JWTClaims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"` // Tambah di claims
	jwt.RegisteredClaims
}

func GenerateToken(userID string, email string, role string, duration time.Duration) (string, error) {
	claims := &JWTClaims{
		UserID: userID,
		Email:  email,
		Role:   role, // Set value role
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(duration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}
```

*Sesuaikan pemanggilan `GenerateToken` di `UserService.Register` dan `UserService.Login` dengan meneruskan variabel `user.Role`.*

### Step 3: Membuat Middleware Otorisasi `RequireRole`
Middleware ini akan memeriksa klaim `role` yang disimpan di context Gin oleh `AuthMiddleware`. Jika role user tidak ada dalam daftar yang diizinkan, kembalikan HTTP `403 Forbidden`.

Buat file baru di `internal/middleware/role.go`:

```go
package middleware

import (
	"net/http"

	customErr "github.com/emzhofb/gowallet/monolith/internal/errors"
	"github.com/gin-gonic/gin"
)

func RequireRole(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Ambil role yang disisipkan oleh AuthMiddleware
		userRole, exists := c.Get("role")
		if !exists {
			c.Error(customErr.NewAppError(http.StatusForbidden, "ACCESS_DENIED", "Akses ditolak. Peran tidak ditemukan."))
			c.Abort()
			return
		}

		// Cocokkan apakah role user terdaftar dalam allowedRoles
		roleStr := userRole.(string)
		isAllowed := false
		for _, role := range allowedRoles {
			if roleStr == role {
				isAllowed = true
				break
			}
		}

		if !isAllowed {
			c.Error(customErr.NewAppError(http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "Anda tidak memiliki hak akses untuk halaman ini."))
			c.Abort()
			return
		}

		c.Next()
	}
}
```

*Catatan: Pastikan di `internal/middleware/auth.go` (AuthMiddleware) Anda menyisipkan klaim role ke context: `c.Set("role", claims.Role)`.*

### Step 4: Membuat Endpoint Khusus Admin
Mari buat endpoint simulasi admin-only di `cmd/main.go` yang dilindungi oleh middleware `RequireRole`:

```go
    // ...
	// Routes Protected (Umum)
	protected := r.Group("/api/v1")
	protected.Use(middleware.AuthMiddleware(rdb))
	{
		protected.GET("/wallets/me", wHandler.GetMyWallet)
		
		// Group Route Khusus Admin (Hanya admin yang boleh masuk)
		adminOnly := protected.Group("/admin")
		adminOnly.Use(middleware.RequireRole("admin")) // Proteksi RBAC
		{
			adminOnly.GET("/users", func(c *gin.Context) {
				// Simulasi: Admin bisa melihat semua user (biasanya panggil user service)
				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"message": "Halo Admin! Anda berhasil mengakses data panel kontrol.",
				})
			})
		}
	}
    // ...
```

---

## ✅ Acceptance Criteria
* [ ] Kolom `role` terbuat di tabel `users` MySQL.
* [ ] Login mengembalikan JWT Token yang memuat klaim `"role": "user"` atau `"role": "admin"`.
* [ ] User biasa mencoba mengakses `GET /api/v1/admin/users` ditolak dengan status HTTP `403 Forbidden` dan menampilkan error `INSUFFICIENT_PERMISSIONS`.
* [ ] User dengan role `admin` (diubah via database update `users` set `role = 'admin'`) sukses masuk ke endpoint admin.

---

## 💡 Tips untuk Junior
* **Stateless Authorization:** Menyimpan role di JWT membuat otorisasi bersifat cepat (*stateless*) karena server tidak perlu bolak-balik query database ke tabel user hanya untuk mengecek role di setiap request. Namun ingat, jika role user diubah oleh admin di database, perubahan tersebut baru akan aktif setelah JWT token lama user kedaluwarsa dan di-refresh.

---

## 📚 Referensi Belajar
* [OWASP Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)
* [Role-Based Access Control (RBAC) Overview](https://en.wikipedia.org/wiki/Role-based_access_control)
