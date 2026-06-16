# Episode 19: Monorepo & API Gateway (Reverse Proxy)

## 🎯 Tujuan
* Mengubah project directory kita menjadi **Monorepo** menggunakan **Go Workspaces** (`go.work`).
* Membuat service baru bernama `api-gateway` sebagai pintu masuk tunggal (*single entry point*) seluruh request client.
* Mengimplementasikan **Reverse Proxy** menggunakan standard library Go untuk meneruskan traffic dari Gateway ke microservice tujuan.

---

## 📐 Mengapa Butuh API Gateway?
Saat kita memecah monolith menjadi microservices, setiap service akan berjalan di port yang berbeda-beda (misalnya Auth di `:8081`, Wallet di `:8082`, dst). 
Sangat buruk jika aplikasi mobile atau frontend client harus menghafal semua port tersebut. 
* **Solusinya:** Client hanya menembak satu alamat Gateway di port `:8080`. Gateway kemudian bertugas mengarahkan (*routing*) request tersebut:
  * `/api/v1/users/*` ➔ diteruskan ke Auth/User Service.
  * `/api/v1/wallets/*` ➔ diteruskan ke Wallet Service.

---

## 📦 Langkah-langkah

### Step 1: Migrasi ke Go Workspace (Monorepo)
Kita akan memindahkan folder `monolith` saat ini menjadi folder `api-gateway` dan memisahkannya secara workspace.
Di folder root project (`/Users/ikhda/Documents/coding/golang/wallet-microservice/`), hapus file workspace lama jika ada, lalu jalankan inisialisasi:

```bash
# Inisialisasi Go Workspace di folder ROOT repository
go work init

# Buat folder baru untuk api-gateway dan copy kode monolith lama ke sub-folder baru
# (Di terminal Anda, lakukan ini)
```

Untuk struktur folder di workspace root, kita inginkan:
```
/Users/ikhda/Documents/coding/golang/wallet-microservice/
├── go.work
├── api-gateway/
│   ├── cmd/
│   ├── internal/
│   ├── go.mod
│   └── .env
└── auth-service/   (Akan dibuat di Episode 16)
```

Daftarkan folder ke workspace:
```bash
go work use ./api-gateway
```

### Step 2: Membuat Reverse Proxy Helper (`internal/proxy/proxy.go`)
Di dalam `api-gateway/internal/`, buat folder `proxy` dan file `proxy.go`. Kita gunakan `httputil.ReverseProxy` bawaan Go yang sangat efisien dan otomatis menangani streaming request/response headers.

```go
package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/google/uuid"
)

type ReverseProxy struct {
	target *url.URL
	proxy  *httputil.ReverseProxy
}

func NewReverseProxy(targetURL string) (*ReverseProxy, error) {
	url, err := url.Parse(targetURL)
	if err != nil {
		return nil, err
	}

	// Buat reverse proxy bawaan Go
	proxy := httputil.NewSingleHostReverseProxy(url)

	// Modifikasi director agar request diteruskan dengan path yang benar
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Header.Set("X-Forwarded-Host", req.Header.Get("Host"))
		req.Host = url.Host

		// Inject & forward Request Correlation ID untuk distributed logging
		corID := req.Header.Get("X-Correlation-ID")
		if corID == "" {
			corID = uuid.New().String()
		}
		req.Header.Set("X-Correlation-ID", corID)
	}

	return &ReverseProxy{
		target: url,
		proxy:  proxy,
	}, nil
}

func (p *ReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.proxy.ServeHTTP(w, r)
}
```

### Step 3: Membuat CORS Middleware (`internal/middleware/cors.go`)
Untuk mencegah isu pemblokiran browser saat frontend client (web/mobile) memanggil API Gateway kita lintas domain, kita perlu mengaktifkan **CORS (Cross-Origin Resource Sharing)**.

Buat file baru di `api-gateway/internal/middleware/cors.go`:

```go
package middleware

import "github.com/gin-gonic/gin"

func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-Correlation-ID")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		// Tangani preflight OPTIONS request
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
```

### Step 4: Implementasi Routing di Gateway `cmd/main.go`
Buka file `cmd/main.go` di `api-gateway/`. Gateway tidak lagi melakukan query database atau validasi registrasi user secara langsung. Gateway hanya bertugas mem-proxy traffic ke microservices yang sesuai dan menangani *cross-cutting concerns* seperti CORS.

