# 🏗️ Monolith to Microservices — Development Guide

> **Target Audience:** Junior Software Engineers & Free AI coding assistants.  
> **Repository Structure:** Starts as a single application, evolves into a Go Workspace (`go.work`) monorepo.  
> **Key Architecture Pattern:** Clean Architecture (Handler ➔ Service ➔ Repository).  
> **Difficulty Profile:** Slowly transitions from basic web CRUD to advanced distributed event-driven systems.

---

## 🗺️ Peta Kurikulum (6 Fase, 45 Episode)

Panduan ini dirancang untuk membimbing junior developer secara bertahap tanpa membuat mereka pusing dengan terlalu banyak teknologi di awal.

```
       [ Fase 1: Monolith Dasar ] (Eps 1-12)
                 │  (Go + MySQL + Clean Arch + Swagger)
                 ▼
     [ Fase 2: Cache & Background ] (Eps 13-19)
                 │  (Redis + OAuth2 + Token Rotation + RBAC)
                 ▼
    [ Fase 3: Pemecahan Microservices ] (Eps 20-29)
                 │  (API Gateway + gRPC + Complete Service Decomposition + Scheduler)
                 ▼
       [ Fase 4: Event-Driven ] (Eps 30-35)
                 │  (RabbitMQ + MongoDB Audit + Payment Service)
                 ▼
     [ Fase 5: Resiliency, Config & Testing ] (Eps 36-43)
                 │  (Secrets + Postman + Integration Testing + Tracing + ELK)
                 ▼
        [ Fase 6: DevOps & CI/CD ] (Eps 44-45)
                    (Docker Compose + GitHub Actions)
```

---

## 📅 Daftar Episode

### Fase 1: The Monolith (Core Domain)
Fase ini fokus pada penguasaan dasar Go backend, relasi database, clean architecture, transaksi SQL, dan penulisan unit test.
* [Episode 1: Setup Go & Database Connection](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-1-monolith/ep01-setup-db.md)
* [Episode 2: Database Migration & Schema Design](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-1-monolith/ep02-migration-schema.md)
* [Episode 3: Clean Architecture — CRUD User](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-1-monolith/ep03-clean-arch.md)
* [Episode 4: Structured Logging & Centralized Error Handling](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-1-monolith/ep04-logging-errors.md)
* [Episode 5: Authentication dengan JWT & Bcrypt](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-1-monolith/ep05-auth-jwt.md)
* [Episode 6: Wallet Management & Balance Check](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-1-monolith/ep06-wallet-creation.md)
* [Episode 7: Double-Entry Ledger System (Introduction)](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-1-monolith/ep07-ledger-system.md)
* [Episode 8: Database Transactions & Optimistic Locking](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-1-monolith/ep08-db-tx-locking.md)
* [Episode10: Pagination, Sorting, & Soft Delete](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-1-monolith/ep11-pagination-softdelete.md)
* [Episode11: Unit Testing & Mocking](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-1-monolith/ep12-unit-testing.md)
* [Episode12: API Documentation dengan Swagger/OpenAPI](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-1-monolith/ep13-swagger-openapi.md)

### Fase 2: Caching & Background Processing
Fase ini mengenalkan optimasi performa, pemrosesan latar belakang (background processing), dan security hardening pada autentikasi.
* [Episode13: Redis Caching](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-2-enhancing-monolith/ep14-redis-caching.md)
* [Episode14: Rate Limiting & JWT Blacklisting](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-2-enhancing-monolith/ep15-rate-limit-blacklist.md)
* [Episode15: Background Schedulers (Cron)](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-2-enhancing-monolith/ep16-background-scheduler.md)
* [Episode16: Email Verification & Forgot Password](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-2-enhancing-monolith/ep17-email-verification.md)
* [Episode17: Integrasi Google OAuth](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-2-enhancing-monolith/ep18-google-oauth.md)
* [Episode18: Refresh Token Rotation & Token Reuse Detection](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-2-enhancing-monolith/ep19-refresh-token-rotation.md)
* [Episode19: Role-Based Access Control (RBAC)](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-2-enhancing-monolith/ep20-rbac.md)

