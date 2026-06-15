# Episode 18: ELK + Prometheus + Grafana

## 🎯 Tujuan
- Integrate structured logging ke ELK Stack
- Setup Prometheus metrics di setiap service
- Buat Grafana dashboards

## 📝 Prerequisites
- ELK, Prometheus, Grafana containers running
- All services producing structured JSON logs

---

## 📦 Langkah-langkah

### Step 1: ELK Integration

**Log Flow:**
```
Service (Zap JSON log) → Logstash (TCP port 5000) → Elasticsearch → Kibana
```

Konfigurasi Zap agar kirim log ke Logstash via TCP:

```go
// Option 1: Gunakan Zap hook untuk kirim ke Logstash
// Option 2: Gunakan filebeat (sidecar) untuk baca log file
// Option 3: Log ke stdout → Docker logging driver → Logstash

// Paling simpel: Option 3 (Docker logging driver)
// Di docker-compose.yml, tambahkan logging config:
services:
  auth-service:
    logging:
      driver: "syslog"
      options:
        syslog-address: "tcp://localhost:5000"
        tag: "auth-service"
```

Atau buat custom Zap writer yang kirim ke Logstash TCP:
```go
type LogstashWriter struct {
    conn net.Conn
}

func (w *LogstashWriter) Write(p []byte) (n int, err error) {
    return w.conn.Write(p)
}

// Add sebagai Zap core tambahan
logstashCore := zapcore.NewCore(
    zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
    zapcore.AddSync(logstashWriter),
    zap.InfoLevel,
)

logger := zap.New(zapcore.NewTee(consoleCore, logstashCore))
```

### Step 2: Kibana Setup

```bash
# 1. Buka Kibana: http://localhost:5601
# 2. Go to Management → Index Patterns
# 3. Create index pattern: "gowallet-*"
# 4. Set time field: "@timestamp"
# 5. Go to Discover → pilih index pattern
# 6. Search logs by service_name, correlation_id, level, dll
```

### Step 3: Prometheus Metrics

Setiap service harus expose metrics di `GET /metrics`:

```go
import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

// Definisikan metrics
var (
    httpRequestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "http_requests_total",
            Help: "Total HTTP requests",
        },
        []string{"method", "path", "status"},
    )
    
    httpRequestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "http_request_duration_seconds",
            Help:    "HTTP request duration",
            Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
        },
        []string{"method", "path"},
    )
    
    dbQueryDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "db_query_duration_seconds",
            Help:    "Database query duration",
            Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
        },
        []string{"query_type", "table"},
    )
)

func init() {
    prometheus.MustRegister(httpRequestsTotal, httpRequestDuration, dbQueryDuration)
}

// Register /metrics endpoint
router.GET("/metrics", gin.WrapH(promhttp.Handler()))
```

**Metrics middleware:**
```go
func PrometheusMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        
        c.Next()
        
        duration := time.Since(start).Seconds()
        status := strconv.Itoa(c.Writer.Status())
        
        httpRequestsTotal.WithLabelValues(c.Request.Method, c.FullPath(), status).Inc()
        httpRequestDuration.WithLabelValues(c.Request.Method, c.FullPath()).Observe(duration)
    }
}
```

**Business metrics tambahan:**
```go
transactionCount = prometheus.NewCounterVec(...)   // Total transactions
walletBalanceTotal = prometheus.NewGauge(...)       // Total balance
cacheHits = prometheus.NewCounterVec(...)           // Cache hits/misses
circuitBreakerState = prometheus.NewGaugeVec(...)   // CB state per service
rabbitmqPublished = prometheus.NewCounterVec(...)   // Messages published
rabbitmqConsumed = prometheus.NewCounterVec(...)    // Messages consumed
```

### Step 4: Prometheus Config

Update `deployments/prometheus/prometheus.yml` agar scrape semua services (sudah dibuat di Episode 1).

### Step 5: Grafana Dashboards

```bash
# 1. Buka Grafana: http://localhost:3000 (admin/admin)
# 2. Add Data Source → Prometheus → URL: http://prometheus:9090
# 3. Create Dashboard
```

**Dashboard 1: Overview (RED Metrics)**
- Request Rate: `rate(http_requests_total[5m])`
- Error Rate: `rate(http_requests_total{status=~"5.."}[5m])`
- Duration (P95): `histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m]))`

**Dashboard 2: Per Service**
- Request rate per service
- Error rate per service
- Latency per service

**Dashboard 3: Infrastructure**
- MySQL connections
- Redis memory/connections
- RabbitMQ queue depth

**Dashboard 4: Business Metrics**
- Transaction volume per hour
- Top up/withdraw volume
- Active wallets

### Step 6: Alerting (Optional)

Di Grafana, buat alert rules:
- Error rate > 5% → Alert
- P95 latency > 2s → Alert
- DLQ count > 10 → Alert

---

## ✅ Acceptance Criteria

- [ ] Log tersedia dan searchable di Kibana
- [ ] Prometheus scrape metrics dari semua services
- [ ] `/metrics` endpoint menampilkan Prometheus format
- [ ] Grafana dashboards menampilkan data real-time
- [ ] RED metrics (Rate, Error, Duration) termonitor

---

## 📚 Referensi

- [Prometheus Go Client](https://github.com/prometheus/client_golang)
- [Grafana Dashboard Guide](https://grafana.com/docs/grafana/latest/dashboards/)
- [RED Method](https://grafana.com/blog/2018/08/02/the-red-method-how-to-instrument-your-services/)
- [ELK Stack Guide](https://www.elastic.co/guide/en/elastic-stack-get-started/current/index.html)
