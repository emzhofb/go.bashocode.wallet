# Episode 16: Memecah Auth & User Service

## 🎯 Tujuan
* Membuat folder microservice baru bernama `auth-service` di dalam monorepo kita.
* Mendaftarkan service baru ke dalam workspace `go.work`.
* Memindahkan domain `user` (Model, Repositori, Service, dan Handler) dari monolith lama agar berjalan sebagai service mandiri di port `:8081`.

---

## 📐 Konsep Pemecahan Kode (Decomposition)
Ketika memecah aplikasi monolith, langkah terbaik adalah memindahkan satu per satu domain fungsional yang memiliki dependensi paling sedikit. 

Domain **Auth & User** adalah pilihan terbaik sebagai langkah pertama karena:
* Menjadi fondasi identitas (User ID) bagi domain lain.
* Tidak bergantung pada data Wallet secara langsung (Wallet yang bergantung pada User).

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
mkdir -p auth-service/cmd auth-service/internal/{config,database,user}
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

### Step 2: Salin Modul Database & Config
Salin file konfigurasi database helper (`database.go`), structured logging (`logger.go`), error handler (`errors.go`), config loader (`config.go`), serta middleware-middleware dari monolith lama ke folder `auth-service/internal/` yang sesuai.

### Step 3: Pindahkan Domain User
Pindahkan folder domain `user` yang berisi `model`, `repository`, `service`, dan `handler` ke `auth-service/internal/user/`.

Perbarui seluruh package import di dalam file-file tersebut agar menggunakan nama module yang baru:
```go
// Ganti import lama
import "github.com/emzhofb/gowallet/monolith/internal/user/model"

// Menjadi import baru
import "github.com/emzhofb/gowallet/auth-service/internal/user/model"
```

### Step 4: Setup Entry Point `auth-service/cmd/main.go`
Buat file `main.go` di `auth-service/cmd/main.go` untuk menjalankan HTTP server khusus domain Auth & User di port `:8081`.

```go
package main

import (
	"log"

	"github.com/emzhofb/gowallet/auth-service/internal/config"
	"github.com/emzhofb/gowallet/auth-service/internal/database"
	"github.com/emzhofb/gowallet/auth-service/internal/logger"
	"github.com/emzhofb/gowallet/auth-service/internal/middleware"
	userHandler "github.com/emzhofb/gowallet/auth-service/internal/user/handler"
	userRepository "github.com/emzhofb/gowallet/auth-service/internal/user/repository"
	userService "github.com/emzhofb/gowallet/auth-service/internal/user/service"
	"github.com/gin-gonic/gin"
)

func main() {
	logger.InitLogger()
	logger.Log.Info("Starting Auth & User Microservice...")

	cfg := config.LoadConfig()

	db, err := database.ConnectWithRetry(cfg.DBDSN)
	if err != nil {
		log.Fatalf("Could not connect to database: %v", err)
	}
	defer db.Close()

	// Inisialisasi layer
	uRepo := userRepository.NewMySQLUserRepository(db)
	uSvc := userService.NewUserService(uRepo) // Wallet Repository dihilangkan dulu (Ep 17 diganti gRPC)
	uHandler := userHandler.NewUserHandler(uSvc)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.ErrorHandler())

	// Routes
	r.POST("/api/v1/auth/register", uHandler.Register)
	r.POST("/api/v1/auth/login", uHandler.Login)
	r.POST("/api/v1/auth/logout", uHandler.Logout)
	r.GET("/api/v1/users/:id", uHandler.GetProfile)

	logger.Log.Info("Auth Service listening on port 8081...")
	if err := r.Run(":8081"); err != nil {
		log.Fatalf("Auth Service failed: %v", err)
	}
}
```

---

## ✅ Acceptance Criteria
* [ ] Folder `auth-service` terdaftar di workspace `go.work`.
* [ ] Menjalankan `go run auth-service/cmd/main.go` mengaktifkan server di port `8081` dengan database MySQL tersambung.
* [ ] Memanggil API gateway `/api/v1/auth/login` di port `8080` berhasil diteruskan ke auth-service di port `8081` dan mengembalikan JWT token.

---

## 💡 Tips untuk Junior
* **Gunakan Environment Port:** Alih-alih menulis port `:8081` secara keras (*hardcoded*) di dalam kode, gunakan file `.env` (misal: `PORT=8081`) agar port setiap service bisa diubah dengan mudah jika ada tabrakan (*port collision*) di server deployment.

---

## 📚 Referensi Belajar
* [Microservices decomposition patterns](https://microservices.io/patterns/decomposition/decompose-by-subdomain.html)
