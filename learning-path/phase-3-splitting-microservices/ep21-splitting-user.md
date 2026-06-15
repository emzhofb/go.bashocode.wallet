# Episode 21: Memecah User Service

## 🎯 Tujuan
* Membuat folder microservice baru bernama `user-service` di dalam monorepo kita.
* Mendaftarkan service baru ke dalam workspace `go.work`.
* Memindahkan domain **User** (Registrasi, Profil pengguna, dan tabel data MySQL `users`) dari monolith lama agar berjalan sebagai service mandiri di port `:8084`.

---

## 📐 Konsep Pemecahan User Service
**User Service** bertanggung jawab sebagai sumber data tunggal (single source of truth) untuk profil pengguna:
1. **Tabel MySQL Dedikasi:** Service ini memegang kontrol eksklusif atas tabel `users` di database MySQL. Service lain yang butuh data user wajib menghubungi User Service (tidak boleh langsung melakukan query ke tabel `users` milik User Service).
2. **Endpoint gRPC:** Di samping melayani request HTTP dari client (melalui gateway), User Service juga menyediakan port gRPC server `:50052` agar Auth Service, Wallet Service, dan Transaction Service bisa meminta data profil secara internal dengan latensi rendah.

```
/Users/ikhda/Documents/coding/golang/wallet-microservice/
├── go.work
├── api-gateway/         (Port 8080)
├── auth-service/        (Port 8081)
└── user-service/        (Port 8084)  <-- Service Baru!
```

---

## 📦 Langkah-langkah

### Step 1: Membuat Folder user-service
Buat struktur folder untuk `user-service`:

```bash
mkdir -p user-service/cmd user-service/internal/{config,database,user}
```

Inisialisasi Go Module di dalam folder `user-service`:
```bash
cd user-service
go mod init github.com/emzhofb/gowallet/user-service
cd ..
```

Daftarkan service baru ini ke Go Workspace di file `go.work`:
```bash
go work use ./user-service
```

### Step 2: Salin Helper & Config
Salin file helper database (`database.go`), structured logging (`logger.go`), error handler (`errors.go`), config loader (`config.go`), serta middleware-middleware dari monolith ke folder `user-service/internal/` yang sesuai.

### Step 3: Pindahkan Domain User
Pindahkan kode dari subfolder `internal/user/` monolith (Model, Repository, Service, dan Handler) ke `user-service/internal/user/`.

Sesuaikan package import di seluruh file di dalam `user-service` agar menggunakan nama module yang baru:
```go
import "github.com/emzhofb/gowallet/user-service/internal/user/model"
```

### Step 4: Setup Entry Point `user-service/cmd/main.go`
Buat file `main.go` di `user-service/cmd/main.go` untuk menjalankan HTTP server User Service di port `:8084`.

```go
package main

import (
	"log"

	"github.com/emzhofb/gowallet/user-service/internal/config"
	"github.com/emzhofb/gowallet/user-service/internal/database"
	"github.com/emzhofb/gowallet/user-service/internal/logger"
	"github.com/emzhofb/gowallet/user-service/internal/middleware"
	userHandler "github.com/emzhofb/gowallet/user-service/internal/user/handler"
	userRepository "github.com/emzhofb/gowallet/user-service/internal/user/repository"
	userService "github.com/emzhofb/gowallet/user-service/internal/user/service"
	"github.com/gin-gonic/gin"
)

func main() {
	logger.InitLogger()
	logger.Log.Info("Starting User Microservice...")

	cfg := config.LoadConfig()

	db, err := database.ConnectWithRetry(cfg.DBDSN)
	if err != nil {
		log.Fatalf("Could not connect to database: %v", err)
	}
	defer db.Close()

	// Inisialisasi layer
	uRepo := userRepository.NewMySQLUserRepository(db)
	uSvc := userService.NewUserService(uRepo)
	uHandler := userHandler.NewUserHandler(uSvc)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.ErrorHandler())

	// Routes User HTTP
	r.POST("/api/v1/users/register", uHandler.Register)
	r.GET("/api/v1/users/:id", uHandler.GetProfile)

	logger.Log.Info("User Service listening on port 8084...")
	if err := r.Run(":8084"); err != nil {
		log.Fatalf("User Service failed: %v", err)
	}
}
```

---

## ✅ Acceptance Criteria
* [ ] Folder `user-service` terdaftar di workspace `go.work`.
* [ ] Menjalankan `go run user-service/cmd/main.go` mengaktifkan server di port `8084` dengan database MySQL tersambung.
* [ ] Request POST `/api/v1/users/register` ke port `8084` berhasil memproses registrasi user baru dan mencatatnya di tabel MySQL `users`.

---

## 💡 Tips untuk Junior
* **Satu Service, Satu Database Schema:** Jangan pernah membiarkan Auth Service memodifikasi atau menulis data langsung ke database MySQL milik User Service. Ini melanggar batas kontekstual (*bounded context*) dan merusak prinsip desentralisasi data.

---

## 📚 Referensi Belajar
* [Database per Service Pattern](https://microservices.io/patterns/data/database-per-service.html)
