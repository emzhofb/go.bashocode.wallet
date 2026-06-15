# Episode 10: Payment Service

## 🎯 Tujuan
- Mock payment gateway (simulate external payment)
- Payment callback handling
- Payment confirmation
- Retry mechanism dengan exponential backoff
- Publish `payment.completed` event

## 📝 Prerequisites
- Episode 9 selesai (Transaction Service)

---

## 📦 Langkah-langkah

### Step 1: Inisialisasi Service

```bash
mkdir -p payment-service/cmd
mkdir -p payment-service/internal/{handler,service,repository,model,dto,config,gateway}
mkdir -p payment-service/db/migrations

cd payment-service
go mod init github.com/emzhofb/gowallet/payment-service
cd ..
go work use ./payment-service
```

### Step 2: Database Migration

**`000001_create_payments.up.sql`:**
```sql
CREATE TABLE IF NOT EXISTS payments (
    id CHAR(36) PRIMARY KEY,
    transaction_id CHAR(36) NOT NULL,
    user_id CHAR(36) NOT NULL,
    payment_method ENUM('bank_transfer', 'credit_card', 'ewallet') NOT NULL,
    amount DECIMAL(15,2) NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'IDR',
    status ENUM('pending', 'processing', 'completed', 'failed', 'expired') NOT NULL DEFAULT 'pending',
    external_ref VARCHAR(255) DEFAULT NULL,
    callback_data JSON DEFAULT NULL,
    retry_count INT NOT NULL DEFAULT 0,
    max_retries INT NOT NULL DEFAULT 3,
    failure_reason VARCHAR(500) DEFAULT NULL,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    INDEX idx_transaction (transaction_id),
    INDEX idx_user (user_id),
    INDEX idx_external_ref (external_ref),
    INDEX idx_status (status),
    INDEX idx_expires (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

### Step 3: Endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/api/v1/payments/create` | ✅ | Initiate payment (top up) |
| POST | `/api/v1/payments/callback` | ❌* | Callback dari payment gateway |
| GET | `/api/v1/payments/:id` | ✅ | Cek payment status |
| GET | `/api/v1/payments` | ✅ | Payment history |

*Callback di-protect oleh signature verification, bukan JWT.

### Step 4: Mock Payment Gateway

Buat internal mock di `payment-service/internal/gateway/mock_gateway.go`:

```go
// MockGateway mensimulasikan payment gateway eksternal
type MockGateway struct {
    callbackURL string
    httpClient  *http.Client
}

type PaymentRequest struct {
    PaymentID     string  `json:"payment_id"`
    Amount        float64 `json:"amount"`
    Currency      string  `json:"currency"`
    PaymentMethod string  `json:"payment_method"`
    CallbackURL   string  `json:"callback_url"`
}

type PaymentResult struct {
    ExternalRef string `json:"external_ref"`
    Status      string `json:"status"`    // "success" atau "failed"
    Message     string `json:"message"`
}

func (g *MockGateway) ProcessPayment(req PaymentRequest) {
    // Simulate async processing
    go func() {
        // 1. Simulate processing delay (1-3 detik)
        delay := time.Duration(1+rand.Intn(3)) * time.Second
        time.Sleep(delay)
        
        // 2. Random result (80% success, 20% failure)
        success := rand.Float32() < 0.8
        
        result := PaymentResult{
            ExternalRef: "EXT-" + uuid.New().String()[:8],
        }
        
        if success {
            result.Status = "success"
            result.Message = "Payment processed successfully"
        } else {
            result.Status = "failed"
            result.Message = "Payment declined by bank"
        }
        
        // 3. Send callback
        callbackBody := map[string]interface{}{
            "payment_id":   req.PaymentID,
            "external_ref": result.ExternalRef,
            "status":       result.Status,
            "message":      result.Message,
            "timestamp":    time.Now().Format(time.RFC3339),
            "signature":    generateSignature(req.PaymentID, result.Status), // HMAC
        }
        
        body, _ := json.Marshal(callbackBody)
        http.Post(req.CallbackURL, "application/json", bytes.NewBuffer(body))
    }()
}

// Signature verification (HMAC-SHA256)
func generateSignature(paymentID, status string) string {
    secret := "payment-gateway-secret"
    message := paymentID + ":" + status
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write([]byte(message))
    return hex.EncodeToString(mac.Sum(nil))
}
```

