# Episode 4: Structured Logging & Centralized Error Handling

## 🎯 Tujuan
* Mengganti logging bawaan Go (`log`) dengan **Structured Logging** menggunakan package standard `log/slog` (format JSON).
* Membuat sistem penanganan error yang terpusat (*Centralized Error Handling*).
* Menyeragamkan format response error yang dikirim ke client API:
  ```json
  {
    "success": false,
    "error": {
      "code": "EMAIL_ALREADY_REGISTERED",
      "message": "Email ini sudah digunakan oleh akun lain."
    }
  }
  ```

---

## 📦 Langkah-langkah

### Step 1: Membuat Helper Custom Error (`internal/errors/errors.go`)
Kita akan membuat struktur data error khusus yang memuat HTTP Status Code, kode error internal (string), dan pesan ramah pengguna.

Buat file baru di `internal/errors/errors.go`:

```go
package errors

import "net/http"

type AppError struct {
	StatusCode int    `json:"-"` // Tidak diekspos ke JSON client
	Code       string `json:"code"`
	Message    string `json:"message"`
}

func (e *AppError) Error() string {
	return e.Message
}

func NewAppError(status int, code string, msg string) *AppError {
	return &AppError{
		StatusCode: status,
		Code:       code,
		Message:    msg,
	}
}

// Definisikan beberapa error standard yang sering dipakai
var (
	ErrInternalServer = NewAppError(http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Terjadi kesalahan internal pada server.")
	ErrBadRequest     = NewAppError(http.StatusBadRequest, "BAD_REQUEST", "Format input tidak valid.")
	ErrNotFound       = NewAppError(http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
	ErrUnauthorized   = NewAppError(http.StatusUnauthorized, "UNAUTHORIZED", "Anda tidak memiliki akses.")
)
```

### Step 2: Konfigurasi Structured Logging (`internal/logger/logger.go`)
Structured logging dengan format JSON sangat membantu tim SysOps/DevOps saat menganalisis log menggunakan log aggregator (seperti Kibana di Fase 5). Kita gunakan standard library `log/slog`. Kita juga mendefinisikan kunci konteks untuk Request Correlation ID agar setiap log dapat dilacak per request.

Buat file baru di `internal/logger/logger.go`:

```go
package logger

import (
	"context"
	"log/slog"
	"os"
)

var Log *slog.Logger

const CorrelationIDKey = "correlation_id"

func InitLogger() {
	// Set default structured JSON handler ke stdout
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo, // Tampilkan log level INFO ke atas
	})
	Log = slog.New(handler)
	slog.SetDefault(Log)
}

// Helper untuk log dengan context yang otomatis menyertakan correlation_id jika ada
func getLogArgs(ctx context.Context, args []any) []any {
	if ctx != nil {
		if cid, ok := ctx.Value(CorrelationIDKey).(string); ok {
			return append(args, slog.String("correlation_id", cid))
		}
	}
	return args
}

func Info(ctx context.Context, msg string, args ...any) {
	Log.InfoContext(ctx, msg, getLogArgs(ctx, args)...)
}

func Error(ctx context.Context, msg string, args ...any) {
	Log.ErrorContext(ctx, msg, getLogArgs(ctx, args)...)
}

func Warn(ctx context.Context, msg string, args ...any) {
	Log.WarnContext(ctx, msg, getLogArgs(ctx, args)...)
}
```

### Step 2.5: Membuat Correlation ID Middleware (`internal/middleware/correlation.go`)
Middleware ini akan mengambil header `X-Correlation-ID` dari request. Jika tidak dikirim oleh client/gateway, middleware akan menghasilkan UUID baru. ID ini kemudian ditanam di request context dan dikirim balik lewat HTTP response header.

Unduh library UUID terlebih dahulu:
```bash
go get github.com/google/uuid
```

Buat file baru di `internal/middleware/correlation.go`:

```go
package middleware

import (
	"context"

	"github.com/emzhofb/gowallet/monolith/internal/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func CorrelationID() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Baca dari header request
		corID := c.GetHeader("X-Correlation-ID")
		if corID == "" {
			// Generate UUID baru jika kosong
			corID = uuid.New().String()
		}

		// Tanam di request context
		ctx := context.WithValue(c.Request.Context(), logger.CorrelationIDKey, corID)
		c.Request = c.Request.WithContext(ctx)

		// Sertakan di header HTTP Response
		c.Header("X-Correlation-ID", corID)

		c.Next()
	}
}
```

### Step 3: Membuat Error Handling Middleware (`internal/middleware/error_handler.go`)
Middleware ini akan menangkap panic (recovery) dan memastikan semua error di-format secara seragam sebelum dikirim ke client.

Buat file baru di `internal/middleware/error_handler.go`:

```go
package middleware

import (
	"net/http"

	customErr "github.com/emzhofb/gowallet/monolith/internal/errors"
	"github.com/emzhofb/gowallet/monolith/internal/logger"
	"github.com/gin-gonic/gin"
)

func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Jalankan handler selanjutnya
		c.Next()

		// Cek apakah ada error yang dilaporkan selama request
		if len(c.Errors) > 0 {
			err := c.Errors.Last().Err
			
			// Cek apakah tipe error adalah custom AppError kita
			if appErr, ok := err.(*customErr.AppError); ok {
				logger.Warn(c.Request.Context(), "Client error occurred", 
					"code", appErr.Code, 
					"message", appErr.Message, 
					"status", appErr.StatusCode,
				)
				c.JSON(appErr.StatusCode, gin.H{
					"success": false,
					"error":   appErr,
				})
				return
			}

			// Jika error umum/tidak terduga
			logger.Error(c.Request.Context(), "Unhandled error occurred", "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   customErr.ErrInternalServer,
			})
		}
	}
}
```

