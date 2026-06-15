# Episode 3: Clean Architecture — CRUD User

## 🎯 Tujuan
* Mengenalkan pola **Clean Architecture** (Hexagonal/Layered Architecture) yang konsisten.
* Membuat domain User yang dipisahkan menjadi:
  * **Model/Entity:** Struktur data utama.
  * **Repository Layer:** Akses ke database MySQL (raw SQL).
  * **Service/Usecase Layer:** Logika bisnis (independen dari HTTP).
  * **Handler Layer:** Menerima input HTTP (menggunakan Gin).
* Menyambungkan semua komponen (*dependency injection*) di `main.go`.

---

## 📐 Konsep Clean Architecture
Kita membagi kode menjadi 3 layer utama:
```
[ HTTP Request ] ➔ [ Handler Layer ] ➔ [ Service Layer ] ➔ [ Repository Layer ] ➔ [ Database ]
```
* **Repository:** Berhubungan langsung dengan query database. Tidak tahu apa itu HTTP status code atau business rules kompleks.
* **Service:** Pusat dari semua logika bisnis. Di sini validasi logika terjadi. Service tidak boleh tahu apakah client menggunakan REST API, gRPC, atau CLI.
* **Handler:** Hanya bertugas mem-parsing HTTP JSON request, memanggil Service, dan mem-format JSON response.

---

## 📦 Langkah-langkah

### Step 1: Install Gin Router
Unduh web framework Gin untuk Handler kita:
```bash
go get github.com/gin-gonic/gin
```

### Step 2: Buat Folder Structure Baru
Di dalam folder `monolith`, buat folder baru:
```bash
mkdir -p internal/user/handler internal/user/service internal/user/repository internal/user/model
```

### Step 3: Membuat Model User (`internal/user/model/user.go`)
```go
package model

import "time"

type User struct {
	ID           string     `json:"id"`
	FullName     string     `json:"full_name"`
	Email        string     `json:"email"`
	PasswordHash string     `json:"-"` // Jangan ekspos hash password ke JSON
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
}

type CreateUserRequest struct {
	FullName string `json:"full_name" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

type UpdateUserRequest struct {
	FullName string `json:"full_name" binding:"required"`
}
```

### Step 4: Membuat Repository Layer (`internal/user/repository/repository.go`)
Definisikan interface dan implementasinya untuk akses database:

```go
package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/emzhofb/gowallet/monolith/internal/user/model"
)

type UserRepository interface {
	Create(ctx context.Context, u *model.User) error
	GetByID(ctx context.Context, id string) (*model.User, error)
	GetByEmail(ctx context.Context, email string) (*model.User, error)
	Update(ctx context.Context, u *model.User) error
}

type mysqlUserRepository struct {
	db *sql.DB
}

func NewMySQLUserRepository(db *sql.DB) UserRepository {
	return &mysqlUserRepository{db: db}
}

func (r *mysqlUserRepository) Create(ctx context.Context, u *model.User) error {
	query := `INSERT INTO users (id, full_name, email, password_hash) VALUES (?, ?, ?, ?)`
	_, err := r.db.ExecContext(ctx, query, u.ID, u.FullName, u.Email, u.PasswordHash)
	return err
}

func (r *mysqlUserRepository) GetByID(ctx context.Context, id string) (*model.User, error) {
	query := `SELECT id, full_name, email, password_hash, created_at, updated_at FROM users WHERE id = ? AND deleted_at IS NULL`
	u := &model.User{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(&u.ID, &u.FullName, &u.Email, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("user not found")
		}
		return nil, err
	}
	return u, nil
}

func (r *mysqlUserRepository) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	query := `SELECT id, full_name, email, password_hash, created_at, updated_at FROM users WHERE email = ? AND deleted_at IS NULL`
	u := &model.User{}
	err := r.db.QueryRowContext(ctx, query, email).Scan(&u.ID, &u.FullName, &u.Email, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("user not found")
		}
		return nil, err
	}
	return u, nil
}

