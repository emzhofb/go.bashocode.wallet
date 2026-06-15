# Episode 2: API Gateway

## 🎯 Tujuan
- Membuat API Gateway sebagai single entry point untuk semua service
- Implement reverse proxy ke backend services
- Implement middleware stack: Recovery, Correlation ID, Logging, CORS, Rate Limiter, JWT Auth
- Setup health checks (`/health`, `/ready`, `/live`)
- Implement graceful shutdown dan timeout configuration

## 📝 Prerequisites
- Episode 1 selesai (Docker Compose running, shared packages ready)

---

## 📦 Langkah-langkah

### Step 1: Inisialisasi Service

```bash
# Dari root project
mkdir -p api-gateway/cmd
mkdir -p api-gateway/internal/{middleware,proxy,config}

cd api-gateway
go mod init github.com/emzhofb/gowallet/api-gateway
cd ..

# Tambahkan ke Go workspace
go work use ./api-gateway

# Install dependencies
cd api-gateway
go get github.com/gin-gonic/gin
go get github.com/google/uuid
go get github.com/redis/go-redis/v9
go get go.uber.org/zap
cd ..
```

### Step 2: Config (`api-gateway/internal/config/config.go`)

Buat config loader untuk API Gateway:

```go
type Config struct {
    Port            string
    Env             string
    
    // Service URLs (untuk reverse proxy)
    AuthServiceURL        string  // http://localhost:8081
    UserServiceURL        string  // http://localhost:8082
    WalletServiceURL      string  // http://localhost:8083
    TransactionServiceURL string  // http://localhost:8085
    PaymentServiceURL     string  // http://localhost:8086
    AuditServiceURL       string  // http://localhost:8088
    
    // Redis (untuk rate limiter)
    RedisHost     string
    RedisPort     string
    RedisPassword string
    
    // JWT
    JWTSecret string
    
    // Rate Limiting
    RateLimitAnonymous     int
    RateLimitAuthenticated int
    RateLimitAdmin         int
    RateLimitWindow        time.Duration
}
```

### Step 3: Middleware Stack

Middleware dieksekusi berurutan. **Urutan penting!**

```
Request masuk
    │
    ▼
[1. Recovery]         ← Catch panic, return 500
    │
    ▼
[2. Correlation ID]   ← Generate UUID, set di header & context
    │
    ▼
[3. Structured Log]   ← Log request masuk (method, path, IP)
    │
    ▼
[4. CORS]             ← Set CORS headers
    │
    ▼
[5. Rate Limiter]     ← Cek limit (Redis), return 429 jika melebihi
    │
    ▼
[6. JWT Auth]         ← Validasi token (skip untuk public routes)
    │
    ▼
[Reverse Proxy]       ← Forward ke backend service
    │
    ▼
[3. Structured Log]   ← Log response keluar (status, duration)
```

#### 3a. Recovery Middleware

```go
// Catch panic agar server tidak crash
// Return HTTP 500 Internal Server Error
// Log stack trace
func Recovery() gin.HandlerFunc {
    // Gunakan gin.Recovery() bawaan atau buat custom
}
```

#### 3b. Correlation ID Middleware

```go
// 1. Cek header X-Correlation-ID dari request
// 2. Jika tidak ada, generate UUID baru
// 3. Set di request context
// 4. Set di response header X-Correlation-ID
// 5. Semua log di request ini harus include correlation_id

func CorrelationID() gin.HandlerFunc {
    return func(c *gin.Context) {
        correlationID := c.GetHeader("X-Correlation-ID")
        if correlationID == "" {
            correlationID = uuid.New().String()
        }
        
        c.Set("correlation_id", correlationID)
        c.Header("X-Correlation-ID", correlationID)
        
        c.Next()
    }
}
```

#### 3c. Logging Middleware

```go
// Log setiap request dan response
// Format: JSON structured log menggunakan Zap
// Fields: method, path, status, duration, ip, correlation_id, user_agent

// Contoh output:
// {"level":"info","ts":"...","msg":"request","method":"POST","path":"/api/v1/auth/login",
//  "status":200,"duration":"45ms","ip":"192.168.1.1","correlation_id":"abc-123"}
```

