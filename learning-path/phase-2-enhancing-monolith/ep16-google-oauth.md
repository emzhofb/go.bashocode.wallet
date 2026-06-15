# Episode 16: Integrasi Google OAuth

## 🎯 Tujuan
* Memahami konsep alur **OAuth 2.0** (Authorization Code Grant).
* Mengintegrasikan library `golang.org/x/oauth2` ke dalam aplikasi Go.
* Membuat REST API endpoint untuk:
  * Redirect user ke layar login Google (`/api/v1/auth/google`).
  * Menerima callback dari Google, menukarkan *auth code* dengan token, mengambil profil user Google, dan otomatis me-register/login user tersebut di platform kita (`/api/v1/auth/google/callback`).

---

## 📐 Konsep Alur OAuth 2.0
OAuth 2.0 memungkinkan user login ke aplikasi kita menggunakan akun Google mereka secara aman tanpa menyerahkan password Google mereka kepada kita.

```
1. User ➔ GET /auth/google ➔ Redirect ke Google Consent Screen
2. User menyetujui akses ➔ Google redirect kembali ke /auth/google/callback?code=AUTH_CODE
3. Aplikasi kita ➔ Kirim AUTH_CODE ke Google Token Server ➔ Google kirim ACCESS_TOKEN
4. Aplikasi kita ➔ Gunakan ACCESS_TOKEN untuk request detail profile (Email & Nama) dari Google API
5. Aplikasi kita ➔ Cari/Daftarkan user di DB lokal ➔ Hasilkan JWT Access & Refresh Token platform kita
```

---

## 📦 Langkah-langkah

### Step 1: Install Package OAuth2
Unduh library resmi OAuth2 untuk Go:
```bash
go get golang.org/x/oauth2
go get golang.org/x/oauth2/google
```

### Step 2: Konfigurasi Kredensial di `.env`
Buka file `.env` di folder root monolith, tambahkan kredensial Google Client ID (dapat digenerate dari Google Cloud Console):

```env
GOOGLE_CLIENT_ID=your-client-id-from-google-cloud
GOOGLE_CLIENT_SECRET=your-client-secret-from-google-cloud
GOOGLE_REDIRECT_URL=http://localhost:8080/api/v1/auth/google/callback
```

Suaikan `internal/config/config.go` agar membaca 3 konfigurasi baru tersebut.

### Step 3: Membuat OAuth Config Helper (`internal/auth/oauth.go`)
Buat file helper konfigurasi OAuth di `internal/auth/oauth.go`:

```go
package auth

import (
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var GoogleOAuthConfig *oauth2.Config

func InitOAuthConfig() {
	GoogleOAuthConfig = &oauth2.Config{
		ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		RedirectURL:  os.Getenv("GOOGLE_REDIRECT_URL"),
		Scopes: []string{
			"https://www.googleapis.com/auth/userinfo.profile",
			"https://www.googleapis.com/auth/userinfo.email",
		},
		Endpoint: google.Endpoint,
	}
}
```

Pastikan untuk memanggil `auth.InitOAuthConfig()` di `cmd/main.go` saat startup.

### Step 4: Menambahkan Handler Google Redirect & Callback
Buka `internal/user/handler/handler.go`. Tambahkan handler baru untuk `/auth/google` (mengarahkan user ke Google) dan `/auth/google/callback`:

