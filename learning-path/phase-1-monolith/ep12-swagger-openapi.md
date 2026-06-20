# Episode12: API Documentation dengan Swagger/OpenAPI

## 🎯 Tujuan
* Mengenalkan pentingnya **API Documentation** standar industri untuk memudahkan integrasi dengan frontend developer / client.
* Menginstal dan mengonfigurasi **swaggo/swag** untuk men-generate spesifikasi OpenAPI dari komentar kode Go.
* Menyediakan endpoint `/swagger/*any` untuk merender interaktif **Swagger UI**.

---

## 📐 Konsep OpenAPI & Swagger
OpenAPI Specification (OAS) adalah standar format deskripsi API untuk RESTful API.
* Daripada menulis dokumentasi manual di Google Doc/Notion yang gampang *out-of-date*, kita menulis dokumentasi langsung di dekat kode Go kita berupa **Komentar Kode (Annotations)**.
* Library `swaggo/swag` akan membaca komentar tersebut dan menerjemahkannya secara otomatis menjadi berkas `swagger.json` / `swagger.yaml`.
* Keuntungan: Jika ada perubahan parameter di kode, developer cukup mengupdate komentar kodenya lalu me-run generator, dokumentasi otomatis terupdate secara sinkron.

---

## 📦 Langkah-langkah

### Step 1: Install Swag CLI & Middleware
Unduh command-line tool `swag` untuk men-generate dokumen:
```bash
go install github.com/swaggo/swag/cmd/swag@latest
```
Unduh library middleware adapter untuk Gin web framework:
```bash
go get github.com/swaggo/gin-swagger
go get github.com/swaggo/files
```

### Step 2: Menulis Deskripsi General API di `cmd/main.go`
Tambahkan deklarasi metadata API umum di atas fungsi `main()` di file `cmd/main.go`:

```go
// @title           GoWallet Monolith API
// @version         1.0
// @description     API dokumentasi untuk layanan GoWallet Monolith Backend.
// @termsOfService  http://swagger.io/terms/

// @contact.name   API Support
// @contact.email  support@gowallet.com

// @host      localhost:8080
// @BasePath  /api/v1
```

### Step 3: Menambahkan Annotations di Model DTO
Buka file `internal/user/model/user.go`, tambahkan anotasi `example` dan deskripsi opsional pada struct tag JSON:

```go
type CreateUserRequest struct {
	FullName string `json:"full_name" binding:"required" example:"Budi Santoso"`
	Email    string `json:"email" binding:"required,email" example:"budi@example.com"`
	Password string `json:"password" binding:"required,min=6" example:"password123"`
}
```

### Step 4: Menambahkan Annotations di HTTP Handlers
Buka file `internal/user/handler/handler.go`. Tambahkan komentar anotasi di atas fungsi `Register` dan `GetProfile`:

```go
// Register godoc
// @Summary      Register User Baru
// @Description  Membuat user baru beserta wallet default-nya.
// @Tags         users
// @Accept       json
// @Produce      json
// @Param        request body model.CreateUserRequest true "Payload pendaftaran user"
// @Success      201  {object}  model.User
// @Failure      400  {object}  errors.AppError
// @Failure      409  {object}  errors.AppError
// @Router       /users/register [post]
func (h *UserHandler) Register(c *gin.Context) {
    // ...
}

// GetProfile godoc
// @Summary      Mendapatkan Profil User
// @Description  Mengambil profil user berdasarkan user ID.
// @Tags         users
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "User ID (UUID)"
// @Success      200  {object}  model.User
// @Failure      404  {object}  errors.AppError
// @Router       /users/{id} [get]
func (h *UserHandler) GetProfile(c *gin.Context) {
    // ...
}
```

### Step 5: Men-generate Dokumentasi
Jalankan perintah ini di terminal root directory `monolith/`:
```bash
swag init -g cmd/main.go
```
Perintah ini akan membaca anotasi kode Anda dan menghasilkan folder baru bernama `docs/` yang berisi `docs.go`, `swagger.json`, dan `swagger.yaml`.

### Step 6: Serve Swagger UI di `cmd/main.go`
Buka file `cmd/main.go`, daftarkan routing Swagger UI agar bisa diakses lewat browser:

```go
import (
	_ "github.com/emzhofb/gowallet/monolith/docs" // Import docs folder secara default (blank import)
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func main() {
    // ... setup gin ...
    
    // Register Swagger UI route
    r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
    
    // ... start server ...
}
```

---

## ✅ Acceptance Criteria
* [ ] Folder `docs/` sukses ter-generate berisi file spesifikasi swagger JSON.
* [ ] Menjalankan server `monolith` dan membuka URL browser `http://localhost:8080/swagger/index.html` berhasil merender dashboard interaktif Swagger UI.
* [ ] Dashboard menampilkan tag `users` beserta visualisasi skema request body DTO dan tombol "Try it out" untuk melakukan tes request HTTP secara langsung.

---

## 💡 Tips untuk Junior
* **Selalu Run Swag Init Setelah Edit Handler:** Jika Anda mengubah route path, query parameter, atau model request di kode, dokumentasi Swagger **tidak akan otomatis ter-update** kecuali Anda kembali mengetikkan command `swag init` di terminal sebelum menjalankan server.

---

## 📚 Referensi Belajar
* [swaggo/swag GitHub documentation](https://github.com/swaggo/swag)
* [Declarative Comments Format for Swag](https://github.com/swaggo/swag#declarative-comments-format)