#### 3d. CORS Middleware

```go
// Headers yang harus di-set:
// Access-Control-Allow-Origin: * (development) atau domain spesifik (production)
// Access-Control-Allow-Methods: GET, POST, PUT, DELETE, PATCH, OPTIONS
// Access-Control-Allow-Headers: Content-Type, Authorization, X-Correlation-ID, X-Idempotency-Key
// Access-Control-Expose-Headers: X-Correlation-ID
// Access-Control-Max-Age: 86400

// Handle preflight (OPTIONS) request → return 204
```

#### 3e. Rate Limiter Middleware (Redis-based)

```go
// Algoritma: Sliding Window Counter
// 
// Cara kerja:
// 1. Tentukan identifier: IP (anonymous) atau user_id (authenticated)
// 2. Key di Redis: "rate_limit:<identifier>:<window_start>"
// 3. INCR key, set EXPIRE = window duration
// 4. Jika count > limit → return 429 Too Many Requests
//
// Tiers:
//   Anonymous:     30 req/menit
//   Authenticated: 200 req/menit
//   Admin:         1000 req/menit
//
// Response 429:
// {
//   "success": false,
//   "error": {
//     "code": "RATE_LIMIT_EXCEEDED",
//     "message": "Too many requests. Please try again later."
//   }
// }
//
// Headers yang harus di-set:
// X-RateLimit-Limit: 200
// X-RateLimit-Remaining: 150
// X-RateLimit-Reset: 1705312800 (Unix timestamp)
```

#### 3f. JWT Authentication Middleware

```go
// Public routes (SKIP JWT validation):
//   POST /api/v1/auth/register
//   POST /api/v1/auth/login
//   POST /api/v1/auth/refresh
//   POST /api/v1/auth/forgot-password
//   POST /api/v1/auth/verify-email
//   POST /api/v1/auth/verify-email/confirm
//   POST /api/v1/auth/reset-password
//   GET  /api/v1/auth/google
//   GET  /api/v1/auth/google/callback
//   GET  /health
//   GET  /ready
//   GET  /live
//
// Protected routes:
// 1. Ambil token dari header: Authorization: Bearer <token>
// 2. Parse dan validasi JWT (check signature, expiry)
// 3. Extract claims: user_id, email, role
// 4. Set claims di context
// 5. Jika invalid → return 401 Unauthorized
//
// Library: github.com/golang-jwt/jwt/v5
```

### Step 4: Reverse Proxy

```go
// Route mapping:
// /api/v1/auth/*     → AUTH_SERVICE_URL
// /api/v1/users/*    → USER_SERVICE_URL
// /api/v1/wallets/*  → WALLET_SERVICE_URL
// /api/v1/transactions/* → TRANSACTION_SERVICE_URL
// /api/v1/payments/* → PAYMENT_SERVICE_URL
// /api/v1/audit/*    → AUDIT_SERVICE_URL
//
// Cara implement:
// Gunakan net/http/httputil.NewSingleHostReverseProxy()
//
// Yang perlu diperhatikan:
// 1. Forward semua headers (termasuk Authorization, X-Correlation-ID)
// 2. Forward body
// 3. Set X-Forwarded-For header
// 4. Set timeout: 30 detik
// 5. Handle proxy error → return 502 Bad Gateway

import "net/http/httputil"

func NewProxy(targetURL string) *httputil.ReverseProxy {
    url, _ := url.Parse(targetURL)
    proxy := httputil.NewSingleHostReverseProxy(url)
    
    // Custom error handler
    proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
        // Return 502 Bad Gateway
    }
    
    return proxy
}
```

### Step 5: Health Checks

Gunakan shared `pkg/healthcheck` package, register ke router:

```go
router.GET("/health", healthHandler.Health)
router.GET("/ready", healthHandler.Ready)
router.GET("/live", healthHandler.Live)
```

### Step 6: Graceful Shutdown