### Step 4: Menyesuaikan Service & Handler User
Mari sesuaikan service user agar me-return `AppError`:

Edit `internal/user/service/service.go`:
```go
// ... import dan method lainnya ...

func (s *userService) Register(ctx context.Context, req model.CreateUserRequest) (*model.User, error) {
	existing, _ := s.repo.GetByEmail(ctx, req.Email)
	if existing != nil {
		// Mengembalikan custom AppError
		return nil, customErr.NewAppError(http.StatusConflict, "EMAIL_ALREADY_REGISTERED", "Email ini sudah terdaftar.")
	}

	user := &model.User{
		ID:           uuid.New().String(),
		FullName:     req.FullName,
		Email:        req.Email,
		PasswordHash: req.Password,
	}

	if err := s.repo.Create(ctx, user); err != nil {
		// Kembalikan Internal Server Error
		return nil, customErr.ErrInternalServer
	}

	return s.repo.GetByID(ctx, user.ID)
}

func (s *userService) GetProfile(ctx context.Context, id string) (*model.User, error) {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, customErr.NewAppError(http.StatusNotFound, "USER_NOT_FOUND", "Profil pengguna tidak ditemukan.")
	}
	return u, nil
}
```

Edit `internal/user/handler/handler.go` agar menggunakan `c.Error(err)` alih-alih me-render JSON manual:

```go
package handler

import (
	"net/http"

	customErr "github.com/emzhofb/gowallet/monolith/internal/errors"
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
		// Daftarkan error input ke Gin Context
		c.Error(customErr.NewAppError(http.StatusBadRequest, "INVALID_INPUT", err.Error()))
		return
	}

	user, err := h.svc.Register(c.Request.Context(), req)
	if err != nil {
		c.Error(err) // Mendaftarkan error ke middleware
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    user,
	})
}

func (h *UserHandler) GetProfile(c *gin.Context) {
	id := c.Param("id")
	user, err := h.svc.GetProfile(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    user,
	})
}
```

### Step 5: Update `cmd/main.go`
Inisialisasi Logger di awal aplikasi, dan daftarkan middleware `CorrelationID` serta `ErrorHandler` ke router Gin:

```go
package main

import (
	"log"

	"github.com/emzhofb/gowallet/monolith/internal/config"
	"github.com/emzhofb/gowallet/monolith/internal/database"
	"github.com/emzhofb/gowallet/monolith/internal/logger"
	"github.com/emzhofb/gowallet/monolith/internal/middleware"
	userHandler "github.com/emzhofb/gowallet/monolith/internal/user/handler"
	userRepository "github.com/emzhofb/gowallet/monolith/internal/user/repository"
	userService "github.com/emzhofb/gowallet/monolith/internal/user/service"
	"github.com/gin-gonic/gin"
)

func main() {
	// 1. Inisialisasi Logger Structured slog
	logger.InitLogger()
	logger.Log.Info("Starting Monolith Wallet Application...")

	cfg := config.LoadConfig()

	db, err := database.ConnectWithRetry(cfg.DBDSN)
	if err != nil {
		log.Fatalf("Could not connect to database: %v", err)
	}
	defer db.Close()

	uRepo := userRepository.NewMySQLUserRepository(db)
	uSvc := userService.NewUserService(uRepo)
	uHandler := userHandler.NewUserHandler(uSvc)

	// 2. Setup Gin Router
	r := gin.New() // Gunakan gin.New() agar log default console gin tidak bertabrakan dengan slog
	r.Use(gin.Recovery()) // Tangkap panic secara otomatis
	r.Use(middleware.CorrelationID()) // Daftarkan Correlation ID middleware
	r.Use(middleware.ErrorHandler()) // Daftarkan central error handler kita

	// Routes
	r.POST("/api/v1/users", uHandler.Register)
	r.GET("/api/v1/users/:id", uHandler.GetProfile)

	logger.Log.Info("Server running on port 8080...")
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
```

---

## ✅ Acceptance Criteria
* [ ] Log aplikasi di konsol tampil dalam format JSON terstruktur (mengandung parameter `"time"`, `"level"`, `"msg"`, dan `"correlation_id"`).
* [ ] Setiap request HTTP response mengembalikan header `X-Correlation-ID` yang bernilai UUID unik.
* [ ] Terjadi panic di dalam program tetap menghasilkan respons API yang aman (HTTP 500 JSON, bukan blank white screen).
* [ ] Error response memiliki format standard yang konsisten (`{"success": false, "error": { "code": "...", "message": "..." } }`).

---

## 💡 Tips untuk Junior
* **Jangan Ekspos Detail Stack Trace database/internal:** Jangan pernah mengirim pesan error mentah dari database seperti `sql: no rows in result set` atau `connection refused` ke client API. Hal ini mempermudah hacker mengidentifikasi celah keamanan (SQL Injection, dll). Kembalikan pesan yang sopan kepada user seperti "Data tidak ditemukan." dan simpan detail error aslinya di server log (`logger.Error`).

---

## 📚 Referensi Belajar
* [Guide to slog package in Go 1.21+](https://go.dev/blog/slog)
* [API Error Handling Best Practices](https://github.com/microsoft/api-guidelines/blob/vNext/Guidelines.md#7102-error-response-body)
