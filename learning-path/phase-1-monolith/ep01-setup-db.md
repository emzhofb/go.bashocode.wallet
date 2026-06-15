# Episode 1: Setup Go & Database Connection

## 🎯 Tujuan
* Menyiapkan project directory untuk Monolith Wallet.
* Inisialisasi Go modules.
* Menjalankan database MySQL lokal menggunakan Docker Compose.
* Membuat program Go sederhana untuk koneksi ke database dengan fitur retry (backoff) jika koneksi gagal di awal.

---

## 📦 Langkah-langkah

### Step 1: Inisialisasi Project Directory
Buat folder baru bernama `monolith` untuk menyimpan seluruh kode aplikasi monolitik kita.

```bash
# Buat folder project
mkdir -p monolith/cmd monolith/internal/config monolith/internal/database
cd monolith

# Inisialisasi Go Module (gunakan nama module github Anda)
go mod init github.com/emzhofb/gowallet/monolith
```

### Step 2: Membuat Docker Compose untuk MySQL
Kita hanya akan menjalankan MySQL database saja untuk fase pertama ini agar hemat RAM laptop dan fokus.

Buat file `docker-compose.yml` di dalam folder root `monolith/`:

```yaml
version: '3.8'

services:
  mysql:
    image: mysql:8.0
    container_name: gowallet-mysql
    environment:
      MYSQL_ROOT_PASSWORD: rootpassword
      MYSQL_DATABASE: gowallet
      MYSQL_USER: gowallet_user
      MYSQL_PASSWORD: gowallet_password
    ports:
      - "3306:3306"
    volumes:
      - mysql_data:/var/lib/mysql
    restart: always

volumes:
  mysql_data:
```

Jalankan container database:
```bash
docker compose up -d
```

### Step 3: Setup Configuration Loader
Kita memerlukan cara aman untuk membaca kredensial database dari file `.env`. Kita gunakan library standard `joho/godotenv`.

Unduh library dotenv:
```bash
go get github.com/joho/godotenv
```

Buat file `.env` di folder root `monolith/`:
```env
DB_HOST=localhost
DB_PORT=3306
DB_USER=gowallet_user
DB_PASSWORD=gowallet_password
DB_NAME=gowallet
```

Buat code configuration loader di `internal/config/config.go`:

```go
package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DBDSN string
}

func LoadConfig() *Config {
	// Load file .env jika ada (biasanya di local dev)
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: No .env file found, using system environment variables")
	}

	dsn := os.Getenv("DB_USER") + ":" + os.Getenv("DB_PASSWORD") +
		"@tcp(" + os.Getenv("DB_HOST") + ":" + os.Getenv("DB_PORT") + ")/" +
		os.Getenv("DB_NAME") + "?parseTime=true"

	return &Config{
		DBDSN: dsn,
	}
}
```

### Step 4: Membuat Database Connection dengan Retry
Saat aplikasi dijalankan di production atau container Docker, database seringkali membutuhkan beberapa detik untuk *start-up* sempurna. Aplikasi Go kita harus tangguh dan mencoba menyambung kembali (*retry*) beberapa kali sebelum menyerah (*panic*).

Unduh driver MySQL untuk Go:
```bash
go get github.com/go-sql-driver/mysql
```

Buat file helper database di `internal/database/database.go`:

```go
package database

import (
	"database/sql"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func ConnectWithRetry(dsn string) (*sql.DB, error) {
	var db *sql.DB
	var err error
	maxRetries := 5
	backoff := 2 * time.Second

	for i := 1; i <= maxRetries; i++ {
		log.Printf("Connecting to database (Attempt %d/%d)...", i, maxRetries)
		db, err = sql.Open("mysql", dsn)
		if err == nil {
			// Lakukan Ping untuk memastikan koneksi benar-benar aktif
			err = db.Ping()
			if err == nil {
				log.Println("Successfully connected to database!")
				
				// Setup connection pool properties
				db.SetMaxOpenConns(25)
				db.SetMaxIdleConns(25)
				db.SetConnMaxLifetime(5 * time.Minute)
				
				return db, nil
			}
		}

		log.Printf("Database connection failed: %v. Retrying in %v...", err, backoff)
		time.Sleep(backoff)
		// Gandakan durasi tunggu untuk percobaan berikutnya (Exponential backoff)
		backoff *= 2
	}

	return nil, err
}
```

### Step 5: Membuat Main Entry Point
Mari kita satukan semuanya di `cmd/main.go`:

```go
package main

import (
	"log"

	"github.com/emzhofb/gowallet/monolith/internal/config"
	"github.com/emzhofb/gowallet/monolith/internal/database"
)

func main() {
	log.Println("Starting Monolith Wallet Application...")

	// 1. Load configuration
	cfg := config.LoadConfig()

	// 2. Connect to database with retry
	db, err := database.ConnectWithRetry(cfg.DBDSN)
	if err != nil {
		log.Fatalf("Critical Error: Could not connect to database after retries: %v", err)
	}
	defer db.Close()

	log.Println("Application successfully initialized. Ready to build features!")
}
```

Jalankan aplikasi untuk mencoba koneksi:
```bash
go run cmd/main.go
```

---

## ✅ Acceptance Criteria
* [ ] Folder `monolith/` terinisialisasi sebagai Go module.
* [ ] Container MySQL berjalan di docker (`docker ps` menunjukkan port `3306`).
* [ ] File `.env` sukses dibaca.
* [ ] Menjalankan `go run cmd/main.go` menghasilkan log `Successfully connected to database!`.
* [ ] Jika container database dimatikan, aplikasi Go akan mencoba menyambung ulang sebanyak 5 kali lalu keluar dengan log error yang jelas.

---

## 💡 Tips untuk Junior
* **Connection Pool:** Selalu atur `SetMaxOpenConns` dan `SetMaxIdleConns`. Koneksi yang dibiarkan tanpa limit dapat menyebabkan database kehabisan resource (*too many connections error*).
* **Exponential Backoff:** Kita melipatgandakan waktu tunggu (`backoff *= 2`) setiap kali gagal. Ini adalah praktek terbaik di industri agar server database kita tidak "dibombardir" request terus menerus saat sedang down/restart.
* **Driver Import:** Perhatikan baris `_ "github.com/go-sql-driver/mysql"`. Kita menggunakan underscore (`_`) karena kita hanya membutuhkan fungsi `init()` dari package driver tersebut untuk mendaftarkan dirinya ke package bawaan Go `database/sql`.

---

## 📚 Referensi Belajar
* [Go database/sql tutorial](https://go.dev/doc/database/)
* [Managing database connection pools in Go](https://www.alexedwards.net/blog/configuring-sqldb)
* [Exponential Backoff & Jitter explanation](https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/)
