# Episode 1: Setup Proyek & Docker Compose

## 🎯 Tujuan
- Menyiapkan monorepo project dengan Go workspace
- Setup Docker Compose dengan semua infrastructure (MySQL, MongoDB, Redis, RabbitMQ, ELK, Prometheus, Grafana, Jaeger)
- Membuat shared packages: logger, errors, config, database, redis, rabbitmq, healthcheck
- Memastikan semua container berjalan dan saling terhubung

## 📝 Prerequisites
- Semua software sudah terinstall (lihat `development_guide.md`)
- Docker Desktop sudah running

---

## 📦 Langkah-langkah

### Step 1: Inisialisasi Project

```bash
# 1. Buat folder project
mkdir gowallet && cd gowallet

# 2. Inisialisasi Git
git init
git checkout -b develop
echo "# GoWallet Microservice" > README.md

# 3. Buat .gitignore
cat > .gitignore << 'EOF'
# Binaries
*.exe
*.exe~
*.dll
*.so
*.dylib
/bin/

# Go
vendor/
*.test

# Environment
.env
.env.dev
.env.staging
.env.prod
!.env.example

# IDE
.idea/
.vscode/
*.swp
*.swo

# Docker
docker-compose.override.yml

# OS
.DS_Store
Thumbs.db

# Logs
*.log

# Coverage
coverage.out
coverage.html
EOF

# 4. Inisialisasi Go workspace
go work init

# 5. Buat struktur folder
mkdir -p pkg/{logger,errors,config,database,redis,rabbitmq,healthcheck}
mkdir -p proto/{user,wallet,ledger,auth}
mkdir -p scripts
mkdir -p deployments/{prometheus,grafana/dashboards,elk,jaeger}
mkdir -p postman
mkdir -p .github/workflows

# 6. Inisialisasi shared package module
cd pkg
go mod init github.com/emzhofb/gowallet/pkg
cd ..

# 7. Tambahkan pkg ke Go workspace
go work use ./pkg
```

### Step 2: Buat Docker Compose

Buat file `docker-compose.yml` di root project:

```yaml
# docker-compose.yml
version: '3.8'

services:
  # ============================================
  # DATABASE
  # ============================================
  mysql:
    image: mysql:8.0
    container_name: gowallet-mysql
    environment:
      MYSQL_ROOT_PASSWORD: rootpassword
      MYSQL_DATABASE: gowallet
      MYSQL_USER: gowallet
      MYSQL_PASSWORD: secret
    ports:
      - "3306:3306"
    volumes:
      - mysql_data:/var/lib/mysql
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost"]
      interval: 10s
      timeout: 5s
      retries: 5
    networks:
      - gowallet-network

  mongodb:
    image: mongo:7
    container_name: gowallet-mongodb
    environment:
      MONGO_INITDB_ROOT_USERNAME: gowallet
      MONGO_INITDB_ROOT_PASSWORD: secret
      MONGO_INITDB_DATABASE: gowallet_audit
    ports:
      - "27017:27017"
    volumes:
      - mongo_data:/data/db
    networks:
      - gowallet-network

  # ============================================
  # CACHE & MESSAGE BROKER
  # ============================================
  redis:
    image: redis:7-alpine
    container_name: gowallet-redis
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5
    networks:
      - gowallet-network

  rabbitmq:
    image: rabbitmq:3-management
    container_name: gowallet-rabbitmq
    environment:
      RABBITMQ_DEFAULT_USER: gowallet
      RABBITMQ_DEFAULT_PASS: secret
    ports:
      - "5672:5672"    # AMQP
      - "15672:15672"  # Management UI
    volumes:
      - rabbitmq_data:/var/lib/rabbitmq
    healthcheck:
      test: ["CMD", "rabbitmqctl", "status"]
      interval: 10s
      timeout: 5s
      retries: 5
    networks:
      - gowallet-network

  # ============================================
  # OBSERVABILITY
  # ============================================
  elasticsearch:
    image: docker.elastic.co/elasticsearch/elasticsearch:8.12.0
    container_name: gowallet-elasticsearch
    environment:
      - discovery.type=single-node
      - xpack.security.enabled=false
      - "ES_JAVA_OPTS=-Xms512m -Xmx512m"
    ports:
      - "9200:9200"
    volumes:
      - elasticsearch_data:/usr/share/elasticsearch/data
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
      - "5000:5000/udp"
    depends_on:
      - elasticsearch
    networks:
      - gowallet-network

  kibana:
    image: docker.elastic.co/kibana/kibana:8.12.0
    container_name: gowallet-kibana
    environment:
      - ELASTICSEARCH_HOSTS=http://elasticsearch:9200
    ports:
      - "5601:5601"
    depends_on:
      - elasticsearch
    networks:
      - gowallet-network

  prometheus:
    image: prom/prometheus:latest
    container_name: gowallet-prometheus
    volumes:
      - ./deployments/prometheus/prometheus.yml:/etc/prometheus/prometheus.yml
    ports:
      - "9090:9090"
    networks:
      - gowallet-network

  grafana:
    image: grafana/grafana:latest
    container_name: gowallet-grafana
    environment:
      - GF_SECURITY_ADMIN_USER=admin
      - GF_SECURITY_ADMIN_PASSWORD=admin
    ports:
      - "3000:3000"
    volumes:
      - grafana_data:/var/lib/grafana
    depends_on:
      - prometheus
    networks:
      - gowallet-network

  jaeger:
    image: jaegertracing/all-in-one:latest
    container_name: gowallet-jaeger
    environment:
      - COLLECTOR_OTLP_ENABLED=true
    ports:
      - "16686:16686"  # Jaeger UI
      - "14268:14268"  # Collector HTTP
      - "4317:4317"    # OTLP gRPC
      - "4318:4318"    # OTLP HTTP
    networks:
      - gowallet-network

volumes:
  mysql_data:
  mongo_data:
  redis_data:
  rabbitmq_data:
  elasticsearch_data:
  grafana_data:

networks:
  gowallet-network:
    driver: bridge
```

