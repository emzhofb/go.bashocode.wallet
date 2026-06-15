# 🏗️ Monolith to Microservices — Development Guide

> **Target Audience:** Junior Software Engineers & Free AI coding assistants.  
> **Repository Structure:** Starts as a single application, evolves into a Go Workspace (`go.work`) monorepo.  
> **Key Architecture Pattern:** Clean Architecture (Handler ➔ Service ➔ Repository).  
> **Difficulty Profile:** Slowly transitions from basic web CRUD to advanced distributed event-driven systems.

---

## 🗺️ Peta Kurikulum (6 Fase, 36 Episode)

Panduan ini dirancang untuk membimbing junior developer secara bertahap tanpa membuat mereka pusing dengan terlalu banyak teknologi di awal.

```
       [ Fase 1: Monolith Dasar ] (Eps 1-11)
                 │  (Go + MySQL + Clean Arch + Swagger)
                 ▼
     [ Fase 2: Cache & Background ] (Eps 12-18)
                 │  (Redis + OAuth2 + Token Rotation + RBAC)
                 ▼
    [ Fase 3: Pemecahan Microservices ] (Eps 19-23)
                 │  (API Gateway + gRPC + Outbox)
                 ▼
       [ Fase 4: Event-Driven ] (Eps 24-29)
                 │  (RabbitMQ + MongoDB Audit + Payment Service)
                 ▼
     [ Fase 5: Resiliency & Tracing ] (Eps 30-34)
                 │  (Circuit Breaker + OpenTelemetry + ELK)
                 ▼
        [ Fase 6: DevOps & CI/CD ] (Eps 35-36)
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
* [Episode 9: Pagination, Sorting, & Soft Delete](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-1-monolith/ep09-pagination-softdelete.md)
* [Episode 10: Unit Testing & Mocking](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-1-monolith/ep10-unit-testing.md)
* [Episode 11: API Documentation dengan Swagger/OpenAPI](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-1-monolith/ep11-swagger-openapi.md)

### Fase 2: Caching & Background Processing
Fase ini mengenalkan optimasi performa, pemrosesan latar belakang (background processing), dan security hardening pada autentikasi.
* [Episode 12: Redis Caching](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-2-enhancing-monolith/ep12-redis-caching.md)
* [Episode 13: Rate Limiting & JWT Blacklisting](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-2-enhancing-monolith/ep13-rate-limit-blacklist.md)
* [Episode 14: Background Schedulers (Cron)](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-2-enhancing-monolith/ep14-background-scheduler.md)
* [Episode 15: Email Verification & Forgot Password](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-2-enhancing-monolith/ep15-email-verification.md)
* [Episode 16: Integrasi Google OAuth](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-2-enhancing-monolith/ep16-google-oauth.md)
* [Episode 17: Refresh Token Rotation & Token Reuse Detection](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-2-enhancing-monolith/ep17-refresh-token-rotation.md)
* [Episode 18: Role-Based Access Control (RBAC)](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-2-enhancing-monolith/ep18-rbac.md)

### Fase 3: Pemecahan Microservices
Fase ini mengajarkan bagaimana memecah monolith menjadi layanan-layanan independen yang berkomunikasi lewat jaringan.
* [Episode 19: Monorepo & API Gateway (Reverse Proxy)](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-3-splitting-microservices/ep19-monorepo-gateway.md)
* [Episode 20: Memecah Auth & User Service](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-3-splitting-microservices/ep20-splitting-auth-user.md)
* [Episode 21: Komunikasi Internal gRPC](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-3-splitting-microservices/ep21-grpc-communication.md)
* [Episode 22: Memecah Wallet & Ledger Service](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-3-splitting-microservices/ep22-splitting-wallet-ledger.md)
* [Episode 23: Outbox Pattern untuk Transaksi Terdistribusi](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-3-splitting-microservices/ep23-outbox-pattern.md)

### Fase 4: Event-Driven Communication
Fase ini mengenalkan komunikasi asinkronus menggunakan message broker untuk skalabilitas yang lebih tinggi.
* [Episode 24: RabbitMQ & Event Publishing](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-4-event-driven/ep24-rabbitmq-publisher.md)
* [Episode 25: Payment Service (External Integration & Webhook HMAC)](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-4-event-driven/ep25-payment-service.md)
* [Episode 26: Consuming Payment Events in Wallet Service](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-4-event-driven/ep26-consuming-payment-events.md)
* [Episode 27: Notification Service (Consumer)](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-4-event-driven/ep27-notification-consumer.md)
* [Episode 28: Audit Logging dengan MongoDB](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-4-event-driven/ep28-audit-mongodb.md)
* [Episode 29: RabbitMQ Resiliency (DLQ & Retries)](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-4-event-driven/ep29-rabbitmq-dlq-retries.md)

### Fase 5: Resiliency & Observability
Fase ini mengajarkan cara membuat sistem yang tangguh menghadapi kegagalan jaringan dan mudah dipantau (monitored).
* [Episode 30: Reliability Patterns (Circuit Breakers & Retries)](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-5-resiliency-observability/ep30-reliability-patterns.md)
* [Episode 31: Graceful Shutdown di Microservices](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-5-resiliency-observability/ep31-graceful-shutdown.md)
* [Episode 32: Distributed Tracing (OpenTelemetry & Jaeger)](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-5-resiliency-observability/ep32-opentelemetry-jaeger.md)
* [Episode 33: Metrics Monitoring (Prometheus & Grafana)](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-5-resiliency-observability/ep33-prometheus-grafana.md)
* [Episode 34: Centralized Log Aggregation (ELK Stack)](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-5-resiliency-observability/ep34-elk-stack.md)

### Fase 6: DevOps & Automation
Fase deployment standard industri.
* [Episode 35: Containerizing all Microservices](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-6-devops-cicd/ep35-dockerization.md)
* [Episode 36: CI/CD Pipeline dengan GitHub Actions](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/learning-path/phase-6-devops-cicd/ep36-github-actions.md)

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
