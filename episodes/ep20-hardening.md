# Episode 20: Hardening & Final Integration

## 🎯 Tujuan
- Integration testing end-to-end
- Postman collection lengkap
- Security hardening
- Performance tuning
- Documentation finalisasi

## 📝 Prerequisites
- Episode 1-19 semua selesai

---

## 📦 Langkah-langkah

### Step 1: Integration Test (End-to-End)

Buat integration test yang menjalankan full flow:

```go
// integration_test.go
func TestFullTransferFlow(t *testing.T) {
    // 1. Register User A
    respA := POST("/api/v1/auth/register", RegisterRequest{
        Email: "usera@test.com", Password: "password123", FullName: "User A",
    })
    assert.Equal(t, 201, respA.StatusCode)
    tokenA := respA.Body.AccessToken
    
    // 2. Register User B
    respB := POST("/api/v1/auth/register", RegisterRequest{
        Email: "userb@test.com", Password: "password123", FullName: "User B",
    })
    tokenB := respB.Body.AccessToken
    
    // 3. Verify email User A
    // ... (get OTP from DB, verify)
    
    // 4. Top up User A: Rp 100.000
    POST("/api/v1/wallets/topup", TopUpRequest{
        Amount: 100000, IdempotencyKey: uuid.New().String(),
    }, WithToken(tokenA))
    
    // 5. Check balance A = 100.000
    balanceA := GET("/api/v1/wallets/me", WithToken(tokenA))
    assert.Equal(t, 100000.0, balanceA.Body.Balance)
    
    // 6. Transfer A → B: Rp 50.000
    POST("/api/v1/transactions/transfer", TransferRequest{
        ReceiverID: userBID, Amount: 50000,
        IdempotencyKey: uuid.New().String(),
    }, WithToken(tokenA))
    
    // 7. Check balance A = 50.000
    balanceA = GET("/api/v1/wallets/me", WithToken(tokenA))
    assert.Equal(t, 50000.0, balanceA.Body.Balance)
    
    // 8. Check balance B = 50.000
    balanceB := GET("/api/v1/wallets/me", WithToken(tokenB))
    assert.Equal(t, 50000.0, balanceB.Body.Balance)
    
    // 9. Check transaction history
    txHistory := GET("/api/v1/transactions", WithToken(tokenA))
    assert.GreaterOrEqual(t, len(txHistory.Body.Data), 1)
    
    // 10. Check ledger entries
    ledger := GET("/api/v1/ledger/entries?wallet_id=...", WithToken(tokenA))
    assert.GreaterOrEqual(t, len(ledger.Body.Entries), 2) // topup + transfer
    
    // 11. Reconcile
    reconcile := GET("/api/v1/ledger/reconcile?wallet_id=...", WithToken(adminToken))
    assert.True(t, reconcile.Body.IsConsistent)
    
    // 12. Check audit logs (admin)
    auditLogs := GET("/api/v1/audit/logs?actor_id=...", WithToken(adminToken))
    assert.GreaterOrEqual(t, len(auditLogs.Body.Data), 1)
}
```

Jalankan:
```bash
# Start full stack
docker compose up -d

# Run integration tests
go test -tags integration -v -timeout 5m ./tests/...
```

### Step 2: Postman Collection

Buat Postman Collection (`postman/GoWallet.postman_collection.json`):

**Struktur:**
```
GoWallet API
├── Auth
│   ├── Register
│   ├── Login
│   ├── Refresh Token
│   ├── Logout
│   ├── Google OAuth
│   ├── Send Verification
│   ├── Verify Email
│   ├── Forgot Password
│   └── Reset Password
├── User
│   ├── Get Profile
│   ├── Update Profile
│   ├── Upload Avatar
│   ├── Change Password
│   ├── Delete Account
│   ├── [Admin] List Users
│   └── [Admin] Get User
├── Wallet
│   ├── Get Balance
│   ├── Top Up
│   ├── Withdraw
│   ├── [Admin] Freeze
│   └── [Admin] Unfreeze
├── Transaction
│   ├── Transfer
│   ├── History
│   └── Detail
├── Payment
│   ├── Create Payment
│   ├── Payment Status
│   └── Payment History
├── Ledger
│   ├── Entries
│   ├── Balance
│   └── [Admin] Reconcile
├── Audit
│   ├── [Admin] List Logs
│   ├── [Admin] Log Detail
│   └── [Admin] User Logs
└── Health
    ├── Health
    ├── Ready
    └── Live
```

**Postman Environment Variables:**
```json
{
  "base_url": "http://localhost:8080",
  "access_token": "",
  "refresh_token": "",
  "admin_token": "",
  "user_id": "",
  "wallet_id": ""
}
```

**Pre-request Script (Auto-login):**
```javascript
// Otomatis login dan set token sebelum request
if (!pm.environment.get("access_token")) {
    pm.sendRequest({
        url: pm.environment.get("base_url") + "/api/v1/auth/login",
        method: "POST",
        header: { "Content-Type": "application/json" },
        body: { mode: "raw", raw: JSON.stringify({
            email: "test@example.com",
            password: "password123"
        })}
    }, function(err, res) {
        pm.environment.set("access_token", res.json().data.access_token);
        pm.environment.set("refresh_token", res.json().data.refresh_token);
    });
}
```