### Step 3: Buat Config Files untuk Observability

**`deployments/prometheus/prometheus.yml`:**

```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'api-gateway'
    static_configs:
      - targets: ['api-gateway:8080']
    metrics_path: '/metrics'

  - job_name: 'auth-service'
    static_configs:
      - targets: ['auth-service:8081']
    metrics_path: '/metrics'

  - job_name: 'user-service'
    static_configs:
      - targets: ['user-service:8082']
    metrics_path: '/metrics'

  - job_name: 'wallet-service'
    static_configs:
      - targets: ['wallet-service:8083']
    metrics_path: '/metrics'

  - job_name: 'ledger-service'
    static_configs:
      - targets: ['ledger-service:8084']
    metrics_path: '/metrics'

  - job_name: 'transaction-service'
    static_configs:
      - targets: ['transaction-service:8085']
    metrics_path: '/metrics'

  - job_name: 'payment-service'
    static_configs:
      - targets: ['payment-service:8086']
    metrics_path: '/metrics'
```

**`deployments/elk/logstash.conf`:**

```
input {
  tcp {
    port => 5000
    codec => json_lines
  }
  udp {
    port => 5000
    codec => json_lines
  }
}

filter {
  if [service_name] {
    mutate {
      add_field => { "[@metadata][index_prefix]" => "gowallet-%{service_name}" }
    }
  } else {
    mutate {
      add_field => { "[@metadata][index_prefix]" => "gowallet-unknown" }
    }
  }
}

output {
  elasticsearch {
    hosts => ["http://elasticsearch:9200"]
    index => "%{[@metadata][index_prefix]}-%{+YYYY.MM.dd}"
  }
  stdout {
    codec => rubydebug
  }
}
```

### Step 4: Buat Environment File

**`.env.example`:**

