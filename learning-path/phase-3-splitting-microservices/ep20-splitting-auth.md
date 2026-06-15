# Episode 20: Memecah Auth Service

## 🎯 Tujuan
* Membuat folder microservice baru bernama `auth-service` di dalam monorepo kita.
* Mendaftarkan service baru ke dalam workspace `go.work`.
* Mengisolasi domain **Authentication** (login, token generation, refresh token rotation, dan blacklisting) agar berjalan sebagai service mandiri di port `:8081`.

---

## 📐 Konsep Pemecahan Auth Service
Dalam arsitektur microservices modern, **Auth Service** bertanggung jawab penuh atas manajemen sesi dan keamanan token:
1. **Pemisahan Data:** Auth Service hanya mengurusi data token sesi dan blacklist (Redis), sementara profil data pengguna dikelola oleh **User Service**.
2. **Komunikasi:** Auth Service akan memvalidasi kredensial login dengan meminta data hash password dari User Service melalui gRPC (akan dibahas di Episode 22).
3. **Efisiensi:** Dengan memisahkan Auth Service, jika traffic login meningkat tajam (misal pada jam masuk kerja), kita dapat men-scale up kontainer Auth Service saja tanpa harus menduplikasi resource User Service.

```
/Users/ikhda/Documents/coding/golang/wallet-microservice/
├── go.work
├── api-gateway/         (Port 8080)
└── auth-service/        (Port 8081)  <-- Service Baru!
```

---

## 📦 Langkah-langkah

### Step 1: Membuat Folder auth-service
Buat struktur folder untuk `auth-service`:

```bash
mkdir -p auth-service/cmd auth-service/internal/{config,database,auth,middleware}
```

Inisialisasi Go Module di dalam folder `auth-service`:
```bash
cd auth-service
go mod init github.com/emzhofb/gowallet/auth-service
cd ..
```

Daftarkan service baru ini ke Go Workspace di file `go.work`:
```bash
go work use ./auth-service
```

### Step 2: Salin Helper & Config
Salin helper redis (`redis.go`), structured logging (`logger.go`), error handler (`errors.go`), config loader (`config.go`), serta middleware-middleware dari monolith ke folder `auth-service/internal/` yang sesuai.

### Step 3: Pindahkan Domain Auth
Pindahkan logika token generation, validation, dan blacklisting (dari file `internal/auth/jwt.go` monolith) ke `auth-service/internal/auth/`.

Sesuaikan import package agar mengarah ke module path `auth-service`:
```go
import "github.com/emzhofb/gowallet/auth-service/internal/logger"
```

### Step 4: Setup Entry Point `auth-service/cmd/main.go`
Buat file `main.go` di `auth-service/cmd/main.go` untuk menjalankan HTTP server Auth Service di port `:8081`.

```go
package main

import (
	"log"

	"github.com/emzhofb/gowallet/auth-service/internal/auth/handler"
	"github.com/emzhofb/gowallet/auth-service/internal/auth/service"
	"github.com/emzhofb/gowallet/auth-service/internal/config"
	"github.com/emzhofb/gowallet/auth-service/internal/database"
	"github.com/emzhofb/gowallet/auth-service/internal/logger"
	"github.com/emzhofb/gowallet/auth-service/internal/middleware"
	"github.com/gin-gonic/gin"
)

func main() {
	logger.InitLogger()
	logger.Log.Info("Starting Auth Microservice...")

	cfg := config.LoadConfig()

	rdb, err := database.ConnectRedis(cfg.RedisAddr)
	if err != nil {
		log.Fatalf("Could not connect to Redis: %v", err)
	}
	defer rdb.Close()

	// Inisialisasi layer
	authSvc := service.NewAuthService(rdb)
	authHandler := handler.NewAuthHandler(authSvc)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.ErrorHandler())

	// Routes Auth
	r.POST("/api/v1/auth/login", authHandler.Login)
	r.POST("/api/v1/auth/refresh", authHandler.Refresh)
	r.POST("/api/v1/auth/logout", authHandler.Logout)

	logger.Log.Info("Auth Service listening on port 8081...")
	if err := r.Run(":8081"); err != nil {
		log.Fatalf("Auth Service failed: %v", err)
	}
}
```

---

## ✅ Acceptance Criteria
* [ ] Folder `auth-service` terdaftar di workspace `go.work`.
* [ ] Menjalankan `go run auth-service/cmd/main.go` mengaktifkan server di port `8081` dengan koneksi Redis aktif.
* [ ] Endpoint `/api/v1/auth/login` berhasil dideklarasikan tanpa runtime error dependency.

---

## 💡 Tips untuk Junior
* **Session Cache di Redis:** Menyimpan session status atau token blacklist di Redis jauh lebih cepat daripada menggunakan MySQL karena operasi memori Redis berjalan dengan latensi di bawah 1 milidetik.

---

## 📚 Referensi Belajar
* [Microservices Security Patterns](https://microservices.io/patterns/security/api-gateway-security.html)
