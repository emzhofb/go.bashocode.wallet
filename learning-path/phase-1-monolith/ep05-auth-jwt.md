# Episode 5: Authentication dengan JWT & Bcrypt

## 🎯 Tujuan
* Mengamankan password user menggunakan algoritma hashing **Bcrypt** sebelum disimpan ke database.
* Membuat API Login yang memvalidasi email/password dan menghasilkan **JWT Access Token** serta **Refresh Token**.
* Membuat **Auth Middleware** untuk mengamankan route sensitif (seperti update profil, mutasi wallet, dll).

---

## 📦 Langkah-langkah

### Step 1: Install Dependencies
Kita butuh library `golang.org/x/crypto/bcrypt` untuk hashing password, dan `golang-jwt/jwt/v5` untuk generate token.

```bash
go get golang.org/x/crypto/bcrypt
go get github.com/golang-jwt/jwt/v5
```

### Step 2: Update Schema Registrasi (Hash Password)
Buka file `internal/user/service/service.go`. Ubah bagian pembuatan user agar menggunakan bcrypt untuk hashing password:

```go
package service

import (
	"context"
	"errors"
	"net/http"

	customErr "github.com/emzhofb/gowallet/monolith/internal/errors"
	"github.com/emzhofb/gowallet/monolith/internal/user/model"
	"github.com/emzhofb/gowallet/monolith/internal/user/repository"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// ...

func (s *userService) Register(ctx context.Context, req model.CreateUserRequest) (*model.User, error) {
	existing, _ := s.repo.GetByEmail(ctx, req.Email)
	if existing != nil {
		return nil, customErr.NewAppError(http.StatusConflict, "EMAIL_ALREADY_REGISTERED", "Email ini sudah terdaftar.")
	}

	// 1. Hash password dengan bcrypt
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, customErr.ErrInternalServer
	}

	user := &model.User{
		ID:           uuid.New().String(),
		FullName:     req.FullName,
		Email:        req.Email,
		PasswordHash: string(hashedBytes),
	}

	if err := s.repo.Create(ctx, user); err != nil {
		return nil, customErr.ErrInternalServer
	}

	return s.repo.GetByID(ctx, user.ID)
}
```

### Step 3: Membuat JWT Helper (`internal/auth/jwt.go`)
Kita akan membuat token helper untuk men-generate Access Token (masa aktif 15 menit) dan Refresh Token (masa aktif 7 hari).

Buat file di `internal/auth/jwt.go`:

```go
package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type JWTClaims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

var jwtSecret = []byte("super-secret-key-change-this-in-production")

func GenerateToken(userID string, email string, duration time.Duration) (string, error) {
	claims := &JWTClaims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(duration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ID:        userID + "-" + time.Now().Format("20060102150405"),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

func ValidateToken(tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(t *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}

	return claims, nil
}
```

### Step 4: Membuat Endpoint Login (`internal/user/model/auth.go` & `internal/user/service/` & `internal/user/handler/`)
Buat DTO request & response login di `internal/user/model/auth.go`:
```go
package model

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}
```

Tambahkan method Login di `internal/user/service/service.go`:
```go
// Tambah di interface UserService:
// Login(ctx context.Context, req model.LoginRequest) (*model.LoginResponse, error)

func (s *userService) Login(ctx context.Context, req model.LoginRequest) (*model.LoginResponse, error) {
	// 1. Cari user berdasarkan email
	user, err := s.repo.GetByEmail(ctx, req.Email)
	if err != nil {
		return nil, customErr.NewAppError(http.StatusUnauthorized, "INVALID_CREDENTIALS", "Email atau password salah.")
	}

	// 2. Verifikasi hash password
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password))
	if err != nil {
		return nil, customErr.NewAppError(http.StatusUnauthorized, "INVALID_CREDENTIALS", "Email atau password salah.")
	}

	// 3. Generate Access Token (15 menit)
	accessToken, err := auth.GenerateToken(user.ID, user.Email, 15*time.Minute)
	if err != nil {
		return nil, customErr.ErrInternalServer
	}

	// 4. Generate Refresh Token (7 hari)
	refreshToken, err := auth.GenerateToken(user.ID, user.Email, 7*24*time.Hour)
	if err != nil {
		return nil, customErr.ErrInternalServer
	}

	return &model.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}
```

