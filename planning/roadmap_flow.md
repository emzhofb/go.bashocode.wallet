# 🚀 Roadmap Microservices Banking Project (Golang)

## 📌 Tahap 1 – Fondasi

* [ ] Inisialisasi monorepo atau multi-repo
* [ ] Setup Docker Compose
* [ ] Setup MySQL
* [ ] Setup MongoDB
* [ ] Setup Redis
* [ ] Setup RabbitMQ
* [ ] Setup ELK Stack
* [ ] Setup Prometheus
* [ ] Setup Grafana
* [ ] Shared config & environment
* [ ] Shared logger
* [ ] Shared error handling

---

## 🌐 Tahap 2 – API Gateway

* [ ] Reverse proxy
* [ ] JWT middleware
* [ ] Rate limiter (Redis)
* [ ] Request ID / Correlation ID
* [ ] Logging middleware
* [ ] CORS
* [ ] API Versioning (`/api/v1`)
* [ ] Health check (`/health`)
* [ ] Readiness check (`/ready`)

---

## 🔐 Tahap 3 – Auth Service

* [ ] Register
* [ ] Login
* [ ] JWT Access Token
* [ ] Refresh Token
* [ ] Logout
* [ ] Google OAuth
* [ ] Email verification
* [ ] Forgot password
* [ ] Reset password
* [ ] Password hashing
* [ ] Swagger / OpenAPI
* [ ] Unit test

---

## 👤 Tahap 4 – User Service

* [ ] Get profile
* [ ] Update profile
* [ ] Upload avatar
* [ ] Change password
* [ ] Soft delete user
* [ ] Pagination & filtering
* [ ] gRPC endpoint

---

## 💰 Tahap 5 – Wallet Service

* [ ] Buat wallet otomatis
* [ ] Cek saldo
* [ ] Top up
* [ ] Withdraw
* [ ] Freeze / unfreeze wallet
* [ ] Optimistic locking
* [ ] Redis cache

---

## 📒 Tahap 6 – Ledger Service ⭐

* [ ] Immutable debit / credit entries
* [ ] Double-entry bookkeeping sederhana
* [ ] Riwayat mutasi
* [ ] Rekonsiliasi saldo
* [ ] gRPC API

---

## 💸 Tahap 7 – Transaction Service

* [ ] Transfer antar pengguna
* [ ] Idempotency key
* [ ] Transaction history
* [ ] Transaction detail
* [ ] Pagination & filtering
* [ ] Publish event ke Outbox

---

## 💳 Tahap 8 – Payment Service

* [ ] Mock payment gateway
* [ ] Payment callback
* [ ] Payment confirmation
* [ ] Retry transaksi gagal
* [ ] Publish `payment.completed`

---

## 📧 Tahap 9 – Notification Service

* [ ] Consume RabbitMQ
* [ ] Email top up
* [ ] Email transfer
* [ ] Email reset password
* [ ] Retry notification

---

## 🛡️ Tahap 10 – Audit Service

* [ ] Login log
* [ ] Transfer log
* [ ] Admin activity log
* [ ] Security events
* [ ] Simpan ke MongoDB

---

## ⏰ Tahap 11 – Scheduler / Worker Service ⭐

* [ ] Cleanup refresh token kedaluwarsa
* [ ] Cleanup OTP
* [ ] Retry event gagal
* [ ] Scheduled report
* [ ] Scheduled maintenance

---

## 📊 Tahap 12 – Observability

* [ ] Structured JSON logging
* [ ] ELK integration
* [ ] Prometheus metrics
* [ ] Grafana dashboard
* [ ] OpenTelemetry
* [ ] Jaeger tracing

---

## 🐰 Tahap 13 – RabbitMQ Lanjutan

* [ ] Exchange & routing key
* [ ] Dead Letter Queue (DLQ)
* [ ] Retry queue
* [ ] Consumer acknowledgment
* [ ] Poison message handling

---

## ⚙️ Tahap 14 – Reliability

* [ ] Circuit breaker
* [ ] Retry policy
* [ ] Timeout
* [ ] Graceful shutdown
* [ ] Health, readiness & liveness checks

---

## 🚀 Tahap 15 – CI/CD & Deployment

* [ ] Dockerfile tiap service
* [ ] Docker Compose penuh
* [ ] GitHub Actions (lint, test, build)
* [ ] Multi-environment (dev, staging, prod)
* [ ] Secret management
* [ ] Environment variables

---

# 🎬 Daftar Episode Implementasi

| Episode | Materi                                                                              |
| ------- | ----------------------------------------------------------------------------------- |
| 1       | Setup proyek & Docker Compose                                                       |
| 2       | API Gateway                                                                         |
| 3       | Auth (JWT)                                                                          |
| 4       | Refresh Token                                                                       |
| 5       | Google OAuth                                                                        |
| 6       | Email Verification & Reset Password                                                 |
| 7       | User Service                                                                        |
| 8       | Wallet Service                                                                      |
| 9       | Ledger Service                                                                      |
| 10      | Transfer & Transaction Service                                                      |
| 11      | Payment Service                                                                     |
| 12      | RabbitMQ & Event-Driven                                                             |
| 13      | Notification Service                                                                |
| 14      | Audit Service                                                                       |
| 15      | Scheduler / Worker                                                                  |
| 16      | Redis & Rate Limiter                                                                |
| 17      | OpenTelemetry & Jaeger                                                              |
| 18      | ELK + Prometheus + Grafana                                                          |
| 19      | CI/CD & GitHub Actions                                                              |
| 20      | Hardening: Outbox Pattern, DLQ, Circuit Breaker, Optimistic Locking, dan Deployment |
