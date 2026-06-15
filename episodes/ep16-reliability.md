# Episode 16: Reliability Patterns

## 🎯 Tujuan
- Circuit breaker untuk inter-service communication
- Retry policy dengan exponential backoff
- Timeout configuration
- Graceful shutdown verification

## 📝 Prerequisites
- Multiple services sudah berjalan dan saling berkomunikasi

---

## 📦 Langkah-langkah

### Step 1: Circuit Breaker

**Library:** `github.com/sony/gobreaker`

```go
import "github.com/sony/gobreaker"

// States:
// CLOSED  → Semua request dilewatkan (normal)
// OPEN    → Semua request langsung gagal (fast-fail)
// HALF-OPEN → Beberapa request dicoba untuk test

cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
    Name:        "wallet-service",
    MaxRequests: 3,                          // Max request saat HALF-OPEN
    Interval:    10 * time.Second,           // Reset counter setiap 10s (saat CLOSED)
    Timeout:     30 * time.Second,           // Durasi di OPEN sebelum HALF-OPEN
    ReadyToTrip: func(counts gobreaker.Counts) bool {
        return counts.ConsecutiveFailures > 5 // Trip setelah 5 failure berturut-turut
    },
    OnStateChange: func(name string, from, to gobreaker.State) {
        logger.Warn("circuit breaker state change",
            zap.String("name", name),
            zap.String("from", from.String()),
            zap.String("to", to.String()),
        )
    },
})

// Usage:
result, err := cb.Execute(func() (interface{}, error) {
    return walletClient.GetBalance(ctx, userID)
})
```

Terapkan circuit breaker di setiap inter-service call:

| Caller → Target | Circuit Breaker Name |
|---|---|
| Transaction → Wallet | `cb-wallet` |
| Transaction → Ledger | `cb-ledger` |
| Wallet → Ledger | `cb-ledger` |
| Gateway → Any Service | `cb-<service>` |

### Step 2: Retry Policy

```go
func RetryWithBackoff(ctx context.Context, maxRetries int, fn func() error) error {
    var lastErr error
    for i := 0; i <= maxRetries; i++ {
        err := fn()
        if err == nil {
            return nil
        }
        lastErr = err
        
        if i < maxRetries {
            // Exponential backoff: 1s, 2s, 4s...
            backoff := time.Duration(math.Pow(2, float64(i))) * time.Second
            
            // Add jitter (±25%)
            jitter := time.Duration(rand.Float64() * 0.5 * float64(backoff))
            backoff = backoff + jitter - (backoff / 4)
            
            logger.Info("retrying",
                zap.Int("attempt", i+1),
                zap.Duration("backoff", backoff),
                zap.Error(err))
            
            select {
            case <-ctx.Done():
                return ctx.Err()
            case <-time.After(backoff):
            }
        }
    }
    return fmt.Errorf("max retries exceeded: %w", lastErr)
}
```

### Step 3: Timeout per Service Call

| Caller → Target | Timeout |
|---|---|
| Gateway → Any Service | 30s |
| Transaction → Wallet (gRPC) | 5s |
| Transaction → Ledger (gRPC) | 5s |
| Any → Redis | 1s |
| Any → MySQL | 5s |
| Any → RabbitMQ Publish | 3s |

```go
// Implement dengan context timeout:
ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
defer cancel()

result, err := walletClient.GetBalance(ctx, userID)
// Jika timeout → context.DeadlineExceeded error
```

### Step 4: Verify Graceful Shutdown

Checklist untuk setiap service:
- [ ] Handle SIGINT dan SIGTERM
- [ ] HTTP server Shutdown (tunggu request selesai)
- [ ] gRPC server GracefulStop
- [ ] Close database connections
- [ ] Close Redis connections
- [ ] Close RabbitMQ connections
- [ ] Stop cron jobs

Test:
```bash
# Start service lalu kill
kill -SIGTERM <pid>
# Log harus menunjukkan graceful shutdown
# Tidak boleh ada "connection reset" di client
```

### Step 5: Prometheus Metrics

```go
// Track circuit breaker state
circuitBreakerGauge := prometheus.NewGaugeVec(
    prometheus.GaugeOpts{
        Name: "circuit_breaker_state",
        Help: "Circuit breaker state (0=closed, 1=open, 2=half-open)",
    },
    []string{"service"},
)

// Track retry count
retryCounter := prometheus.NewCounterVec(
    prometheus.CounterOpts{
        Name: "service_call_retries_total",
        Help: "Total number of retries",
    },
    []string{"target_service", "result"},
)
```

---

## ✅ Acceptance Criteria

- [ ] Circuit breaker mencegah cascade failure
- [ ] CB OPEN state → fast-fail tanpa call ke service
- [ ] Retry dengan exponential backoff bekerja
- [ ] Timeout tersetup di semua service calls
- [ ] Graceful shutdown di semua services
- [ ] Prometheus metrics untuk CB state

---

## 📚 Referensi

- [Circuit Breaker Pattern](https://martinfowler.com/bliki/CircuitBreaker.html)
- [sony/gobreaker](https://github.com/sony/gobreaker)
- [Exponential Backoff](https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/)
