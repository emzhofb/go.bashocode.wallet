# Episode 6: Wallet Management & Balance Check

## 🎯 Tujuan
* Otomatis membuat `wallet` baru saat user berhasil mendaftar (Register).
* Memahami pentingnya **Database Transaction** (`tx.Begin()`) untuk operasi multi-tabel agar data konsisten (tidak boleh ada user tanpa wallet).
* Membuat endpoint `/wallets/me` untuk memeriksa saldo dan status wallet user saat ini.

---

## 📐 Kenapa Butuh Database Transaction?
Pada episode ini, proses registrasi user berubah menjadi:
1. Simpan data user ke tabel `users`.
2. Buat data wallet default untuk user tersebut di tabel `wallets` dengan saldo `0.00`.

Jika kita melakukan langkah tersebut secara terpisah tanpa transaction, dan tiba-tiba server mati atau koneksi putus tepat setelah langkah 1 selesai, maka kita akan memiliki **data sampah (orphan record)**: user terdaftar tapi tidak punya wallet!

Dengan Database Transaction (`sql.Tx`), jika langkah 2 gagal, langkah 1 otomatis dibatalkan (**Rollback**). Data hanya tersimpan jika semua langkah sukses (**Commit**).

---

## 📦 Langkah-langkah

### Step 1: Membuat Folder Layer Wallet
Buat struktur folder baru di dalam `internal/`:
```bash
mkdir -p internal/wallet/model internal/wallet/repository internal/wallet/service internal/wallet/handler
```

### Step 2: Membuat Model Wallet (`internal/wallet/model/wallet.go`)
```go
package model

import "time"

type Wallet struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Balance   float64   `json:"balance"`
	Currency  string    `json:"currency"`
	Status    string    `json:"status"` // active, frozen
	Version   int       `json:"-"`      // Digunakan untuk concurrency control
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
```

### Step 3: Membuat Repository Wallet (`internal/wallet/repository/repository.go`)
Repository wallet bertugas menyimpan wallet baru dan mencari data wallet berdasarkan user ID.
*Penting: Kita membutuhkan dua versi `Create` — satu yang menerima database koneksi biasa (`*sql.DB`) dan satu lagi yang menerima koneksi transaksi (`*sql.Tx`). Namun, praktek terbaik di Go adalah dengan meneruskan database transaction lewat context, atau menggunakan interface pembungkus. Untuk simplicity tingkat junior, kita buat method yang menerima `*sql.Tx` langsung.*

```go
package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/emzhofb/gowallet/monolith/internal/wallet/model"
)

type WalletRepository interface {
	CreateTx(ctx context.Context, tx *sql.Tx, w *model.Wallet) error
	GetByUserID(ctx context.Context, userID string) (*model.Wallet, error)
}

type mysqlWalletRepository struct {
	db *sql.DB
}

func NewMySQLWalletRepository(db *sql.DB) WalletRepository {
	return &mysqlWalletRepository{db: db}
}

func (r *mysqlWalletRepository) CreateTx(ctx context.Context, tx *sql.Tx, w *model.Wallet) error {
	query := `INSERT INTO wallets (id, user_id, balance, currency, status) VALUES (?, ?, ?, ?, ?)`
	_, err := tx.ExecContext(ctx, query, w.ID, w.UserID, w.Balance, w.Currency, w.Status)
	return err
}

func (r *mysqlWalletRepository) GetByUserID(ctx context.Context, userID string) (*model.Wallet, error) {
	query := `SELECT id, user_id, balance, currency, status, version, created_at, updated_at FROM wallets WHERE user_id = ?`
	w := &model.Wallet{}
	err := r.db.QueryRowContext(ctx, query, userID).Scan(&w.ID, &w.UserID, &w.Balance, &w.Currency, &w.Status, &w.Version, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("wallet not found")
		}
		return nil, err
	}
	return w, nil
}
```

