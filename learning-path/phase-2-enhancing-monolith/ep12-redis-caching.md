# Episode 12: Redis Caching

## 🎯 Tujuan
* Menambahkan **Redis** ke dalam Docker Compose sebagai media caching memori berkecepatan tinggi.
* Mengimplementasikan pola **Cache-Aside** (membaca dari cache dulu, jika kosong baru baca dari DB dan simpan ke cache).
* Mengatasi masalah **Cache Invalidation** (memastikan cache dihapus ketika ada perubahan data saldo agar data tidak basi).

---

## 📐 Konsep Cache-Aside Pattern
Untuk mengurangi beban query MySQL saat pengguna sering memeriksa saldo wallet, kita menyimpan saldo di Redis dengan masa aktif (Time to Live / TTL) tertentu, misalnya 5 menit.

```
                  ┌──────────────────────┐
                  │ GET /wallets/me      │
                  └──────────┬───────────┘
                             ▼
                    [ Cek di Redis? ]
                     /             \
             (Ada/Hit)             (Kosong/Miss)
                   /                 \
                  ▼                   ▼
           [ Kembalikan ]      [ Query MySQL ]
                                      │
                                      ▼
                               [ Simpan ke Redis ]
                                      │
                                      ▼
                               [ Kembalikan ]
```

### Aturan Emas Caching:
* **Selalu Invalidate saat Write:** Saat saldo bertambah atau berkurang (misal setelah Transfer), kita **harus menghapus** key cache di Redis agar request cek saldo berikutnya terpaksa mengambil data terbaru dari database MySQL.

---

## 📦 Langkah-langkah

### Step 1: Tambahkan Redis ke Docker Compose
Buka file `docker-compose.yml` di folder root `monolith/`, tambahkan service `redis`:

```yaml
# ... service mysql ...
  redis:
    image: redis:7-alpine
    container_name: gowallet-redis
    ports:
      - "6379:6379"
    restart: always
```

Jalankan container:
```bash
docker compose up -d
```

### Step 2: Install Library Redis Client
Unduh official library client Redis untuk Go:
```bash
go get github.com/redis/go-redis/v9
```

### Step 3: Update `.env` & Config Loader
Tambahkan variabel environment untuk Redis di `.env`:
```env
# ... mysql config ...
REDIS_HOST=localhost
REDIS_PORT=6379
```

Buka `internal/config/config.go`, sesuaikan struct `Config` dan loader-nya:
```go
type Config struct {
	DBDSN     string
	RedisAddr string
}

func LoadConfig() *Config {
	// ... godotenv.Load ...
	
	dsn := os.Getenv("DB_USER") + ":" + os.Getenv("DB_PASSWORD") +
		"@tcp(" + os.Getenv("DB_HOST") + ":" + os.Getenv("DB_PORT") + ")/" +
		os.Getenv("DB_NAME") + "?parseTime=true"
        
	redisAddr := os.Getenv("REDIS_HOST") + ":" + os.Getenv("REDIS_PORT")

	return &Config{
		DBDSN:     dsn,
		RedisAddr: redisAddr,
	}
}
```

### Step 4: Membuat Redis Wrapper (`internal/database/redis.go`)
Buat file helper koneksi Redis di `internal/database/redis.go`:

```go
package database

import (
	"context"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

func ConnectRedis(addr string) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: "", // Default tanpa password di local dev
		DB:       0,  // Gunakan default DB
	})

	// Cek koneksi dengan Ping
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		return nil, err
	}

	log.Println("Successfully connected to Redis!")
	return rdb, nil
}
```

### Step 5: Implementasi Caching di WalletService
Buka `internal/wallet/service/service.go`. Kita akan memodifikasi `walletService` agar menerima `*redis.Client` dan menerapkan pola Cache-Aside.