```env
# ============================================
# APPLICATION
# ============================================
APP_ENV=development
APP_NAME=gowallet

# ============================================
# API GATEWAY
# ============================================
GATEWAY_PORT=8080

# ============================================
# SERVICE PORTS
# ============================================
AUTH_SERVICE_PORT=8081
AUTH_SERVICE_GRPC_PORT=9081
USER_SERVICE_PORT=8082
USER_SERVICE_GRPC_PORT=9082
WALLET_SERVICE_PORT=8083
WALLET_SERVICE_GRPC_PORT=9083
LEDGER_SERVICE_PORT=8084
LEDGER_SERVICE_GRPC_PORT=9084
TRANSACTION_SERVICE_PORT=8085
TRANSACTION_SERVICE_GRPC_PORT=9085
PAYMENT_SERVICE_PORT=8086
NOTIFICATION_SERVICE_PORT=8087
AUDIT_SERVICE_PORT=8088
SCHEDULER_SERVICE_PORT=8089

# ============================================
# MYSQL
# ============================================
MYSQL_HOST=localhost
MYSQL_PORT=3306
MYSQL_USER=gowallet
MYSQL_PASSWORD=secret
MYSQL_ROOT_PASSWORD=rootpassword
MYSQL_DATABASE=gowallet
MYSQL_MAX_OPEN_CONNS=25
MYSQL_MAX_IDLE_CONNS=5
MYSQL_CONN_MAX_LIFETIME=5m

# ============================================
# MONGODB
# ============================================
MONGO_URI=mongodb://gowallet:secret@localhost:27017
MONGO_DATABASE=gowallet_audit

# ============================================
# REDIS
# ============================================
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0

# ============================================
# RABBITMQ
# ============================================
RABBITMQ_URL=amqp://gowallet:secret@localhost:5672/
RABBITMQ_EXCHANGE=gowallet.events
RABBITMQ_DLX_EXCHANGE=gowallet.dlx

# ============================================
# JWT
# ============================================
JWT_SECRET=your-super-secret-key-change-in-production
JWT_ACCESS_EXPIRY=15m
JWT_REFRESH_EXPIRY=168h

# ============================================
# OBSERVABILITY
# ============================================
ELASTICSEARCH_URL=http://localhost:9200
LOGSTASH_URL=localhost:5000
JAEGER_ENDPOINT=http://localhost:14268/api/traces
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317

# ============================================
# EMAIL (Development: MailHog / Mailtrap)
# ============================================
SMTP_HOST=localhost
SMTP_PORT=1025
SMTP_USER=
SMTP_PASSWORD=
SMTP_FROM=noreply@gowallet.com

# ============================================
# GOOGLE OAUTH
# ============================================
GOOGLE_CLIENT_ID=your-google-client-id
GOOGLE_CLIENT_SECRET=your-google-client-secret
GOOGLE_REDIRECT_URL=http://localhost:8080/api/v1/auth/google/callback

# ============================================
# RATE LIMITING
# ============================================
RATE_LIMIT_ANONYMOUS=30
RATE_LIMIT_AUTHENTICATED=200
RATE_LIMIT_ADMIN=1000
RATE_LIMIT_WINDOW=1m
```

Lalu copy untuk development:
```bash
cp .env.example .env.dev
```

### Step 5: Buat Shared Package — Logger (`pkg/logger`)

Buat `pkg/logger/logger.go`:

Fitur yang harus ada:
- **Inisialisasi** — `NewLogger(env string) *zap.Logger` (development = console, production = JSON)
- **With Correlation ID** — `WithCorrelationID(ctx context.Context) *zap.Logger`
- **Structured fields** — service_name, correlation_id, level, timestamp, message
- **Log levels** — Debug, Info, Warn, Error, Fatal

Contoh penggunaan:
```go
logger := logger.NewLogger("development")
logger.Info("server started", 
    zap.String("port", "8080"),
    zap.String("service", "api-gateway"),
)

// Dengan correlation ID dari context
log := logger.WithCorrelationID(ctx)
log.Info("processing request", zap.String("method", "POST"))
```

Contoh output (JSON):
```json
{
  "level": "info",
  "ts": "2024-01-15T10:30:00Z",
  "msg": "server started",
  "service": "api-gateway",
  "port": "8080",
  "correlation_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

**Library:** `go.uber.org/zap`

### Step 6: Buat Shared Package — Errors (`pkg/errors`)

Buat `pkg/errors/errors.go`:

Fitur yang harus ada:
- **Custom error type** — `AppError` dengan Code, Message, HTTPStatus
- **Predefined errors** — `ErrNotFound`, `ErrUnauthorized`, `ErrForbidden`, `ErrBadRequest`, `ErrInternalServer`, `ErrInsufficientBalance`, `ErrDuplicateEntry`
- **Error response formatter** — format error ke JSON response yang konsisten
- **Error wrapping** — wrap error standar Go ke AppError

Format response error:
```json
{
  "success": false,
  "error": {
    "code": "INSUFFICIENT_BALANCE",
    "message": "Saldo tidak mencukupi untuk melakukan transfer"
  }
}
```

Format response sukses:
```json
{
  "success": true,
  "data": { ... },
  "meta": {
    "page": 1,
    "limit": 10,
    "total": 100,
    "total_pages": 10
  }
}
```

### Step 7: Buat Shared Package — Config (`pkg/config`)

Buat `pkg/config/config.go`:

Fitur yang harus ada:
- **Load dari .env** — menggunakan `github.com/spf13/viper` atau `github.com/joho/godotenv`
- **Multi-environment** — support `development`, `staging`, `production`
- **Type-safe** — struct untuk setiap config section
- **Validation** — pastikan required config tidak kosong

Contoh struct:
```go
type Config struct {
    App      AppConfig
    MySQL    MySQLConfig
    MongoDB  MongoConfig
    Redis    RedisConfig
    RabbitMQ RabbitMQConfig
    JWT      JWTConfig
}