```go
package main

import (
	"log"
	"net/http"

	"github.com/emzhofb/gowallet/api-gateway/internal/config"
	"github.com/emzhofb/gowallet/api-gateway/internal/middleware"
	"github.com/emzhofb/gowallet/api-gateway/internal/proxy"
	"github.com/gin-gonic/gin"
)

func main() {
	log.Println("Starting API Gateway on port 8080...")

	// 1. Buat router proxy untuk masing-masing target microservices
	userServiceUrl := "http://localhost:8081" // Auth & User Service
	walletServiceUrl := "http://localhost:8082" // Wallet Service
	transactionServiceUrl := "http://localhost:8086" // Transaction Service
	paymentServiceUrl := "http://localhost:8083" // Payment Service

	userProxy, err := proxy.NewReverseProxy(userServiceUrl)
	if err != nil {
		log.Fatalf("Failed to initialize user proxy: %v", err)
	}

	walletProxy, err := proxy.NewReverseProxy(walletServiceUrl)
	if err != nil {
		log.Fatalf("Failed to initialize wallet proxy: %v", err)
	}

	transactionProxy, err := proxy.NewReverseProxy(transactionServiceUrl)
	if err != nil {
		log.Fatalf("Failed to initialize transaction proxy: %v", err)
	}

	paymentProxy, err := proxy.NewReverseProxy(paymentServiceUrl)
	if err != nil {
		log.Fatalf("Failed to initialize payment proxy: %v", err)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	
	// Aktifkan CORS Middleware
	r.Use(middleware.CORSMiddleware())

	// 2. Tentukan aturan routing proxy
	// Semua request ke /api/v1/users/* akan diteruskan ke User Service di port 8081 (monolith) / 8084 (microservices split)
	r.Any("/api/v1/users/*path", func(c *gin.Context) {
		userProxy.ServeHTTP(c.Writer, c.Request)
	})

	r.Any("/api/v1/auth/*path", func(c *gin.Context) {
		userProxy.ServeHTTP(c.Writer, c.Request)
	})

	// Semua request ke /api/v1/wallets/* akan diteruskan ke Wallet Service di port 8082
	r.Any("/api/v1/wallets/*path", func(c *gin.Context) {
		walletProxy.ServeHTTP(c.Writer, c.Request)
	})

	// Semua request ke /api/v1/transactions/* akan diteruskan ke Transaction Service di port 8086
	r.Any("/api/v1/transactions/*path", func(c *gin.Context) {
		transactionProxy.ServeHTTP(c.Writer, c.Request)
	})

	// Semua request ke /api/v1/payments/* akan diteruskan ke Payment Service di port 8083
	r.Any("/api/v1/payments/*path", func(c *gin.Context) {
		paymentProxy.ServeHTTP(c.Writer, c.Request)
	})

	log.Println("API Gateway listening on port 8080...")
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("Gateway failed: %v", err)
	}
}
```

---

## ✅ Acceptance Criteria
* [ ] File `go.work` sukses terbuat di root directory dan mendaftarkan folder `api-gateway`.
* [ ] Gateway berjalan di port `8080` secara independen.
* [ ] Memanggil endpoint `GET http://localhost:8080/api/v1/users/me` diteruskan dengan benar ke target service dibelakangnya.
* [ ] Mengirimkan HTTP `OPTIONS` request ke API Gateway berhasil mengembalikan status HTTP `204 No Content` beserta header CORS yang sesuai.

---

## 💡 Tips untuk Junior
* **X-Forwarded Headers:** Selalu sertakan header `X-Forwarded-For` dan `X-Forwarded-Host` saat melakukan reverse proxy. Ini membantu service di belakang gateway mengetahui IP asli pengguna akhir (*real client IP*), bukan IP server gateway.
* **CORS Preflight (OPTIONS):** Sebelum browser mengirimkan request asli (seperti `POST` atau `PUT` dengan header kustom), browser akan mengirimkan request `OPTIONS` terlebih dahulu (*preflight*). Jika server kita tidak membalas request preflight ini dengan response status sukses (biasanya `200` atau `204`) beserta header CORS, browser akan memblokir kelanjutan request tersebut.

---

## 📚 Referensi Belajar
* [Go Workspaces (Official Documentation)](https://go.dev/doc/tutorial/workspaces)
* [Reverse Proxy Pattern Guide](https://www.nginx.com/resources/glossary/reverse-proxy-server/)
* [Mozilla MDN Web Docs - CORS](https://developer.mozilla.org/en-US/docs/Web/HTTP/CORS)
