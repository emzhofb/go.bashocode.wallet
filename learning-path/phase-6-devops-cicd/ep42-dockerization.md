# Episode 42: Containerizing all Microservices

## 🎯 Tujuan
* Menulis **Dockerfile** berbasis *Multi-stage Build* untuk masing-masing service (Gateway, Auth, Wallet, Notification, Audit) agar ukuran image Docker sangat kecil dan aman.
* Menyatukan seluruh infrastruktur (MySQL, MongoDB, Redis, RabbitMQ, Jaeger, Prometheus, Grafana, ELK) dan microservices buatan kita ke dalam satu berkas **`docker-compose.yml`** utuh.
* Menjalankan seluruh sistem monorepo microservices dengan satu perintah `docker compose up --build`.

---

## 📐 Konsep Multi-stage Build Docker
Di Go, kompilasi kode menghasilkan file biner (*executable binary*) mandiri. Kita tidak membutuhkan compiler Go lagi saat aplikasi dijalankan di production server.
* **Stage 1 (Builder):** Kita menggunakan image utuh `golang:alpine` untuk men-download dependencies dan meng-compile program Go menjadi file biner.
* **Stage 2 (Runtime):** Kita menggunakan image minimal `alpine:latest` (hanya berukuran ~5MB), lalu menyalin file biner hasil Stage 1 ke dalamnya. 
* **Hasilnya:** Ukuran akhir Docker image kita turun drastis dari 800MB (jika menggunakan runtime Go penuh) menjadi **hanya 15MB**! Selain menghemat kuota & disk, ini sangat aman karena mengurangi celah keamanan (*attack surface*).

---

## 📦 Langkah-langkah

### Step 1: Membuat Dockerfile Multi-stage
Buat file bernama `Dockerfile` di setiap folder microservice (misal di folder `api-gateway/`, `auth-service/`, dan `wallet-service/`):

```dockerfile
# ==========================================
# STAGE 1: Build Binary
# ==========================================
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy file go.mod dan go.sum (jika ada) untuk optimalisasi caching layer
COPY go.mod ./
RUN go mod download

# Copy source code lengkap
COPY . .

# Build program Go menjadi single biner static
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /service ./cmd/main.go

# ==========================================
# STAGE 2: Runtime Image Minimal
# ==========================================
FROM alpine:3.19

# Install sertifikat CA untuk mendukung request HTTPS outbound
RUN apk --no-cache add ca-certificates tzdata

# Buat user non-root demi alasan keamanan (security hardening)
RUN adduser -D -g '' appuser

WORKDIR /app

# Salin file binary dari Stage 1
COPY --from=builder /service .

# Gunakan user non-root
USER appuser

# Expose port (sesuai port service masing-masing)
EXPOSE 8080

# Jalankan aplikasi
CMD ["./service"]
```

### Step 2: Menyatukan Semua Service ke `docker-compose.yml`
Di folder root workspace, kita perbarui file `docker-compose.yml` agar mem-build seluruh microservices buatan kita dan menghubungkannya dalam satu jaringan Docker network (`gowallet-network`).

```yaml
version: '3.8'

services:
  # ----------------------------------------------------
  # INFRASTRUCTURE DEPENDENCIES (MySQL, Redis, RabbitMQ, dll)
  # ----------------------------------------------------
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
    networks:
      - gowallet-network

  redis:
    image: redis:7-alpine
    container_name: gowallet-redis
    ports:
      - "6379:6379"
    networks:
      - gowallet-network

  rabbitmq:
    image: rabbitmq:3-management-alpine
    container_name: gowallet-rabbitmq
    ports:
      - "5672:5672"
      - "15672:15672"
    networks:
      - gowallet-network

  # ----------------------------------------------------
  # APPLICATION SERVICES (Microservices)
  # ----------------------------------------------------
  api-gateway:
    build:
      context: ./api-gateway
      dockerfile: Dockerfile
    container_name: gowallet-gateway
    ports:
      - "8080:8080"
    environment:
      - PORT=8080
      - USER_SERVICE_URL=http://auth-service:8081 # Arahkan ke nama container docker
      - WALLET_SERVICE_URL=http://wallet-service:8082
    depends_on:
      - auth-service
      - wallet-service
    networks:
      - gowallet-network

  auth-service:
    build:
      context: ./auth-service
      dockerfile: Dockerfile
    container_name: gowallet-auth
    ports:
      - "8081:8081"
      - "50051:50051" # Expose gRPC port
    environment:
      - PORT=8081
      - DB_HOST=mysql
      - DB_PORT=3306
      - DB_USER=gowallet_user
      - DB_PASSWORD=gowallet_password
      - DB_NAME=gowallet
    depends_on:
      - mysql
    networks:
      - gowallet-network

  wallet-service:
    build:
      context: ./wallet-service
      dockerfile: Dockerfile
    container_name: gowallet-wallet
    ports:
      - "8082:8082"
    environment:
      - PORT=8082
      - DB_HOST=mysql
      - DB_PORT=3306
      - DB_USER=gowallet_user
      - DB_PASSWORD=gowallet_password
      - DB_NAME=gowallet
      - REDIS_HOST=redis
      - REDIS_PORT=6379
      - RABBITMQ_URL=amqp://guest:guest@rabbitmq:5672/
      - AUTH_GRPC_ADDR=auth-service:50051 # Arahkan gRPC client ke container auth-service
    depends_on:
      - mysql
      - redis
      - rabbitmq
    networks:
      - gowallet-network

networks:
  gowallet-network:
    driver: bridge

volumes:
  mysql_data:
```

### Step 3: Run & Test Stack
Matikan seluruh proses aplikasi yang sedang berjalan di komputer lokal Anda, lalu jalankan perintah ini dari folder root workspace:

```bash
docker compose up --build
```
Docker akan mendownload base image, mem-build seluruh microservices Go kita secara bertahap, dan menyalakan semuanya secara orkestrasi. 

---

## ✅ Acceptance Criteria
* [ ] Setiap service memiliki file `Dockerfile` valid berbasis multi-stage build.
* [ ] Menjalankan `docker compose up --build` sukses menyalakan database infrastruktur dan ketiga microservices Go kita.
* [ ] Memanggil endpoint `POST http://localhost:8080/api/v1/auth/login` berhasil mengembalikan JWT Token (menandakan gateway sukses mem-proxy request ke container `auth-service` di dalam Docker bridge network).

---

## 💡 Tips untuk Junior
* **Use Non-root User:** Menambahkan instruksi `USER appuser` di Dockerfile sangat penting untuk security hardening. Secara default, Docker menjalankan aplikasi dengan user `root`. Jika ada celah keamanan RCE (*Remote Code Execution*) di aplikasi kita, hacker otomatis menguasai server host kita sebagai root. Dengan user non-root, hak akses peretas sangat dibatasi.

---

## 📚 Referensi Belajar
* [Docker Multi-stage Builds (Official Guide)](https://docs.docker.com/build/building/multi-stage/)
* [Docker Compose Networking tutorial](https://docs.docker.com/compose/networking/)
