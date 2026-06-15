# 🏗️ GoWallet Microservice — Development Guide

> **Target Audience:** Junior Software Developer/Engineer & Free AI Coding Assistants  
> **Tech Stack:** Go (Golang), Gin, Raw SQL, Zap Logger, MySQL, MongoDB, Redis, RabbitMQ, Docker, gRPC, REST  
> **Repository:** Monorepo (`github.com/emzhofb/gowallet`)  
> **Total Episodes:** 20 | **Estimated Duration:** ~60-90 hari (part-time)

---

## 📋 Persiapan Environment

### Software yang Harus Diinstall

| Software | Versi Minimum | Kegunaan | Install |
|---|---|---|---|
| **Go** | 1.22+ | Bahasa pemrograman utama | [golang.org/dl](https://golang.org/dl/) |
| **Docker Desktop** | 24+ | Container runtime | [docker.com](https://www.docker.com/products/docker-desktop/) |
| **Docker Compose** | v2+ | Orchestrasi container | Included in Docker Desktop |
| **Git** | 2.40+ | Version control | [git-scm.com](https://git-scm.com/) |
| **VS Code** | Latest | Code editor | [code.visualstudio.com](https://code.visualstudio.com/) |
| **Postman** | Latest | API testing | [postman.com](https://www.postman.com/downloads/) |
| **protoc** | 3.x | Protocol Buffer compiler | [grpc.io](https://grpc.io/docs/protoc-installation/) |
| **golang-migrate** | v4 | Database migration tool | `go install -tags 'mysql' github.com/golang-migrate/migrate/v4/cmd/migrate@latest` |
| **golangci-lint** | Latest | Go linter | `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest` |
| **swag** | Latest | Swagger doc generator | `go install github.com/swaggo/swag/cmd/swag@latest` |

### VS Code Extensions (Rekomendasi)

```
golang.go                    → Go language support
ms-azuretools.vscode-docker  → Docker support
zxh404.vscode-proto3         → Protobuf syntax highlighting
humao.rest-client            → REST API testing di editor
```

### Verifikasi Instalasi

```bash
go version          # go version go1.22+ ...
docker --version    # Docker version 24+ ...
docker compose version  # Docker Compose version v2+ ...
git --version       # git version 2.40+ ...
protoc --version    # libprotoc 3.x+ ...
migrate --version   # v4.x.x
```

---

## 🗂️ Struktur Folder Project (Target Akhir)

```
gowallet/
├── docker-compose.yml              ← Orchestrasi semua container
├── docker-compose.dev.yml          ← Override untuk development
├── .env.example                    ← Template environment variables
├── .env.dev                        ← Config development (jangan commit!)
├── Makefile                        ← Shortcuts untuk command
├── go.work                         ← Go workspace file
├── README.md
│
├── api-gateway/                    ← Single entry point
│   ├── cmd/main.go
│   ├── internal/
│   │   ├── middleware/             ← JWT, CORS, Rate Limiter, Logging
│   │   ├── proxy/                  ← Reverse proxy ke backend services
│   │   └── config/
│   ├── Dockerfile
│   └── go.mod
│
├── auth-service/                   ← Authentication & Authorization
│   ├── cmd/main.go
│   ├── internal/
│   │   ├── handler/                ← HTTP handlers (controller layer)
│   │   ├── service/                ← Business logic layer
│   │   ├── repository/             ← Data access layer
│   │   ├── model/                  ← Domain entities (struct)
│   │   ├── dto/                    ← Request/Response DTOs
│   │   └── config/
│   ├── db/migrations/              ← SQL migration files
│   ├── docs/                       ← Swagger output
│   ├── Dockerfile
│   └── go.mod
│
├── user-service/                   ← Serupa dengan auth-service
├── wallet-service/
├── ledger-service/
├── transaction-service/
├── payment-service/
├── notification-service/
├── audit-service/
├── scheduler-service/
│
├── proto/                          ← Shared .proto files untuk gRPC
│   ├── user/user.proto
│   ├── wallet/wallet.proto
│   ├── ledger/ledger.proto
│   └── auth/auth.proto
│
├── pkg/                            ← Shared Go packages
│   ├── logger/                     ← Structured JSON logging (Zap)
│   ├── errors/                     ← Custom error types & HTTP error response
│   ├── config/                     ← Environment config loader
│   ├── database/                   ← MySQL connection helper
│   ├── rabbitmq/                   ← RabbitMQ publisher & consumer
│   ├── redis/                      ← Redis client wrapper
│   ├── tracing/                    ← OpenTelemetry setup
│   └── healthcheck/                ← /health, /ready, /live handlers
│
├── scripts/                        ← Helper scripts
│   ├── migrate.sh
│   ├── seed.sh
│   └── generate-proto.sh
│
├── deployments/                    ← Infrastructure configs
│   ├── prometheus/prometheus.yml
│   ├── grafana/dashboards/
│   ├── elk/logstash.conf
│   └── jaeger/
│
├── postman/
│   └── GoWallet.postman_collection.json
│
└── .github/workflows/              ← CI/CD pipelines
    ├── lint.yml
    ├── test.yml
    └── deploy.yml
```

---

## 📐 Pola Arsitektur Tiap Service (Clean Architecture)

Setiap service mengikuti pattern yang sama agar konsisten:

```
service-name/
├── cmd/
│   └── main.go                  ← Entry point, wire dependencies
├── internal/
│   ├── config/
│   │   └── config.go            ← Load env vars
│   ├── model/
│   │   └── entity.go            ← Domain entities (struct)
│   ├── dto/
│   │   ├── request.go           ← Request DTOs
│   │   └── response.go          ← Response DTOs
│   ├── repository/
│   │   ├── interface.go         ← Repository interface
│   │   └── mysql_impl.go        ← MySQL implementation
│   ├── service/
│   │   ├── interface.go         ← Service/usecase interface
│   │   └── service_impl.go      ← Business logic
│   ├── handler/
│   │   └── http_handler.go      ← HTTP handlers (controller)
│   ├── grpc/
│   │   └── server.go            ← gRPC server (jika ada)
│   └── middleware/
│       └── auth.go              ← Service-specific middleware
├── db/
│   └── migrations/
│       ├── 000001_create_table.up.sql
│       └── 000001_create_table.down.sql
├── Dockerfile
├── go.mod
└── go.sum
```

### Penjelasan Layer (Untuk yang Baru Belajar)

```
┌─────────────────────────────────┐
│           HTTP Request          │
└───────────────┬─────────────────┘
                ▼
┌─────────────────────────────────┐
│         Handler Layer           │  ← Terima request, validasi input,
│    (http_handler.go)            │     format response. TIDAK ada business logic.
└───────────────┬─────────────────┘
                ▼
┌─────────────────────────────────┐
│         Service Layer           │  ← Business logic murni.
│    (service_impl.go)            │     Tidak tahu soal HTTP/gRPC.
└───────────────┬─────────────────┘
                ▼
┌─────────────────────────────────┐
│       Repository Layer          │  ← Akses database.
│    (mysql_impl.go)              │     Tidak tahu soal business logic.
└───────────────┬─────────────────┘
                ▼
┌─────────────────────────────────┐
│          Database               │
│    (MySQL / MongoDB / Redis)    │
└─────────────────────────────────┘
```

**Aturan penting:**
- Dependency mengalir ke **bawah** saja: Handler → Service → Repository
- Handler **tidak boleh** langsung panggil Repository
- Service **tidak boleh** import package `net/http` atau tahu soal HTTP
- Repository **tidak boleh** tahu soal business rules

---

## 📦 Go Library Cheatsheet

| Kegunaan | Library | Install |
|---|---|---|
| HTTP Router | `github.com/gin-gonic/gin` | `go get github.com/gin-gonic/gin` |
| MySQL Driver | `github.com/go-sql-driver/mysql` | `go get github.com/go-sql-driver/mysql` |
| MongoDB | `go.mongodb.org/mongo-driver` | `go get go.mongodb.org/mongo-driver/mongo` |
| Redis | `github.com/redis/go-redis/v9` | `go get github.com/redis/go-redis/v9` |
| RabbitMQ | `github.com/rabbitmq/amqp091-go` | `go get github.com/rabbitmq/amqp091-go` |
| JWT | `github.com/golang-jwt/jwt/v5` | `go get github.com/golang-jwt/jwt/v5` |
| bcrypt | `golang.org/x/crypto/bcrypt` | `go get golang.org/x/crypto` |
| UUID | `github.com/google/uuid` | `go get github.com/google/uuid` |
| Validator | `github.com/go-playground/validator/v10` | `go get github.com/go-playground/validator/v10` |
| gRPC | `google.golang.org/grpc` | `go get google.golang.org/grpc` |
| Protobuf | `google.golang.org/protobuf` | `go get google.golang.org/protobuf` |
| Swagger | `github.com/swaggo/swag` | `go get github.com/swaggo/gin-swagger` |
| Logger | `go.uber.org/zap` | `go get go.uber.org/zap` |
| Config | `github.com/spf13/viper` | `go get github.com/spf13/viper` |
| Env | `github.com/joho/godotenv` | `go get github.com/joho/godotenv` |
| Migration | `github.com/golang-migrate/migrate/v4` | CLI tool (lihat atas) |
| Cron | `github.com/robfig/cron/v3` | `go get github.com/robfig/cron/v3` |
| Circuit Breaker | `github.com/sony/gobreaker` | `go get github.com/sony/gobreaker` |
| OpenTelemetry | `go.opentelemetry.io/otel` | `go get go.opentelemetry.io/otel` |
| OAuth2 | `golang.org/x/oauth2` | `go get golang.org/x/oauth2` |
| Testing | `github.com/stretchr/testify` | `go get github.com/stretchr/testify` |
| HTTP Mock | `github.com/jarcoal/httpmock` | `go get github.com/jarcoal/httpmock` |

---

## 🗺️ Dependency Map Antar Service

```
                        ┌──────────────┐
                        │    Client    │
                        └──────┬───────┘
                               │
                        ┌──────▼───────┐
                        │  API Gateway │ ◄── Rate Limiter (Redis)
                        └──┬──┬──┬──┬──┘     JWT Validation
                           │  │  │  │        CORS, Logging
          ┌────────────────┘  │  │  └────────────────┐
          ▼                   ▼  ▼                   ▼
   ┌────────────┐   ┌──────────┐ ┌──────────────┐ ┌────────────┐
   │Auth Service│   │User Svc  │ │Wallet Service│ │Transaction │
   │            │   │          │ │              │ │Service     │
   └─────┬──────┘   └────┬─────┘ └──────┬───────┘ └──────┬─────┘
         │                │              │                │
         │           ┌────┘          ┌───┘                │
         │           │ gRPC          │ gRPC               │
         │           ▼               ▼                    │
         │      ┌─────────┐   ┌───────────┐              │
         │      │User gRPC│   │Ledger Svc │◄─────────────┘
         │      └─────────┘   └───────────┘         gRPC
         │
         ▼
   ┌──────────────────────────────────────────┐
   │              RabbitMQ                     │
   │  Events: transfer.completed,             │
   │          payment.completed,              │
   │          user.registered, etc.           │
   └──┬───────────┬───────────┬───────────────┘
      │           │           │
      ▼           ▼           ▼
┌──────────┐ ┌──────────┐ ┌──────────┐
│Notif Svc │ │Audit Svc │ │Scheduler │
│(Email)   │ │(MongoDB) │ │(Cron)    │
└──────────┘ └──────────┘ └──────────┘
```

---

## 📅 Episode Overview & Estimasi Waktu

| # | Episode | Durasi | Difficulty | File Panduan |
|---|---|---|---|---|
| 1 | Setup Proyek & Docker Compose | 3-5 hari | ⭐⭐ | `episodes/ep01-setup.md` |
| 2 | API Gateway | 3-5 hari | ⭐⭐⭐ | `episodes/ep02-gateway.md` |
| 3 | Auth Service — JWT | 3-5 hari | ⭐⭐⭐ | `episodes/ep03-auth-jwt.md` |
| 4 | Refresh Token & Google OAuth | 2-3 hari | ⭐⭐⭐ | `episodes/ep04-auth-oauth.md` |
| 5 | Email Verification & Reset Password | 2-3 hari | ⭐⭐ | `episodes/ep05-email-verify.md` |
| 6 | User Service | 3-4 hari | ⭐⭐ | `episodes/ep06-user.md` |
| 7 | Wallet Service | 4-5 hari | ⭐⭐⭐⭐ | `episodes/ep07-wallet.md` |
| 8 | Ledger Service | 4-5 hari | ⭐⭐⭐⭐⭐ | `episodes/ep08-ledger.md` |
| 9 | Transfer & Transaction Service | 5-7 hari | ⭐⭐⭐⭐⭐ | `episodes/ep09-transaction.md` |
| 10 | Payment Service | 3-4 hari | ⭐⭐⭐ | `episodes/ep10-payment.md` |
| 11 | RabbitMQ & Event-Driven | 3-4 hari | ⭐⭐⭐⭐ | `episodes/ep11-rabbitmq.md` |
| 12 | Notification Service | 2-3 hari | ⭐⭐ | `episodes/ep12-notification.md` |
| 13 | Audit Service | 2-3 hari | ⭐⭐ | `episodes/ep13-audit.md` |
| 14 | Scheduler/Worker Service | 2-3 hari | ⭐⭐⭐ | `episodes/ep14-scheduler.md` |
| 15 | Redis Enhancement | 2-3 hari | ⭐⭐⭐ | `episodes/ep15-redis.md` |
| 16 | Reliability Patterns | 3-4 hari | ⭐⭐⭐⭐ | `episodes/ep16-reliability.md` |
| 17 | OpenTelemetry & Jaeger | 3-4 hari | ⭐⭐⭐⭐ | `episodes/ep17-tracing.md` |
| 18 | ELK + Prometheus + Grafana | 3-5 hari | ⭐⭐⭐ | `episodes/ep18-observability.md` |
| 19 | CI/CD & GitHub Actions | 3-4 hari | ⭐⭐⭐ | `episodes/ep19-cicd.md` |
| 20 | Hardening & Final Integration | 5-7 hari | ⭐⭐⭐⭐ | `episodes/ep20-hardening.md` |

**Total estimasi: ~60-90 hari** (part-time, 2-4 jam/hari)

---

## ⚙️ Cara Menggunakan Guide Ini

### Untuk Junior Developer
1. **Kerjakan per episode** — jangan loncat-loncat, urutan penting!
2. **Buat branch per episode** — `feature/ep01-setup`, `feature/ep02-gateway`, dst
3. **Tulis unit test** di setiap episode, jangan ditumpuk di akhir
4. **Commit sering** — setiap fitur kecil selesai, commit
5. **Baca error message baik-baik** — 90% jawaban ada di error message
6. **Gunakan `docker compose logs -f <service>`** untuk debug container

### Untuk AI Coding Assistant (Free Tier)
1. **Kerjakan satu episode per sesi** — karena context window terbatas
2. **Selalu referensi file guide ini** untuk konsistensi
3. **Generate code per layer** — model → repository → service → handler
4. **Jangan skip unit test** — tulis bersamaan dengan implementation
5. **Pastikan code compilable** sebelum pindah ke komponen berikutnya

### Git Workflow

```bash
# Mulai episode baru
git checkout develop
git pull origin develop
git checkout -b feature/ep01-setup

# Selama development, commit sering
git add .
git commit -m "feat(ep01): add docker-compose with MySQL and Redis"

# Selesai episode
git push origin feature/ep01-setup

# Buat Pull Request ke develop
# Setelah review/merge, lanjut episode berikutnya
```

### Konvensi Commit Message

```
feat(epXX): deskripsi fitur baru
fix(epXX): deskripsi bug fix
refactor(epXX): perubahan kode tanpa ubah behavior
docs(epXX): update dokumentasi
test(epXX): tambah/update test
chore(epXX): maintenance task
```

---

## 📎 File Panduan Episode

Buka folder `episodes/` untuk panduan detail per episode. Setiap file berisi:
- 🎯 Tujuan episode
- 📝 Prerequisites (episode sebelumnya yang harus selesai)
- 📦 Langkah-langkah detail
- 💾 Database schema (jika ada)
- 🔌 API endpoints (jika ada)
- ✅ Acceptance criteria (checklist)
- 💡 Tips & common pitfalls
- 📚 Referensi belajar