### Step 5: Service Implementation

**Create Payment Flow:**
```
1. User request top up → create payment record (status: pending)
2. Send to mock gateway → status: processing
3. Gateway process async → callback ke /payments/callback
4. Callback handler:
   a. Verify signature
   b. Update payment status (completed/failed)
   c. Jika completed → call Wallet Service top up + create ledger entry
   d. Insert outbox event (payment.completed)
```

**Retry Mechanism:**
```go
func (s *paymentService) RetryPayment(ctx context.Context, paymentID string) error {
    payment, _ := s.paymentRepo.GetByID(ctx, paymentID)
    
    if payment.Status != "failed" {
        return ErrCannotRetry
    }
    
    if payment.RetryCount >= payment.MaxRetries {
        return ErrMaxRetriesExceeded
    }
    
    // Increment retry count
    payment.RetryCount++
    payment.Status = "processing"
    s.paymentRepo.Update(ctx, payment)
    
    // Resubmit to gateway
    s.gateway.ProcessPayment(PaymentRequest{
        PaymentID:     payment.ID,
        Amount:        payment.Amount,
        PaymentMethod: payment.PaymentMethod,
        CallbackURL:   s.callbackURL,
    })
    
    return nil
}
```

**Callback Handler (dengan signature verification):**
```go
func (h *PaymentHandler) Callback(c *gin.Context) {
    var callback CallbackRequest
    c.ShouldBindJSON(&callback)
    
    // 1. Verify signature (HMAC)
    expectedSig := generateSignature(callback.PaymentID, callback.Status)
    if callback.Signature != expectedSig {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
        return
    }
    
    // 2. Process callback
    err := h.paymentService.ProcessCallback(c.Request.Context(), callback)
    
    c.JSON(http.StatusOK, gin.H{"received": true})
}
```

### Step 6: Test Manual

```bash
# 1. Create payment (top up)
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  http://localhost:8086/api/v1/payments/create \
  -d '{
    "amount": 100000,
    "payment_method": "bank_transfer"
  }'

# 2. Tunggu 1-3 detik (mock gateway processing)

# 3. Check payment status
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8086/api/v1/payments/$PAYMENT_ID

# 4. Jika status = completed, cek saldo wallet
curl -H "Authorization: Bearer $TOKEN" http://localhost:8083/api/v1/wallets/me
```

---

## ✅ Acceptance Criteria

- [ ] Payment bisa diinisiasi dengan payment method
- [ ] Mock gateway memproses dan mengirim callback
- [ ] Signature verification bekerja
- [ ] Callback mengupdate status payment
- [ ] Payment completed → saldo wallet bertambah
- [ ] Payment failed → saldo wallet tidak berubah
- [ ] Retry bekerja (increment counter, resubmit)
- [ ] Max retry tercapai → tidak bisa retry lagi
- [ ] `payment.completed` event masuk outbox
- [ ] Payment history paginated

---

## 💡 Tips

1. **Mock gateway di production diganti** dengan real gateway (Midtrans, Stripe, dll)
2. **Signature verification wajib** — jangan percaya callback tanpa verifikasi
3. **Callback bisa datang lebih dari sekali** — handle idempotently
4. **Payment expiry** — set `expires_at` (misalnya 24 jam), expired payment auto-cancelled

---

## 📚 Referensi Belajar

- [Payment System Design](https://newsletter.pragmaticengineer.com/p/designing-a-payment-system)
- [Webhook Best Practices](https://hookdeck.com/webhooks/guides/webhook-best-practices)
- [HMAC Verification](https://en.wikipedia.org/wiki/HMAC)