type MySQLConfig struct {
    Host            string
    Port            int
    User            string
    Password        string
    Database        string
    MaxOpenConns    int
    MaxIdleConns    int
    ConnMaxLifetime time.Duration
}
```

### Step 8: Buat Shared Package — Database (`pkg/database`)

Buat `pkg/database/mysql.go`:

Fitur yang harus ada:
- **NewMySQLConnection** — buat connection pool `*sql.DB`
- **Connection retry** — coba ulang connect jika gagal (max 5x, backoff 2s)
- **Health check** — `Ping()` method
- **Graceful close** — `Close()` method
- **Connection pool config** — MaxOpenConns, MaxIdleConns, ConnMaxLifetime

```go
func NewMySQLConnection(cfg MySQLConfig) (*sql.DB, error) {
    dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&loc=Local",
        cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database,
    )
    
    db, err := sql.Open("mysql", dsn)
    if err != nil {
        return nil, err
    }
    
    db.SetMaxOpenConns(cfg.MaxOpenConns)
    db.SetMaxIdleConns(cfg.MaxIdleConns)
    db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
    
    // Retry ping
    for i := 0; i < 5; i++ {
        if err := db.Ping(); err == nil {
            return db, nil
        }
        time.Sleep(2 * time.Second)
    }
    
    return nil, fmt.Errorf("failed to connect to MySQL after 5 retries")
}
```

### Step 9: Buat Shared Package — Redis (`pkg/redis`)

Buat `pkg/redis/redis.go`:

Fitur yang harus ada:
- **NewRedisClient** — buat client `*redis.Client`
- **Connection retry** — sama seperti MySQL
- **Health check** — `Ping()` method
- **Common operations wrapper** — Get, Set, Del, Exists, SetWithTTL

**Library:** `github.com/redis/go-redis/v9`

### Step 10: Buat Shared Package — RabbitMQ (`pkg/rabbitmq`)

Buat `pkg/rabbitmq/rabbitmq.go`:

Fitur yang harus ada:
- **NewConnection** — buat connection ke RabbitMQ
- **Publisher** — publish message ke exchange dengan routing key
- **Consumer** — consume message dari queue dengan handler function
- **Reconnect** — auto-reconnect jika connection terputus
- **Graceful close**

**Library:** `github.com/rabbitmq/amqp091-go`

### Step 11: Buat Shared Package — Health Check (`pkg/healthcheck`)

Buat `pkg/healthcheck/healthcheck.go`:

3 endpoint yang harus ada:

| Endpoint | Kegunaan | Cek apa |
|---|---|---|
| `GET /health` | Basic health | Service running |
| `GET /ready` | Readiness check | MySQL, Redis, RabbitMQ connected |
| `GET /live` | Liveness probe | Service responsive |

Response format:
```json
// /health
{ "status": "ok", "service": "api-gateway", "timestamp": "2024-01-15T10:30:00Z" }

// /ready
{
  "status": "ready",
  "checks": {
    "mysql": { "status": "up", "latency": "2ms" },
    "redis": { "status": "up", "latency": "1ms" },
    "rabbitmq": { "status": "up", "latency": "3ms" }
  }
}

// /live
{ "status": "alive" }
```

### Step 12: Buat Makefile

```makefile
.PHONY: help docker-up docker-down docker-logs docker-ps

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ============================================
# DOCKER
# ============================================
docker-up: ## Start all containers
	docker compose up -d

docker-down: ## Stop all containers
	docker compose down

docker-logs: ## Show container logs (usage: make docker-logs s=mysql)
	docker compose logs -f $(s)

docker-ps: ## Show running containers
	docker compose ps

docker-reset: ## Reset all containers and volumes
	docker compose down -v
	docker compose up -d

# ============================================
# DATABASE MIGRATION
# ============================================
migrate-up: ## Run migrations up (usage: make migrate-up s=auth)
	migrate -path ./$(s)-service/db/migrations -database "mysql://gowallet:secret@tcp(localhost:3306)/gowallet" up

migrate-down: ## Run migrations down (usage: make migrate-down s=auth)
	migrate -path ./$(s)-service/db/migrations -database "mysql://gowallet:secret@tcp(localhost:3306)/gowallet" down 1