```go
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	customErr "github.com/emzhofb/gowallet/monolith/internal/errors"
	"github.com/emzhofb/gowallet/monolith/internal/logger"
	"github.com/emzhofb/gowallet/monolith/internal/wallet/model"
	"github.com/emzhofb/gowallet/monolith/internal/wallet/repository"
	"github.com/redis/go-redis/v9"
)

type walletService struct {
	repo  repository.WalletRepository
	rdb   *redis.Client
}

func NewWalletService(repo repository.WalletRepository, rdb *redis.Client) WalletService {
	return &walletService{
		repo: repo,
		rdb:  rdb,
	}
}

func (s *walletService) GetWalletByUserID(ctx context.Context, userID string) (*model.Wallet, error) {
	cacheKey := fmt.Sprintf("wallet:user:%s", userID)

	// 1. Cek data di Redis Cache
	cachedVal, err := s.rdb.Get(ctx, cacheKey).Result()
	if err == nil {
		// Cache HIT! Deserialisasi JSON string ke struct model.Wallet
		wallet := &model.Wallet{}
		if err := json.Unmarshal([]byte(cachedVal), wallet); err == nil {
			logger.Info(ctx, "Cache hit for wallet", "user_id", userID)
			return wallet, nil
		}
	}

	// 2. Cache MISS! Ambil dari MySQL database
	logger.Info(ctx, "Cache miss for wallet. Fetching from MySQL...", "user_id", userID)
	wallet, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, customErr.NewAppError(http.StatusNotFound, "WALLET_NOT_FOUND", "Wallet pengguna tidak ditemukan.")
	}

	// 3. Simpan ke Redis dengan TTL 5 menit
	walletBytes, err := json.Marshal(wallet)
	if err == nil {
		// Set dengan durasi 5 menit
		s.rdb.Set(ctx, cacheKey, walletBytes, 5*time.Minute)
	}

	return wallet, nil
}
```

### Step 6: Invalidate Cache di TransactionService (saat Transfer)
Buka `internal/transaction/service/service.go`. Tambahkan parameter `*redis.Client` ke struct `transactionService` dan `NewTransactionService`.

Setelah proses `tx.Commit()` sukses di method `Transfer`, tambahkan kode untuk menghapus cache milik pengirim dan penerima:

```go
// Di dalam method Transfer setelah tx.Commit() sukses:
	
	// 10. Invalidate Redis Cache untuk pengirim & penerima agar saldo ter-update
	senderCacheKey := "wallet:user:" + senderUserID
	receiverCacheKey := "wallet:user:" + receiverUser.ID
	
	// Hapus key cache secara asinkronus (tidak memblokir response HTTP)
	go func() {
		s.rdb.Del(context.Background(), senderCacheKey, receiverCacheKey)
	}()
```

### Step 7: Update `cmd/main.go`
Koneksikan Redis saat aplikasi start, lalu suntikkan instance `rdb` ke `walletService` dan `transactionService`:

```go
    // ...
	// Connect to MySQL
	db, err := database.ConnectWithRetry(cfg.DBDSN)
	if err != nil {
		log.Fatalf("Could not connect to MySQL: %v", err)
	}
	defer db.Close()

	// Connect to Redis
	rdb, err := database.ConnectRedis(cfg.RedisAddr)
	if err != nil {
		log.Fatalf("Could not connect to Redis: %v", err)
	}
	defer rdb.Close()

	// Inisialisasi Layer
	uRepo := userRepository.NewMySQLUserRepository(db)
	wRepo := walletRepository.NewMySQLWalletRepository(db)
	lRepo := ledgerRepository.NewMySQLLedgerRepository(db)
	txRepo := transactionRepository.NewMySQLTransactionRepository(db)
	
	uSvc := userService.NewUserService(db, uRepo, wRepo) 
	wSvc := walletService.NewWalletService(wRepo, rdb) // Inject Redis
	txSvc := transactionService.NewTransactionService(db, txRepo, uRepo, wRepo, lRepo, rdb) // Inject Redis
    // ...
```

---

## ✅ Acceptance Criteria
* [ ] Container Redis berjalan lancar (`docker ps` menampilkan port `6379`).
* [ ] Memanggil endpoint `GET /api/v1/wallets/me` pertama kali memicu log `Cache miss...` (membaca dari MySQL).
* [ ] Memanggil endpoint yang sama untuk kedua kalinya memicu log `Cache hit...` (membaca instan dari Redis, tidak ada query ke MySQL).
* [ ] Setelah melakukan transfer uang, memanggil `GET /api/v1/wallets/me` mengembalikan nilai saldo yang berkurang (cache sukses dihapus dan di-update kembali dari database).

---

## 💡 Tips untuk Junior
* **Marshal/Unmarshal:** Objek kompleks di Go tidak bisa disimpan langsung ke Redis secara mentah. Kita harus mengubahnya ke string JSON menggunakan `json.Marshal` (*Serialization*), dan membacanya kembali ke struct Go menggunakan `json.Unmarshal` (*Deserialization*).
* **Asynchronous Cache Invalidation:** Menghapus key Redis menggunakan goroutine `go func() { s.rdb.Del(...) }()` adalah teknik optimasi latensi agar client mendapat respons transfer lebih cepat tanpa harus menunggu proses penghapusan jaringan Redis selesai.

---

## 📚 Referensi Belajar
* [Cache-Aside Pattern Explanation](https://learn.microsoft.com/en-us/azure/architecture/patterns/cache-aside)
* [go-redis Official Guide](https://redis.uptrace.dev/)
