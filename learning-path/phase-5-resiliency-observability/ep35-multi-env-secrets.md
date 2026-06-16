# Episode 35: Secret Management & Multi-environment Config

## 🎯 Tujuan
* Memahami bahaya menyimpan kredensial sensitif (*secrets*) di dalam repositori kode.
* Membangun pemuat konfigurasi (*Config Loader*) terpusat menggunakan library **Viper** atau **godotenv** untuk memuat berkas konfigurasi spesifik lingkungan (.env.development, .env.staging, .env.production).
* Menambahkan `.env` ke `.gitignore` untuk melindungi kerahasiaan data rahasia.

---

## 📐 Mengapa Butuh Multi-environment & Secret Management?
Selama development, kita menghubungkan kode kita ke MySQL lokal (`localhost:3306`). Namun di server staging dan production, alamat host MySQL berubah menjadi RDS Cloud atau kluster database internal yang diamankan dengan password yang sangat kuat.
* **JANGAN PERNAH** melakukan hardcode terhadap password, secret key JWT, API key, atau token pihak ketiga.
* **Solusi:** Kita menyimpan seluruh nilai dinamis tersebut dalam berkas `.env` terpisah untuk masing-masing lingkungan, dan memuatnya saat runtime berdasarkan variabel `APP_ENV` (misal: `APP_ENV=staging`).

```
[ Local Dev ]   ➔  Loads .env.development  ➔ Connects to localhost
[ Staging ]     ➔  Loads .env.staging      ➔ Connects to staging-db
[ Production ]  ➔  Loads Environment Vars  ➔ Connects to cloud secrets manager
```

---

## 📦 Langkah-langkah

### Step 1: Install Library Configuration Loader
Kita gunakan library **Viper** dari spf13 yang merupakan standar de facto untuk manajemen konfigurasi di ekosistem Go:

```bash
go get github.com/spf13/viper
```

### Step 2: Membuat File Konfigurasi `.env`
Buat tiga berkas konfigurasi di root folder microservice Anda (misal di `user-service/`):

`.env.development` (Untuk development lokal komputer):
```env
PORT=8084
DB_DSN=gowallet_user:gowallet_password@tcp(127.0.0.1:3306)/gowallet?parseTime=true
JWT_SECRET=super-secret-local-dev-key
APP_ENV=development
```

`.env.staging` (Untuk server staging testing):
```env
PORT=8084
DB_DSN=staging_user:StagingPass123!@tcp(staging-mysql-host:3306)/gowallet_staging?parseTime=true
JWT_SECRET=staging-only-secure-signing-key-109283
APP_ENV=staging
```

`.env.production` (Template untuk production, biasanya nilainya di-inject langsung oleh platform runner seperti Kubernetes / AWS):
```env
PORT=8084
DB_DSN=${DB_DSN_PROD}
JWT_SECRET=${JWT_SECRET_PROD}
APP_ENV=production
```

### Step 3: Membuat Config Loader (`internal/config/config.go`)
Buat file `config.go` untuk membaca berkas `.env` secara dinamis sesuai nilai variabel `APP_ENV`:

```go
package config

import (
	"log"
	"os"

	"github.com/spf13/viper"
)

type Config struct {
	Port      string `mapstructure:"PORT"`
	DBDSN     string `mapstructure:"DB_DSN"`
	JWTSecret string `mapstructure:"JWT_SECRET"`
	AppEnv    string `mapstructure:"APP_ENV"`
}

func LoadConfig() *Config {
	// 1. Dapatkan tipe lingkungan dari system environment variable
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development" // Default ke development
	}

	// 2. Set nama file config yang akan dibaca (misal: .env.development)
	viper.SetConfigName(".env." + env)
	viper.SetConfigType("env")
	
	// Cari file di root folder service saat ini
	viper.AddConfigPath(".")
	
	// Ijinkan membaca environment system variable secara langsung (prioritas tinggi)
	viper.AutomaticEnv()

	// 3. Baca config
	if err := viper.ReadInConfig(); err != nil {
		log.Printf("Warning: .env.%s file not found, loading config from system environment variables", env)
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		log.Fatalf("Unable to decode config into struct: %v", err)
	}

	return &cfg
}
```

### Step 4: Menambahkan Git Protection (`.gitignore`)
Untuk memastikan berkas `.env` yang berisi password asli kita tidak pernah ter-commit secara tidak sengaja ke GitHub, buat berkas `.gitignore` di root folder workspace:

```
# Ignore local environment config files containing secrets
.env.development
.env.staging
.env.production
.env

# Tetap commit file .env.example sebagai dokumentasi template bagi developer lain
!.env.example
```

---

## ✅ Acceptance Criteria
* [ ] File `.env.development` dan `.env.staging` ditolak saat melakukan git add (karena sudah masuk aturan `.gitignore`).
* [ ] Menjalankan aplikasi dengan perintah `APP_ENV=staging go run cmd/main.go` memicu pembacaan DSN database staging yang tertera di file `.env.staging`.
* [ ] Kredensial tidak lagi ter-hardcode di kode program.

---

## 💡 Tips untuk Junior
* **Secret Manager di Production:** Pada tingkatan production, hindari menyimpan file `.env` di server. Lebih baik gunakan Secret Management service seperti AWS Secrets Manager, HashiCorp Vault, atau Kubernetes Secrets yang langsung menyuntikkan (*inject*) nilai rahasia sebagai environment variables saat aplikasi dijalankan.