func (r *mysqlUserRepository) Update(ctx context.Context, u *model.User) error {
	query := `UPDATE users SET full_name = ? WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, u.FullName, u.ID)
	return err
}
```

### Step 5: Membuat Service Layer (`internal/user/service/service.go`)
Service bertugas menghandle logika bisnis. Di sini kita membuat ID (UUID) baru untuk User sebelum menyimpannya ke database.
*Unduh generator UUID:*
```bash
go get github.com/google/uuid
```

```go
package service

import (
	"context"
	"errors"

	"github.com/emzhofb/gowallet/monolith/internal/user/model"
	"github.com/emzhofb/gowallet/monolith/internal/user/repository"
	"github.com/google/uuid"
)

type UserService interface {
	Register(ctx context.Context, req model.CreateUserRequest) (*model.User, error)
	GetProfile(ctx context.Context, id string) (*model.User, error)
	UpdateProfile(ctx context.Context, id string, req model.UpdateUserRequest) (*model.User, error)
}

type userService struct {
	repo repository.UserRepository
}

func NewUserService(repo repository.UserRepository) UserService {
	return &userService{repo: repo}
}

func (s *userService) Register(ctx context.Context, req model.CreateUserRequest) (*model.User, error) {
	// 1. Cek apakah email sudah terdaftar
	existing, _ := s.repo.GetByEmail(ctx, req.Email)
	if existing != nil {
		return nil, errors.New("email already registered")
	}

	// 2. Buat objek User baru (untuk sekarang password_hash langsung disalin dulu, hashing dibahas Episode 5)
	user := &model.User{
		ID:           uuid.New().String(),
		FullName:     req.FullName,
		Email:        req.Email,
		PasswordHash: req.Password, // TODO: Hash password di Episode 5
	}

	// 3. Simpan ke Database
	if err := s.repo.Create(ctx, user); err != nil {
		return nil, err
	}

	return s.repo.GetByID(ctx, user.ID)
}

func (s *userService) GetProfile(ctx context.Context, id string) (*model.User, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *userService) UpdateProfile(ctx context.Context, id string, req model.UpdateUserRequest) (*model.User, error) {
	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	user.FullName = req.FullName
	if err := s.repo.Update(ctx, user); err != nil {
		return nil, err
	}

	return s.repo.GetByID(ctx, id)
}
```

### Step 6: Membuat Handler Layer (`internal/user/handler/handler.go`)
Handler menerima REST HTTP request dan memvalidasi tipe data input menggunakan binding bawaan Gin.

```go
package handler

import (
	"net/http"

	"github.com/emzhofb/gowallet/monolith/internal/user/model"
	"github.com/emzhofb/gowallet/monolith/internal/user/service"
	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	svc service.UserService
}

func NewUserHandler(svc service.UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

func (h *UserHandler) Register(c *gin.Context) {
	var req model.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.svc.Register(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, user)
}

func (h *UserHandler) GetProfile(c *gin.Context) {
	id := c.Param("id")
	user, err := h.svc.GetProfile(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, user)
}

func (h *UserHandler) UpdateProfile(c *gin.Context) {
	id := c.Param("id")
	var req model.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.svc.UpdateProfile(c.Request.Context(), id, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, user)
}
```

### Step 7: Hubungkan Semua Layer di `cmd/main.go`
Kini kita lakukan *Dependency Injection* secara manual di `main.go`. Kita membuat router Gin dan mendaftarkan routing endpoint-nya.

```go
package main

import (
	"log"

	"github.com/emzhofb/gowallet/monolith/internal/config"
	"github.com/emzhofb/gowallet/monolith/internal/database"
	userHandler "github.com/emzhofb/gowallet/monolith/internal/user/handler"
	userRepository "github.com/emzhofb/gowallet/monolith/internal/user/repository"
	userService "github.com/emzhofb/gowallet/monolith/internal/user/service"
	"github.com/gin-gonic/gin"
)

func main() {
	log.Println("Starting Monolith Wallet Application...")

	cfg := config.LoadConfig()

	db, err := database.ConnectWithRetry(cfg.DBDSN)
	if err != nil {
		log.Fatalf("Could not connect to database: %v", err)
	}
	defer db.Close()

	// 1. Inisialisasi Layer
	uRepo := userRepository.NewMySQLUserRepository(db)
	uSvc := userService.NewUserService(uRepo)
	uHandler := userHandler.NewUserHandler(uSvc)

	// 2. Setup Gin Router
	r := gin.Default()

	// Routes
	r.POST("/api/v1/users", uHandler.Register)
	r.GET("/api/v1/users/:id", uHandler.GetProfile)
	r.PUT("/api/v1/users/:id", uHandler.UpdateProfile)

	// Start Server
	log.Println("Server running on port 8080...")
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("Server failed to run: %v", err)
	}
}
```

---

## ✅ Acceptance Criteria
* [ ] Menjalankan `go run cmd/main.go` berhasil mengaktifkan web server Gin di port `8080`.
* [ ] Mengirim `POST /api/v1/users` dengan valid JSON body berhasil membuat User baru di MySQL.
* [ ] Mengirim request dengan parameter email yang sudah terdaftar menghasilkan respons status `409 Conflict` dengan pesan error yang sesuai.
* [ ] Mengirim `GET /api/v1/users/:id` mengembalikan data user secara presisi tanpa menampilkan kolom password hash di JSON response.

---

## 💡 Tips untuk Junior
* **Interface as Contract:** Kita mendefinisikan interface (`UserRepository` dan `UserService`) sebagai kontrak. Ini sangat penting agar layer di atasnya tidak bergantung langsung pada detail implementasi (misal, jika suatu saat database MySQL diganti PostgreSQL, kita cukup membuat implementasi repo baru tanpa perlu mengubah kode di layer Service).
* **Context Propagation:** Selalu teruskan parameter `context.Context` (seperti `c.Request.Context()`) dari Handler ke Service, lalu ke Database ExecContext. Ini berguna jika client membatalkan request (*timeout/cancel*), query database akan otomatis dibatalkan sehingga menghemat resource server.

---

## 📚 Referensi Belajar
* [Clean Architecture by Uncle Bob](https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html)
* [Gin Web Framework Documentation](https://gin-gonic.com/docs/)
* [Go context package explanation](https://go.dev/blog/context)
