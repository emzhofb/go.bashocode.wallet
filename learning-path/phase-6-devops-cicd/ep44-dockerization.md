# Episode44: Containerizing all Microservices

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
Di folder root workspace, kita buat file **`init.sql`** untuk menginisialisasi database yang terpisah untuk masing-masing service (prinsip *database-per-service*).

Buat file `init.sql` di root directory:
```sql
CREATE DATABASE IF NOT EXISTS gowallet_auth;
CREATE DATABASE IF NOT EXISTS gowallet_user;
CREATE DATABASE IF NOT EXISTS gowallet_wallet;
CREATE DATABASE IF NOT EXISTS gowallet_ledger;
CREATE DATABASE IF NOT EXISTS gowallet_transactions;

GRANT ALL PRIVILEGES ON gowallet_auth.* TO 'gowallet_user'@'%';
GRANT ALL PRIVILEGES ON gowallet_user.* TO 'gowallet_user'@'%';
GRANT ALL PRIVILEGES ON gowallet_wallet.* TO 'gowallet_user'@'%';
GRANT ALL PRIVILEGES ON gowallet_ledger.* TO 'gowallet_user'@'%';
GRANT ALL PRIVILEGES ON gowallet_transactions.* TO 'gowallet_user'@'%';
FLUSH PRIVILEGES;
```

Sekarang, perbarui file `docker-compose.yml` di folder root workspace agar mem-build seluruh **10 microservices** buatan kita dan menghubungkannya dalam satu jaringan Docker network (`gowallet-network`) beserta 4 dependency infrastrukturnya:

```yaml
version: '3.8'

services:
  # ----------------------------------------------------
  # INFRASTRUCTURE DEPENDENCIES
  # ----------------------------------------------------
  mysql:
    image: mysql:8.0
    container_name: gowallet-mysql
    environment:
      MYSQL_ROOT_PASSWORD: rootpassword
      MYSQL_USER: gowallet_user
      MYSQL_PASSWORD: gowallet_password
    ports:
      - "3306:3306"
    volumes:
      - mysql_data:/var/lib/mysql
      - ./init.sql:/docker-entrypoint-initdb.d/init.sql # Database per Service init script
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

  mongodb:
    image: mongo:6.0
    container_name: gowallet-mongodb
    ports:
      - "27017:27017"
    volumes:
      - mongodb_data:/data/db
    networks:
      - gowallet-network

  # ----------------------------------------------------
  # CORE APPLICATION MICROSERVICES
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
      - AUTH_SERVICE_URL=http://auth-service:8081
      - USER_SERVICE_URL=http://user-service:8084
      - WALLET_SERVICE_URL=http://wallet-service:8082
      - TRANSACTION_SERVICE_URL=http://transaction-service:8086
      - PAYMENT_SERVICE_URL=http://payment-service:8083
    depends_on:
      - auth-service
      - user-service
      - wallet-service
      - transaction-service
      - payment-service
    networks:
      - gowallet-network

  auth-service:
    build:
      context: ./auth-service
      dockerfile: Dockerfile
    container_name: gowallet-auth
    ports:
      - "8081:8081"
      - "50051:50051"
    environment:
      - PORT=8081
      - DB_HOST=mysql
      - DB_PORT=3306
      - DB_USER=gowallet_user
      - DB_PASSWORD=gowallet_password
      - DB_NAME=gowallet_auth # DB Terpisah
      - REDIS_HOST=redis
      - REDIS_PORT=6379
      - USER_GRPC_ADDR=user-service:50052
    depends_on:
      - mysql
      - redis
    networks:
      - gowallet-network

  user-service:
    build:
      context: ./user-service
      dockerfile: Dockerfile
    container_name: gowallet-user
    ports:
      - "8084:8084"
      - "50052:50052"
    environment:
      - PORT=8084
      - DB_HOST=mysql
      - DB_PORT=3306
      - DB_USER=gowallet_user
      - DB_PASSWORD=gowallet_password
      - DB_NAME=gowallet_user # DB Terpisah
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
      - "50053:50053"
    environment:
      - PORT=8082
      - DB_HOST=mysql
      - DB_PORT=3306
      - DB_USER=gowallet_user
      - DB_PASSWORD=gowallet_password
      - DB_NAME=gowallet_wallet # DB Terpisah
      - REDIS_HOST=redis
      - REDIS_PORT=6379
      - AUTH_GRPC_ADDR=auth-service:50051
    depends_on:
      - mysql
      - redis
    networks:
      - gowallet-network

  ledger-service:
    build:
      context: ./ledger-service
      dockerfile: Dockerfile
    container_name: gowallet-ledger
    ports:
      - "50054:50054"
    environment:
      - DB_HOST=mysql
      - DB_PORT=3306
      - DB_USER=gowallet_user
      - DB_PASSWORD=gowallet_password
      - DB_NAME=gowallet_ledger # DB Terpisah
    depends_on:
      - mysql
    networks:
      - gowallet-network

  transaction-service:
    build:
      context: ./transaction-service
      dockerfile: Dockerfile
    container_name: gowallet-transaction
    ports:
      - "8086:8086"
      - "50056:50056"
    environment:
      - PORT=8086
      - DB_HOST=mysql
      - DB_PORT=3306
      - DB_USER=gowallet_user
      - DB_PASSWORD=gowallet_password
      - DB_NAME=gowallet_transactions # DB Terpisah
      - RABBITMQ_URL=amqp://guest:guest@rabbitmq:5672/
      - USER_GRPC_ADDR=user-service:50052
      - WALLET_GRPC_ADDR=wallet-service:50053
      - LEDGER_GRPC_ADDR=ledger-service:50054
    depends_on:
      - mysql
      - rabbitmq
    networks:
      - gowallet-network

  payment-service:
    build:
      context: ./payment-service
      dockerfile: Dockerfile
    container_name: gowallet-payment
    ports:
      - "8083:8083"
    environment:
      - PORT=8083
      - RABBITMQ_URL=amqp://guest:guest@rabbitmq:5672/
      - WEBHOOK_SECRET_KEY=super-secret-key-change-this
    depends_on:
      - rabbitmq
    networks:
      - gowallet-network

  notification-service:
    build:
      context: ./notification-service
      dockerfile: Dockerfile
    container_name: gowallet-notification
    environment:
      - RABBITMQ_URL=amqp://guest:guest@rabbitmq:5672/
    depends_on:
      - rabbitmq
    networks:
      - gowallet-network

  audit-service:
    build:
      context: ./audit-service
      dockerfile: Dockerfile
    container_name: gowallet-audit
    environment:
      - MONGO_URI=mongodb://mongodb:27017/gowallet_audit
      - RABBITMQ_URL=amqp://guest:guest@rabbitmq:5672/
    depends_on:
      - mongodb
      - rabbitmq
    networks:
      - gowallet-network

  scheduler-service:
    build:
      context: ./scheduler-service
      dockerfile: Dockerfile
    container_name: gowallet-scheduler
    environment:
      - AUTH_GRPC_ADDR=auth-service:50051
      - WALLET_GRPC_ADDR=wallet-service:50053
      - TRANSACTION_GRPC_ADDR=transaction-service:50056
    depends_on:
      - auth-service
      - wallet-service
      - transaction-service
    networks:
      - gowallet-network

  # ----------------------------------------------------
  # OBSERVABILITY & MONITORING STACK
  # ----------------------------------------------------
  jaeger:
    image: jaegertracing/all-in-one:1.55
    container_name: gowallet-jaeger
    ports:
      - "16686:16686" # Web UI
      - "4317:4317"   # OTLP gRPC
      - "4318:4318"   # OTLP HTTP
    restart: always
    networks:
      - gowallet-network

  prometheus:
    image: prom/prometheus:v2.50.0
    container_name: gowallet-prometheus
    volumes:
      - ./deployments/prometheus/prometheus.yml:/etc/prometheus/prometheus.yml
    ports:
      - "9090:9090"
    restart: always
    networks:
      - gowallet-network

  grafana:
    image: grafana/grafana:10.3.3
    container_name: gowallet-grafana
    ports:
      - "3000:3000"
    restart: always
    networks:
      - gowallet-network

  elasticsearch:
    image: docker.elastic.co/elasticsearch/elasticsearch:8.12.0
    container_name: gowallet-elasticsearch
    environment:
      - discovery.type=single-node
      - xpack.security.enabled=false # Matikan security untuk simplifikasi lokal
      - "ES_JAVA_OPTS=-Xms512m -Xmx512m" # Batasi limit RAM agar tidak crash
    ports:
      - "9200:9200"
    restart: always
    networks:
      - gowallet-network

  logstash:
    image: docker.elastic.co/logstash/logstash:8.12.0
    container_name: gowallet-logstash
    volumes:
      - ./deployments/elk/logstash.conf:/usr/share/logstash/pipeline/logstash.conf
    ports:
      - "5044:5044"
      - "5000:5000/tcp"
    environment:
      - "LS_JAVA_OPTS=-Xms256m -Xmx256m"
    depends_on:
      - elasticsearch
    restart: always
    networks:
      - gowallet-network

  kibana:
    image: docker.elastic.co/kibana/kibana:8.12.0
    container_name: gowallet-kibana
    ports:
      - "5601:5601"
    depends_on:
      - elasticsearch
    restart: always
    networks:
      - gowallet-network

networks:
  gowallet-network:
    driver: bridge

volumes:
  mysql_data:
  mongodb_data:
```