### Step 4: Modifikasi UserRepository untuk Support Transaction
Agar bisa menyimpan data User di dalam rangkaian transaksi yang sama, kita perlu menambahkan method `CreateTx` di `UserRepository`.

Buka `internal/user/repository/repository.go`, tambahkan method berikut ke interface dan implementasinya:

```go
// Tambah di interface UserRepository:
// CreateTx(ctx context.Context, tx *sql.Tx, u *model.User) error

func (r *mysqlUserRepository) CreateTx(ctx context.Context, tx *sql.Tx, u *model.User) error {
	query := `INSERT INTO users (id, full_name, email, password_hash) VALUES (?, ?, ?, ?)`
	_, err := tx.ExecContext(ctx, query, u.ID, u.FullName, u.Email, u.PasswordHash)
	return err
}
```

### Step 5: Menggabungkan Transaksi saat Register di UserService
Buka `internal/user/service/service.go`. Kita perlu menyuntikkan `WalletRepository` ke `userService` dan memodifikasi method `Register` agar membuka transaksi database.
*Penting: Untuk membuka transaksi, kita butuh akses ke objek `*sql.DB`. Kita bisa menyuntikkan `*sql.DB` langsung ke service.*

```go
package service

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/emzhofb/gowallet/monolith/internal/auth"
	customErr "github.com/emzhofb/gowallet/monolith/internal/errors"
	"github.com/emzhofb/gowallet/monolith/internal/user/model"
	userRepo "github.com/emzhofb/gowallet/monolith/internal/user/repository"
	walletModel "github.com/emzhofb/gowallet/monolith/internal/wallet/model"
	walletRepo "github.com/emzhofb/gowallet/monolith/internal/wallet/repository"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type userService struct {
	db         *sql.DB
	userRepo   userRepo.UserRepository
	walletRepo walletRepo.WalletRepository
}

func NewUserService(db *sql.DB, uRepo userRepo.UserRepository, wRepo walletRepo.WalletRepository) UserService {
	return &userService{
		db:         db,
		userRepo:   uRepo,
		walletRepo: wRepo,
	}
}

func (s *userService) Register(ctx context.Context, req model.CreateUserRequest) (*model.User, error) {
	existing, _ := s.userRepo.GetByEmail(ctx, req.Email)
	if existing != nil {
		return nil, customErr.NewAppError(http.StatusConflict, "EMAIL_ALREADY_REGISTERED", "Email ini sudah terdaftar.")
	}

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

	// 1. Mulai Transaksi Database
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, customErr.ErrInternalServer
	}
	// Pastikan kita melakukan rollback jika terjadi kepanikan atau error tengah jalan
	defer tx.Rollback()

	// 2. Simpan User menggunakan koneksi Tx
	if err := s.userRepo.CreateTx(ctx, tx, user); err != nil {
		return nil, customErr.ErrInternalServer
	}

	// 3. Buat Wallet baru di dalam rangkaian transaksi yang sama
	wallet := &walletModel.Wallet{
		ID:       uuid.New().String(),
		UserID:   user.ID,
		Balance:  0.00,
		Currency: "IDR",
		Status:   "active",
	}

	if err := s.walletRepo.CreateTx(ctx, tx, wallet); err != nil {
		return nil, customErr.ErrInternalServer
	}

	// 4. Commit Transaksi jika semua langkah berhasil
	if err := tx.Commit(); err != nil {
		return nil, customErr.ErrInternalServer
	}

	return s.userRepo.GetByID(ctx, user.ID)
}

// ... method lainnya ...
```

### Step 6: Membuat Service & Handler Wallet (`internal/wallet/`)
Buat file `internal/wallet/service/service.go`:

```go
package service

import (
	"context"
	"net/http"

	customErr "github.com/emzhofb/gowallet/monolith/internal/errors"
	"github.com/emzhofb/gowallet/monolith/internal/wallet/model"
	"github.com/emzhofb/gowallet/monolith/internal/wallet/repository"
)

type WalletService interface {
	GetWalletByUserID(ctx context.Context, userID string) (*model.Wallet, error)
}

type walletService struct {
	repo repository.WalletRepository
}

func NewWalletService(repo repository.WalletRepository) WalletService {
	return &walletService{repo: repo}
}

func (s *walletService) GetWalletByUserID(ctx context.Context, userID string) (*model.Wallet, error) {
	w, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, customErr.NewAppError(http.StatusNotFound, "WALLET_NOT_FOUND", "Wallet pengguna tidak ditemukan.")
	}
	return w, nil
}
```

Buat file `internal/wallet/handler/handler.go`:

```go
package handler

import (
	"net/http"

	"github.com/emzhofb/gowallet/monolith/internal/wallet/service"
	"github.com/gin-gonic/gin"
)

type WalletHandler struct {
	svc service.WalletService
}

func NewWalletHandler(svc service.WalletService) *WalletHandler {
	return &WalletHandler{svc: svc}
}

func (h *WalletHandler) GetMyWallet(c *gin.Context) {
	// Ambil user_id dari context jwt middleware
	userID, _ := c.Get("user_id")

	wallet, err := h.svc.GetWalletByUserID(c.Request.Context(), userID.(string))
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    wallet,
	})
}
```

### Step 7: Update `cmd/main.go`
Inisialisasi `WalletRepository`, `WalletService`, dan `WalletHandler`. Kirim pointer `db` ke `userService`. Lalu daftarkan endpoint `/wallets/me` di dalam group route yang dilindungi oleh Auth Middleware.

```go
    // ...
	// 1. Inisialisasi Layer
	uRepo := userRepository.NewMySQLUserRepository(db)
	wRepo := walletRepository.NewMySQLWalletRepository(db)
	
	// Inject DB ke User Service untuk transaksi
	uSvc := userService.NewUserService(db, uRepo, wRepo) 
	wSvc := walletService.NewWalletService(wRepo)

	uHandler := userHandler.NewUserHandler(uSvc)
	wHandler := walletHandler.NewWalletHandler(wSvc)
    
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.ErrorHandler())

	// Routes Public
	r.POST("/api/v1/users/register", uHandler.Register)
	r.POST("/api/v1/users/login", uHandler.Login)

	// Routes Protected
	protected := r.Group("/api/v1")
	protected.Use(middleware.AuthMiddleware())
	{
		protected.GET("/users/me", func(c *gin.Context) {
			userID, _ := c.Get("user_id")
			user, err := uSvc.GetProfile(c.Request.Context(), userID.(string))
			if err != nil {
				c.Error(err)
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "data": user})
		})

		// Endpoint Wallet Baru
		protected.GET("/wallets/me", wHandler.GetMyWallet)
	}
    // ...
```

---

## ✅ Acceptance Criteria
* [ ] Registrasi user baru otomatis membuat satu record baru di tabel `wallets` dengan user ID yang bersangkutan dan balance `0.00`.
* [ ] Jika proses pembuatan wallet gagal (misalnya kita sengaja mematikan database di tengah jalan atau melempar panic sebelum commit), data user tidak boleh tersimpan di MySQL (Rollback sukses).
* [ ] Melakukan `GET /api/v1/wallets/me` dengan menyertakan token otorisasi mengembalikan respons data wallet beserta saldonya.

---

## 💡 Tips untuk Junior
* **Selalu defer rollback:** Menulis `defer tx.Rollback()` setelah `db.BeginTx` sangat aman. Jika transaksi berakhir sukses (`tx.Commit()`), pemanggilan `tx.Rollback()` di akhir fungsi otomatis diabaikan oleh Go. Namun jika terjadi panic di tengah jalan, `Rollback()` akan secara otomatis membersihkan sisa transaksi yang menggantung agar database tidak terkunci (*deadlock*).

---

## 📚 Referensi Belajar
* [Go database/sql Transactions](https://go.dev/doc/database/execute-transactions)
* [ACID Transactions database principles](https://en.wikipedia.org/wiki/ACID)