```go
package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/emzhofb/gowallet/monolith/internal/auth"
	customErr "github.com/emzhofb/gowallet/monolith/internal/errors"
	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
)

func (h *UserHandler) GoogleLoginRedirect(c *gin.Context) {
	// Generate URL ke Google consent screen
	// State string digunakan untuk perlindungan CSRF (di production, buat string random)
	url := auth.GoogleOAuthConfig.AuthCodeURL("random-state-string", oauth2.AccessTypeOffline)
	c.Redirect(http.StatusTemporaryRedirect, url)
}

func (h *UserHandler) GoogleCallback(c *gin.Context) {
	state := c.Query("state")
	if state != "random-state-string" {
		c.Error(customErr.NewAppError(http.StatusBadRequest, "INVALID_STATE", "State CSRF token tidak valid."))
		return
	}

	code := c.Query("code")
	if code == "" {
		c.Error(customErr.NewAppError(http.StatusBadRequest, "MISSING_CODE", "Auth code Google tidak ditemukan."))
		return
	}

	// 1. Tukarkan auth code dengan Google Access Token
	token, err := auth.GoogleOAuthConfig.Exchange(c.Request.Context(), code)
	if err != nil {
		c.Error(customErr.NewAppError(http.StatusInternalServerError, "OAUTH_EXCHANGE_FAILED", err.Error()))
		return
	}

	// 2. Gunakan token untuk mengambil profil user dari Google API
	client := auth.GoogleOAuthConfig.Client(c.Request.Context(), token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		c.Error(customErr.ErrInternalServer)
		return
	}
	defer resp.Body.Close()

	var googleUser struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&googleUser); err != nil {
		c.Error(customErr.ErrInternalServer)
		return
	}

	// 3. Cari user di DB berdasarkan email Google
	// Jika belum terdaftar -> otomatis daftarkan (Register)
	// Jika sudah ada -> langsung buatkan login token JWT kita
	loginResponse, err := h.svc.LoginOrRegisterOAuth(c.Request.Context(), googleUser.Email, googleUser.Name)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    loginResponse,
	})
}
```

### Step 5: Membuat Method LoginOrRegisterOAuth di UserService
Buka `internal/user/service/service.go`. Tambahkan method baru ke interface & implementasinya:

```go
// Tambah di interface UserService:
// LoginOrRegisterOAuth(ctx context.Context, email string, name string) (*model.LoginResponse, error)

func (s *userService) LoginOrRegisterOAuth(ctx context.Context, email string, name string) (*model.LoginResponse, error) {
	// 1. Cari user di DB berdasarkan email Google
	user, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		// User belum terdaftar -> Daftarkan user baru (Register otomatis)
		// Karena lewat Google OAuth, kita generate password random/acak karena password login reguler dinonaktifkan untuk user ini
		randomPassword := uuid.New().String()
		hashedBytes, _ := bcrypt.GenerateFromPassword([]byte(randomPassword), bcrypt.DefaultCost)

		user = &model.User{
			ID:           uuid.New().String(),
			FullName:     name,
			Email:        email,
			PasswordHash: string(hashedBytes),
		}

		// Mulai transaksi DB untuk buat user + buat wallet default (Ep 6)
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, customErr.ErrInternalServer
		}
		defer tx.Rollback()

		if err := s.userRepo.CreateTx(ctx, tx, user); err != nil {
			return nil, customErr.ErrInternalServer
		}

		// Panggil Wallet Repository untuk create wallet default (Ep 6)
		// ... walletRepo.CreateTx ...

		if err := tx.Commit(); err != nil {
			return nil, customErr.ErrInternalServer
		}
	}

	// 2. Generate token JWT kita (Access Token + Refresh Token)
	accessToken, err := auth.GenerateToken(user.ID, user.Email, 15*time.Minute)
	if err != nil {
		return nil, customErr.ErrInternalServer
	}

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

### Step 6: Daftarkan Route Baru
Daftarkan endpoints baru ini ke router Gin di `cmd/main.go`:
```go
	r.GET("/api/v1/auth/google", uHandler.GoogleLoginRedirect)
	r.GET("/api/v1/auth/google/callback", uHandler.GoogleCallback)
```

---

## ✅ Acceptance Criteria
* [ ] Memanggil endpoint `GET /api/v1/auth/google` sukses me-redirect user ke Google Accounts sign-in page.
* [ ] Callback url `GET /api/v1/auth/google/callback` berhasil menangkap param code, menukarkan token, mengambil profile, dan mengembalikan JWT Token platform kita.
* [ ] User baru otomatis terdaftar di database MySQL beserta wallet aktif bersaldo `0.00` setelah login via Google sukses.

---

## 💡 Tips untuk Junior
* **State Parameter CSRF:** Selalu gunakan state token unik yang digenerate acak (misal: UUID atau hash string) saat me-redirect user ke Google, dan simpan di cookies/session lokal browser. Saat callback diterima, pastikan state dari Google sama dengan state lokal kita. Ini sangat penting untuk mencegah serangan **Cross-Site Request Forgery (CSRF)**.

---

## 📚 Referensi Belajar
* [OAuth 2.0 Simplified](https://aaronparecki.com/oauth-2-simplified/)
* [Go golang.org/x/oauth2 package documentation](https://pkg.go.dev/golang.org/x/oauth2)