```go
// Di cmd/main.go:
func main() {
    // ... setup router, middleware, proxy ...
    
    srv := &http.Server{
        Addr:         ":" + cfg.Port,
        Handler:      router,
        ReadTimeout:  15 * time.Second,
        WriteTimeout: 15 * time.Second,
        IdleTimeout:  60 * time.Second,
    }
    
    // Start server in goroutine
    go func() {
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatal("server error", zap.Error(err))
        }
    }()
    
    // Wait for interrupt signal
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit
    
    log.Info("shutting down server...")
    
    // Give outstanding requests 30 seconds to complete
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    if err := srv.Shutdown(ctx); err != nil {
        log.Fatal("server forced to shutdown", zap.Error(err))
    }
    
    // Close resources
    redisClient.Close()
    
    log.Info("server exited gracefully")
}
```

### Step 7: Entry Point (`cmd/main.go`)

Struktur main.go:
```go
func main() {
    // 1. Load config
    // 2. Initialize logger
    // 3. Initialize Redis client (untuk rate limiter)
    // 4. Setup Gin router
    // 5. Register middleware (urutan penting!)
    // 6. Register health check routes
    // 7. Register proxy routes
    // 8. Start server
    // 9. Graceful shutdown
}
```

### Step 8: Test Manual

```bash
# 1. Jalankan API Gateway
cd api-gateway
go run cmd/main.go

# 2. Test health checks
curl http://localhost:8080/health
curl http://localhost:8080/ready
curl http://localhost:8080/live

# 3. Test CORS (check response headers)
curl -i -X OPTIONS http://localhost:8080/api/v1/auth/login

# 4. Test rate limiter (kirim banyak request)
for i in {1..35}; do
  curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/health
done
# Seharusnya ada beberapa yang return 429

# 5. Test correlation ID
curl -i http://localhost:8080/health
# Check header X-Correlation-ID di response

# 6. Test JWT (tanpa token ke protected route)
curl -i http://localhost:8080/api/v1/users/me
# Expected: 401 Unauthorized

# 7. Test graceful shutdown
# Start server, lalu tekan Ctrl+C
# Log harus menunjukkan "shutting down server..." dan "server exited gracefully"
```

---

## ✅ Acceptance Criteria

- [ ] API Gateway berjalan di port 8080
- [ ] `GET /health` return `{"status":"ok"}`
- [ ] `GET /ready` return status setiap dependency
- [ ] `GET /live` return `{"status":"alive"}`
- [ ] Correlation ID ada di setiap response header `X-Correlation-ID`
- [ ] Structured JSON log untuk setiap request (method, path, status, duration)
- [ ] CORS headers ada di response
- [ ] Rate limiter memblokir request berlebih (return 429)
- [ ] JWT validation bekerja (401 untuk request tanpa token ke protected route)
- [ ] Public routes bisa diakses tanpa token
- [ ] Graceful shutdown bekerja
- [ ] Request ke `/api/v1/auth/*` di-proxy ke Auth Service URL (akan 502 karena service belum ada — ini expected)

---

## 💡 Tips & Common Pitfalls

1. **Gin sudah punya Recovery middleware** — `gin.Recovery()`. Kamu bisa pakai ini lalu customise output-nya.

2. **Jangan lupa CORS preflight** — Browser kirim `OPTIONS` request sebelum `POST/PUT/DELETE`. Jika tidak dihandle, frontend akan error.

3. **Rate limiter testing** — Gunakan `for` loop di bash untuk test, bukan Postman (Postman terlalu lambat).

4. **Reverse proxy 502** — Ini normal! Backend service belum ada. Yang penting proxy-nya sudah routing dengan benar.

5. **JWT secret harus sama** — Secret di Gateway HARUS sama dengan di Auth Service. Nanti di Episode 3.

---

## 📚 Referensi Belajar

- [Gin Web Framework](https://gin-gonic.com/docs/)
- [Gin Middleware](https://gin-gonic.com/docs/examples/custom-middleware/)
- [Go Reverse Proxy](https://pkg.go.dev/net/http/httputil#ReverseProxy)
- [Sliding Window Rate Limiter](https://blog.logrocket.com/rate-limiting-go-application/)
- [JWT Introduction](https://jwt.io/introduction)
- [Graceful Shutdown in Go](https://pkg.go.dev/net/http#Server.Shutdown)