### Step 3: Security Hardening Checklist

Go through each item:

- [ ] **Authentication:** Semua protected endpoint memerlukan JWT valid
- [ ] **Authorization:** RBAC diterapkan (admin-only endpoints)
- [ ] **Input Validation:** Semua input divalidasi (type, length, format)
- [ ] **SQL Injection:** Semua query pakai parameterized queries (bukan string concat)
- [ ] **XSS Prevention:** Output di-escape (relevan jika ada HTML rendering)
- [ ] **Rate Limiting:** Aktif dan tier-based
- [ ] **CORS:** Properly configured (bukan `*` di production)
- [ ] **Sensitive Data:**
  - [ ] Password tidak pernah di-log
  - [ ] Token tidak pernah di-log
  - [ ] Email tidak di-log di production
- [ ] **HTTPS:** Enforced di production (HTTP → redirect ke HTTPS)
- [ ] **Security Headers:**
  - [ ] `X-Content-Type-Options: nosniff`
  - [ ] `X-Frame-Options: DENY`
  - [ ] `X-XSS-Protection: 1; mode=block`
  - [ ] `Strict-Transport-Security: max-age=31536000`
  - [ ] `Content-Security-Policy: default-src 'self'`
- [ ] **Token Security:**
  - [ ] Access token short-lived (15 menit)
  - [ ] Refresh token rotated on use
  - [ ] Token reuse detection aktif
  - [ ] Token blacklist on logout
- [ ] **Password Security:**
  - [ ] bcrypt cost ≥ 12
  - [ ] Min password length 8
  - [ ] Password change requires current password

### Step 4: Performance Checklist

- [ ] **Database:**
  - [ ] Indexes untuk semua query patterns
  - [ ] Connection pooling configured
  - [ ] Slow query monitoring
- [ ] **Redis:**
  - [ ] Cache hit ratio > 50%
  - [ ] Appropriate TTL values
- [ ] **API:**
  - [ ] P95 latency < 500ms
  - [ ] Pagination di semua list endpoints
  - [ ] Response compression (gzip)
- [ ] **Memory:**
  - [ ] No goroutine leaks
  - [ ] pprof profiling

Benchmark:
```bash
# Install hey (HTTP load generator)
go install github.com/rakyll/hey@latest

# Load test
hey -n 1000 -c 50 -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/wallets/me
```

### Step 5: Documentation

- [ ] **Root README.md:**
  - [ ] Project overview
  - [ ] Architecture diagram
  - [ ] Quick start guide
  - [ ] Environment setup
  - [ ] How to run
- [ ] **Per-service README.md:**
  - [ ] Service description
  - [ ] API endpoints
  - [ ] Dependencies
  - [ ] How to test
- [ ] **Swagger:** Updated untuk setiap service
- [ ] **Postman Collection:** Complete dan tested
- [ ] **Architecture Decision Records (ADR):** Dokumentasi keputusan teknis penting

---

## ✅ Final Acceptance Criteria

- [ ] Full integration test pass end-to-end
- [ ] Postman collection bisa dijalankan dari awal sampai akhir
- [ ] Security checklist semua tercentang
- [ ] Performance benchmarks memenuhi target
- [ ] Dokumentasi lengkap
- [ ] Docker Compose full stack berjalan
- [ ] CI/CD pipeline green
- [ ] Grafana dashboards menampilkan data yang bermakna
- [ ] Jaeger menampilkan traces end-to-end
- [ ] Kibana menampilkan logs searchable

---

## 🎉 Selamat!

Jika kamu sampai di sini, kamu sudah berhasil membangun **production-grade microservice** yang mencakup:

- ✅ 9 microservices (Gateway, Auth, User, Wallet, Ledger, Transaction, Payment, Notification, Audit)
- ✅ 1 scheduler service
- ✅ REST API + gRPC internal
- ✅ JWT Authentication + Google OAuth + RBAC
- ✅ Event-driven architecture (RabbitMQ + Outbox Pattern)
- ✅ Distributed tracing (OpenTelemetry + Jaeger)
- ✅ Observability stack (ELK + Prometheus + Grafana)
- ✅ Reliability patterns (Circuit Breaker, Retry, Timeout)
- ✅ Data consistency (Optimistic Locking, Idempotency, Ledger)
- ✅ CI/CD (Docker + GitHub Actions)

**Ini bukan MVP.** Ini adalah arsitektur yang mendekati sistem finansial nyata. 🏦

---

## 📚 Next Steps (Beyond Roadmap)

Jika mau lanjut belajar:
- **Kubernetes deployment** — Deploy ke K8s cluster
- **Service Mesh (Istio/Linkerd)** — Advanced traffic management
- **Event Sourcing** — Full event sourcing pattern (bukan hanya outbox)
- **CQRS** — Command Query Responsibility Segregation
- **Multi-tenancy** — Support multiple organizations
- **WebSocket** — Real-time notifications
- **Frontend** — Build React/Next.js frontend
