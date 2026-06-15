# Episode 30: Reliability Patterns (Circuit Breakers & Retries)

## 🎯 Tujuan
* Mengenalkan pola **Circuit Breaker** untuk mengisolasi kegagalan jaringan internal dan mencegah *cascade failure* (sistem runtuh domino).
* Mengimplementasikan Circuit Breaker menggunakan library **sony/gobreaker** pada gRPC call.
* Menerapkan **Retry Policy** dengan *Exponential Backoff* dan *Jitter* pada kegagalan request temporal.
* Mengonfigurasi **Request Timeout** di seluruh level koneksi.

---

## 📐 Konsep Circuit Breaker & Retry
Dalam arsitektur microservices, kegagalan salah satu service (misal `auth-service` mati) tidak boleh meruntuhkan service lain (`wallet-service`). 
* **Circuit Breaker (CB)** bertindak sebagai saklar otomatis:
  * **Closed (Normal):** Aliran request berjalan lancar.
  * **Open (Trip):** Jika tingkat error tinggi (misal 5 kali gagal berturut-turut), CB langsung membuka saklar. Request berikutnya langsung dibatalkan di awal (*fast fail*) tanpa membebani target service yang sedang sekarat.
  * **Half-Open:** Setelah beberapa detik, CB mencoba meloloskan sedikit request untuk mengetes apakah target service sudah sembuh. Jika sukses, status kembali ke **Closed**.
* **Retry dengan Exponential Backoff & Jitter:** Jika kegagalan hanya bersifat sementara (*network hiccup*), kita coba memanggil ulang. Waktu jeda tunggu dilipatgandakan (misal 1s, 2s, 4s) ditambah variasi acak (*jitter*) agar server target tidak terbebani secara berbarengan.

---

## 📦 Langkah-langkah

### Step 1: Install Library Circuit Breaker
Unduh library `gobreaker` dari Sony:
```bash
go get github.com/sony/gobreaker
```

### Step 2: Membuat Wrapper gRPC Client dengan Circuit Breaker
Kita buka `wallet-service` dan bungkus panggilan gRPC Client ke `auth-service` menggunakan `gobreaker`.

Buat file baru di `wallet-service/internal/user/client/grpc_client.go` (atau di helper service):

```go
package client

import (
	"context"
	"time"

	pb "github.com/emzhofb/gowallet/auth-service/proto/user"
	"github.com/sony/gobreaker"
	"google.golang.org/grpc"
)

type ProtectedUserClient struct {
	client pb.UserServiceClient
	cb     *gobreaker.CircuitBreaker
}

func NewProtectedUserClient(grpcConn *grpc.ClientConn) *ProtectedUserClient {
	client := pb.NewUserServiceClient(grpcConn)

	// Inisialisasi Circuit Breaker Settings
	cbSettings := gobreaker.Settings{
		Name:        "auth-service-cb",
		MaxRequests: 3,                          // Maksimal request lolos saat HALF-OPEN
		Interval:    10 * time.Second,           // Reset counter interval saat CLOSED
		Timeout:     30 * time.Second,           // Durasi saklar tetap OPEN sebelum ke HALF-OPEN
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// Trip jika terjadi lebih dari 5 kali gagal beruntun
			return counts.ConsecutiveFailures > 5
		},
	}

	return &ProtectedUserClient{
		client: client,
		cb:     gobreaker.NewCircuitBreaker(cbSettings),
	}
}

func (c *ProtectedUserClient) GetUserByEmail(ctx context.Context, email string) (*pb.UserResponse, error) {
	// Jalankan request di dalam proteksi Circuit Breaker
	result, err := c.cb.Execute(func() (interface{}, error) {
		
		// Set timeout spesifik (maksimal 3 detik menunggu)
		timeoutCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		return c.client.GetUserByEmail(timeoutCtx, &pb.GetUserByEmailRequest{Email: email})
	})

	if err != nil {
		return nil, err
	}

	return result.(*pb.UserResponse), nil
}
```

### Step 3: Implementasi Retry Policy dengan Exponential Backoff & Jitter
Jika query database gagal sesaat, kita gunakan fungsi helper berikut untuk mencoba ulang secara cerdas:

Buat file helper di `wallet-service/internal/utils/retry.go`:

```go
package utils

import (
	"context"
	"math"
	"math/rand"
	"time"

	"github.com/emzhofb/gowallet/wallet-service/internal/logger"
)

func RetryWithBackoff(ctx context.Context, maxRetries int, fn func() error) error {
	var lastErr error
	baseBackoff := 1 * time.Second

	for i := 0; i < maxRetries; i++ {
		err := fn()
		if err == nil {
			return nil // Sukses!
		}
		lastErr = err

		if i < maxRetries-1 {
			// Rumus Exponential Backoff: base * 2^attempt
			backoff := time.Duration(float64(baseBackoff) * math.Pow(2, float64(i)))
			
			// Tambahkan Jitter (acak ±25%) agar tidak tabrakan
			jitter := time.Duration(rand.Float64() * 0.5 * float64(backoff))
			sleepTime := backoff + jitter - (backoff / 4)

			logger.Warn(ctx, "Operation failed, retrying...", "attempt", i+1, "sleep_time", sleepTime, "error", err.Error())

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(sleepTime):
			}
		}
	}
	return lastErr
}
```

---

## ✅ Acceptance Criteria
* [ ] Memanggil gRPC ketika target service dimatikan memicu status Circuit Breaker berubah menjadi `OPEN`.
* [ ] Ketika status CB adalah `OPEN`, request berikutnya langsung gagal instan dengan error `circuit breaker is open` tanpa menunggu koneksi timeout TCP yang lama.
* [ ] Logika retry dengan exponential backoff sukses mengulang pemanggilan database yang gagal dengan jeda waktu yang melipat ganda secara dinamis.

---

## 💡 Tips untuk Junior
* **Gunakan gRPC Deadline/Timeout:** Jangan pernah membiarkan request berjalan tanpa batas waktu (*infinite timeout*). Selalu gunakan `context.WithTimeout` di sisi client. Request tanpa timeout adalah salah satu penyebab utama kebocoran memori (*goroutine leak*) di production.

---

## 📚 Referensi Belajar
* [Circuit Breaker Pattern - Martin Fowler](https://martinfowler.com/bliki/CircuitBreaker.html)
* [Exponential Backoff And Jitter - AWS Blog](https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/)