Tambahkan handler login di `internal/user/handler/handler.go`:
```go
func (h *UserHandler) Login(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(customErr.NewAppError(http.StatusBadRequest, "INVALID_INPUT", err.Error()))
		return
	}

	resp, err := h.svc.Login(c.Request.Context(), req)
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

### Step 5: Membuat Auth Middleware (`internal/middleware/auth.go`)
Middleware ini akan memeriksa header `Authorization: Bearer <token>`, memvalidasinya, dan menyisipkan `user_id` ke dalam Gin Context agar endpoint selanjutnya tahu siapa user yang sedang mengakses.

Buat file baru di `internal/middleware/auth.go`:

```go
package middleware

import (
	"net/http"
	"strings"

	"github.com/emzhofb/gowallet/monolith/internal/auth"
	customErr "github.com/emzhofb/gowallet/monolith/internal/errors"
	"github.com/gin-gonic/gin"
)

func AuthMiddleware() gin.HandlerFunc {
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
		claims, err := auth.ValidateToken(tokenString)
		if err != nil {
			c.Error(customErr.NewAppError(http.StatusUnauthorized, "EXPIRED_OR_INVALID_TOKEN", "Token kedaluwarsa atau tidak valid."))
			c.Abort()
			return
		}

		// Simpan user_id dan email di context Gin
		c.Set("user_id", claims.UserID)
		c.Set("email", claims.Email)

		c.Next()
	}
}
```

### Step 6: Mendaftarkan Route Baru di `cmd/main.go`
Buka file `cmd/main.go`. Daftarkan endpoint login dan buat group endpoint `/users/me` yang dilindungi oleh `AuthMiddleware`.

```go
    // ...
	// Routes Public
	r.POST("/api/v1/users/register", uHandler.Register)
	r.POST("/api/v1/users/login", uHandler.Login)

	// Routes Protected (Hanya bisa diakses jika memiliki JWT Token)
	protected := r.Group("/api/v1")
	protected.Use(middleware.AuthMiddleware())
	{
		protected.GET("/users/me", func(c *gin.Context) {
			// Ambil user_id dari context
			userID, _ := c.Get("user_id")
			
			user, err := uSvc.GetProfile(c.Request.Context(), userID.(string))
			if err != nil {
				c.Error(err)
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data":    user,
			})
		})
	}
    // ...
```

---

## ✅ Acceptance Criteria
* [ ] Password user tersimpan di MySQL dalam bentuk ter-hash (tidak boleh plain text, biasanya diawali `$2a$`).
* [ ] Login sukses mengembalikan response JSON berisi `access_token` dan `refresh_token`.
* [ ] Mengakses endpoint `/api/v1/users/me` tanpa header `Authorization` mengembalikan response `401 Unauthorized`.
* [ ] Mengakses endpoint dengan header `Authorization: Bearer <valid_token>` berhasil mengembalikan profil user yang login.

---

## 💡 Tips untuk Junior
* **Bcrypt Cost:** Bcrypt menggunakan konsep "cost" untuk menentukan berapa kali algoritma hashing dijalankan. Nilai default (`bcrypt.DefaultCost`) adalah 10. Nilai yang lebih tinggi lebih aman tapi memakan waktu CPU lebih lama (jangan set > 14 di server production karena bisa membuat endpoint login sangat lambat).
* **JWT Secret Location:** Dalam dunia kerja nyata, **JANGAN PERNAH** menulis JWT Secret key langsung di dalam kode (*hardcoded*). Gunakan `.env` atau system environment variable dan baca nilainya saat startup aplikasi.

---

## 📚 Referensi Belajar
* [Bcrypt Wiki & Explanation](https://en.wikipedia.org/wiki/Bcrypt)
* [JWT.io - Web Token Debugger](https://jwt.io/)
* [OWASP Password Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html)