### Fase 3: Pemecahan Microservices
Fase ini mengajarkan bagaimana memecah monolith menjadi layanan-layanan independen yang berkomunikasi lewat jaringan gRPC secara terdekomposisi penuh.
* [Episode20: Monorepo & API Gateway (Reverse Proxy)](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-3-splitting-microservices/ep21-monorepo-gateway.md)
* [Episode21: Memecah Auth Service](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-3-splitting-microservices/ep22-splitting-auth.md)
* [Episode22: Memecah User Service](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-3-splitting-microservices/ep23-splitting-user.md)
* [Episode23: Komunikasi Internal gRPC](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-3-splitting-microservices/ep24-grpc-communication.md)
* [Episode24: Memecah Wallet Service](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-3-splitting-microservices/ep25-splitting-wallet.md)
* [Episode25: Memecah Ledger Service](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-3-splitting-microservices/ep26-splitting-ledger.md)
* [Episode26: Memecah Transaction Service](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-3-splitting-microservices/ep27-splitting-transaction.md)
* [Episode27: gRPC Communication for Transfer Flow](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-3-splitting-microservices/ep28-grpc-transfer-flow.md)
* [Episode28: Memecah Scheduler Service](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-3-splitting-microservices/ep29-splitting-scheduler.md)
* [Episode29: Outbox Pattern untuk Transaksi Terdistribusi](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-3-splitting-microservices/ep30-outbox-pattern.md)

### Fase 4: Event-Driven Communication
Fase ini mengenalkan komunikasi asinkronus menggunakan message broker untuk skalabilitas yang lebih tinggi.
* [Episode30: RabbitMQ & Event Publishing](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-4-event-driven/ep31-rabbitmq-publisher.md)
* [Episode31: Payment Service (External Integration & Webhook HMAC)](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-4-event-driven/ep32-payment-service.md)
* [Episode32: Consuming Payment Events in Wallet Service](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-4-event-driven/ep33-consuming-payment-events.md)
* [Episode33: Notification Service (Consumer)](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-4-event-driven/ep34-notification-consumer.md)
* [Episode34: Audit Logging dengan MongoDB](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-4-event-driven/ep35-audit-mongodb.md)
* [Episode35: RabbitMQ Resiliency (DLQ & Retries)](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-4-event-driven/ep36-rabbitmq-dlq-retries.md)

### Fase 5: Resiliency, Config & Testing
Fase ini mengajarkan cara mengkonfigurasi environment secara aman, membuat automation test suite, menulis integration test, serta meningkatkan ketahanan (*reliability*) dan pemantauan (*observability*).
* [Episode36: Secret Management & Multi-environment Config](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-5-resiliency-observability/ep37-multi-env-secrets.md)
* [Episode37: Postman Collection & API Test Automation](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-5-resiliency-observability/ep38-postman-test-automation.md)
* [Episode38: Integration & E2E Testing](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-5-resiliency-observability/ep39-integration-e2e-testing.md)
* [Episode39: Reliability Patterns (Circuit Breakers & Retries)](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-5-resiliency-observability/ep40-reliability-patterns.md)
* [Episode40: Graceful Shutdown di Microservices](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-5-resiliency-observability/ep41-graceful-shutdown.md)
* [Episode41: Distributed Tracing (OpenTelemetry & Jaeger)](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-5-resiliency-observability/ep42-opentelemetry-jaeger.md)
* [Episode42: Metrics Monitoring (Prometheus & Grafana)](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-5-resiliency-observability/ep43-prometheus-grafana.md)
* [Episode43: Centralized Log Aggregation (ELK Stack)](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-5-resiliency-observability/ep44-elk-stack.md)

### Fase 6: DevOps & Automation
Fase deployment standard industri.
* [Episode44: Containerizing all Microservices](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-6-devops-cicd/ep45-dockerization.md)
* [Episode45: CI/CD Pipeline dengan GitHub Actions](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-6-devops-cicd/ep46-github-actions.md)

---

## 🛠️ Persiapan Lingkungan Kerja (Bertahap)

### Fase 1 (Cukup Install Ini Dulu):
* **Go** (v1.22+) — Bahasa utama.
* **VS Code / GoLand** — Editor kode.
* **Docker Desktop** — Untuk menjalankan database MySQL secara instan.
* **Postman / REST Client Extension** — Untuk test HTTP endpoint.
* **golang-migrate** — Tool database migration.
  - MacOS: `brew install golang-migrate`
  - Windows/Linux: Download binary dari GitHub repository `golang-migrate/migrate`.

---

## 📐 Filosofi Kurikulum Ini

1. **Evolusi Arsitektur:** Junior akan membangun sistem monolitik yang berfungsi penuh dan stabil terlebih dahulu. Ketika mereka mulai kesulitan dengan scaling, latency komunikasi internal, atau pemrosesan background yang memblokir request, barulah mereka diperkenalkan dengan microservices, gRPC, dan RabbitMQ.
2. **Praktis & Hands-On:** Setiap episode disertai dengan langkah-langkah praktis, contoh kode lengkap (tidak ada potongan TODO yang membingungkan), dan acceptance criteria.
3. **Mengutamakan Kualitas Kode:** Sejak awal, junior diajarkan menulis Unit Test dengan mocking. Hal ini memastikan bahwa kode mereka aman sebelum mengalami refactoring besar saat memecah service.