migrate-create: ## Create new migration (usage: make migrate-create s=auth n=create_users)
	migrate create -ext sql -dir ./$(s)-service/db/migrations -seq $(n)

# ============================================
# PROTO
# ============================================
proto-gen: ## Generate protobuf Go files
	./scripts/generate-proto.sh

# ============================================
# TEST
# ============================================
test: ## Run all tests
	go test ./... -v -cover

test-coverage: ## Run tests with coverage report
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# ============================================
# LINT
# ============================================
lint: ## Run linter
	golangci-lint run ./...

# ============================================
# BUILD
# ============================================
build: ## Build all services
	@for dir in */cmd/main.go; do \
		service=$$(echo $$dir | cut -d'/' -f1); \
		echo "Building $$service..."; \
		cd $$service && go build -o ../bin/$$service ./cmd/main.go && cd ..; \
	done
```

### Step 13: Verifikasi

```bash
# 1. Start semua container
make docker-up
# atau: docker compose up -d

# 2. Cek semua container running
make docker-ps
# atau: docker compose ps

# 3. Verifikasi setiap service:

# MySQL
docker exec -it gowallet-mysql mysql -ugowallet -psecret -e "SELECT 1"

# Redis
docker exec -it gowallet-redis redis-cli ping
# Expected: PONG

# RabbitMQ Management UI
# Buka browser: http://localhost:15672
# Login: gowallet / secret

# Kibana
# Buka browser: http://localhost:5601

# Grafana
# Buka browser: http://localhost:3000
# Login: admin / admin

# Prometheus
# Buka browser: http://localhost:9090

# Jaeger UI
# Buka browser: http://localhost:16686

# MongoDB
docker exec -it gowallet-mongodb mongosh -u gowallet -p secret --authenticationDatabase admin

# 4. Cek shared packages compile
cd pkg && go build ./... && cd ..
```

### Step 14: Unit Test untuk Shared Packages

Tulis minimal test untuk setiap package:

- `pkg/logger/logger_test.go` — test NewLogger, output format
- `pkg/errors/errors_test.go` — test AppError, predefined errors, response format
- `pkg/config/config_test.go` — test Load config dari env
- `pkg/healthcheck/healthcheck_test.go` — test response format

```bash
cd pkg && go test ./... -v
```

### Step 15: Commit

```bash
git add .
git commit -m "feat(ep01): setup project, docker-compose, and shared packages"
```

---

## ✅ Acceptance Criteria

- [ ] `docker compose up -d` berhasil menjalankan semua container
- [ ] Bisa connect ke MySQL via `mysql` client
- [ ] Bisa akses RabbitMQ Management UI di `http://localhost:15672`
- [ ] Bisa akses Kibana di `http://localhost:5601`
- [ ] Bisa akses Grafana di `http://localhost:3000`
- [ ] Bisa akses Prometheus di `http://localhost:9090`
- [ ] Bisa akses Jaeger UI di `http://localhost:16686`
- [ ] `pkg/` Go packages compile tanpa error (`go build ./...`)
- [ ] Shared logger menghasilkan output JSON terstruktur
- [ ] Unit test untuk shared packages pass
- [ ] `.env.example` lengkap dengan semua config
- [ ] `Makefile` berisi shortcuts yang berguna

---

## 💡 Tips & Common Pitfalls

1. **ELK terlalu berat?** — Jika laptop kamu punya RAM < 8GB, comment dulu `elasticsearch`, `logstash`, `kibana` di docker-compose.yml. Kamu bisa pakai langsung di Episode 18.

2. **Port conflict?** — Jika port sudah dipakai, ubah di docker-compose.yml. Contoh: `"3307:3306"` untuk MySQL.

3. **Container restart terus?** — Cek logs: `docker compose logs mysql` atau `docker compose logs elasticsearch`

4. **Go workspace error?** — Pastikan `go.work` sudah ada dan `go work use ./pkg` sudah dijalankan.

5. **MySQL connection refused saat pertama kali?** — MySQL butuh waktu ~30 detik untuk start. Itulah kenapa kita buat connection retry di `pkg/database`.

---

## 📚 Referensi Belajar

- [Docker Compose Documentation](https://docs.docker.com/compose/)
- [Go Workspace Mode](https://go.dev/doc/tutorial/workspaces)
- [Zap Logger](https://pkg.go.dev/go.uber.org/zap)
- [Viper Config](https://github.com/spf13/viper)
- [Go Project Layout](https://github.com/golang-standards/project-layout)
