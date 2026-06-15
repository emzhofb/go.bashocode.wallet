# Tambahan Arsitektur GoWallet Microservice (Production-Grade)

## 🏦 Ledger Service (Sangat Direkomendasikan)

Ledger adalah sumber kebenaran (source of truth) untuk semua pergerakan uang. Saldo wallet dapat dihitung dari akumulasi entri ledger, sehingga setiap transaksi memiliki jejak audit yang jelas.

### Tanggung Jawab

* Mencatat setiap debit dan kredit
* Menyimpan histori transaksi yang immutable
* Mendukung rekonsiliasi data
* Menjadi dasar audit finansial

### Contoh Entri Ledger

| Transaction ID | Wallet ID | Type   | Amount  |
| -------------- | --------- | ------ | ------- |
| TX001          | U001      | Credit | +100000 |
| TX002          | U001      | Debit  | -25000  |
| TX003          | U001      | Credit | +50000  |

### Keuntungan

* Tidak kehilangan histori saldo
* Mempermudah audit dan debugging
* Mendukung rollback atau investigasi transaksi
* Lebih mendekati implementasi sistem finansial nyata

---

# Bonus Fitur "Production-Like"

## API & Arsitektur

* API Versioning (`/api/v1`)
* Request Correlation ID
* Distributed Tracing (OpenTelemetry + Jaeger)
* gRPC untuk komunikasi internal
* REST API untuk client
* API Gateway sebagai single entry point

## Keamanan

* JWT Access Token
* JWT Refresh Token
* Google OAuth
* Email Verification
* Forgot Password & Reset Password
* Password Hashing
* Role-Based Access Control (RBAC)
* Rate Limiting berbasis Redis

## Konsistensi Data

* Idempotency Key untuk transfer dan top up
* Optimistic Locking saat update saldo
* Outbox Pattern untuk publish event secara konsisten
* Dead Letter Queue (DLQ) pada RabbitMQ
* Retry Policy untuk event yang gagal diproses
* Circuit Breaker untuk komunikasi antar service

## Observability

* Structured JSON Logging
* ELK Stack (Elasticsearch, Logstash, Kibana)
* Prometheus Metrics
* Grafana Dashboard
* Health Check (`/health`)
* Readiness Check (`/ready`)
* Liveness Check (`/live`)

## Data & Query

* Pagination
* Sorting
* Filtering
* Soft Delete untuk data yang tidak benar-benar dihapus
* Audit Trail lengkap
* Ledger yang immutable untuk seluruh mutasi saldo

## Background Processing

* Scheduled Cleanup Jobs
* Scheduled Token Cleanup
* Scheduled Expired OTP Cleanup
* Scheduled Refresh Token Cleanup

## Infrastruktur

* Multi-environment (`development`, `staging`, `production`)
* Environment Variables (`.env`)
* Secret Management
* Docker Compose untuk seluruh stack
* GitHub Actions untuk CI/CD
* Graceful Shutdown

## Dokumentasi & Testing

* Swagger / OpenAPI
* Postman Collection
* Unit Test
* Integration Test
* End-to-End Test (opsional)
* Test Coverage Report

---

# Arsitektur Service yang Disarankan

* API Gateway
* Auth Service
* User Service
* Wallet Service
* **Ledger Service**
* Transaction Service
* Payment Service
* Notification Service
* Audit Service

---

# Urutan Alur Transfer Saldo

1. Client mengirim request transfer ke API Gateway.
2. Gateway meneruskan request ke Transaction Service.
3. Transaction Service memvalidasi idempotency key.
4. Wallet Service memeriksa saldo pengirim.
5. Jika valid, Ledger Service mencatat debit pada pengirim.
6. Ledger Service mencatat kredit pada penerima.
7. Wallet Service memperbarui saldo menggunakan optimistic locking.
8. Transaction Service menyimpan status transaksi.
9. Event `transfer.completed` dipublikasikan ke RabbitMQ melalui Outbox Pattern.
10. Notification Service mengirim notifikasi.
11. Audit Service menyimpan jejak aktivitas.
12. Prometheus merekam metrik, ELK menyimpan log, dan Grafana menampilkan dashboard monitoring.