### Step 3: Run & Test Stack
Matikan seluruh proses aplikasi yang sedang berjalan di komputer lokal Anda, lalu jalankan perintah ini dari folder root workspace:

```bash
docker compose up --build
```
Docker akan mendownload base image, mem-build seluruh 10 microservices Go kita secara bertahap, dan menyalakan semuanya beserta seluruh infrastruktur datastore dan stack observability secara otomatis dalam satu kesatuan orkestrasi. 

---

## ✅ Acceptance Criteria
* [ ] Setiap service memiliki file `Dockerfile` valid berbasis multi-stage build.
* [ ] Menjalankan `docker compose up --build` sukses menyalakan 4 database/broker infrastruktur, 6 stack observability (Jaeger, Prometheus, Grafana, Elasticsearch, Logstash, Kibana), dan seluruh 10 microservices Go kita.
* [ ] MySQL menginisialisasi 5 database terpisah (`gowallet_auth`, `gowallet_user`, `gowallet_wallet`, `gowallet_ledger`, `gowallet_transactions`) pada startup awal container.
* [ ] Memanggil endpoint `POST http://localhost:8080/api/v1/auth/login` berhasil mengembalikan JWT Token (menandakan gateway sukses mem-proxy request ke container `auth-service` di dalam Docker bridge network).
* [ ] Memanggil `/api/v1/transactions/transfer` berhasil mengorkestrasi mutasi saldo di `wallet-service` dan menulis ledger di `ledger-service` melalui komunikasi gRPC internal.
* [ ] Seluruh dashboard monitoring (Kibana di `:5601`, Grafana di `:3000`, Prometheus di `:9090`, dan Jaeger di `:16686`) dapat diakses secara lancar.

---

## 💡 Tips untuk Junior
* **Use Non-root User:** Menambahkan instruksi `USER appuser` di Dockerfile sangat penting untuk security hardening. Secara default, Docker menjalankan aplikasi dengan user `root`. Jika ada celah keamanan RCE (*Remote Code Execution*) di aplikasi kita, hacker otomatis menguasai server host kita sebagai root. Dengan user non-root, hak akses peretas sangat dibatasi.
* **Isolasi Database:** Masing-masing microservice HANYA boleh mengakses database miliknya sendiri (misal `user-service` mengakses `gowallet_user`). Mengakses tabel di database service lain secara langsung melanggar prinsip *loose coupling* dan akan merusak integritas microservices Anda di lingkungan production.
* **Resource Optimization:** Karena kita menjalankan total 20 container secara bersamaan di local machine (10 microservices + 4 infra datastore + 6 observability stack), pastikan RAM komputer development Anda memadai (minimal 16GB) dan alokasi resource Docker Desktop diatur dengan baik agar tidak lambat atau hang.

---

## 📚 Referensi Belajar
* [Docker Multi-stage Builds (Official Guide)](https://docs.docker.com/build/building/multi-stage/)
* [Docker Compose Networking tutorial](https://docs.docker.com/compose/networking/)
* [Database-per-service Pattern Guide](https://microservices.io/patterns/data/database-per-service.html)
* [Observability Stack with Docker Compose](https://opentelemetry.io/docs/demo/docker-deployment/)
