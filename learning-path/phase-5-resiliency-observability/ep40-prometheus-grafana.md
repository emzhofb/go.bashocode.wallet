# Episode 40: Metrics Monitoring (Prometheus & Grafana)

## 🎯 Tujuan
* Memahami konsep **RED Metrics** (Rate, Error, Duration) untuk memantau kesehatan server secara statistik.
* Menambahkan **Prometheus** dan **Grafana** ke Docker Compose.
* Mengekspos endpoint `/metrics` di setiap microservice menggunakan library Prometheus client.
* Memvisualisasikan data metrik (grafik performa CPU, total request, error rate, dan kecepatan response) di dashboard Grafana.

---

## 📐 Konsep Metrics Monitoring (RED Method)
Berbeda dengan logs (yang mencatat kejadian spesifik) dan tracing (yang melacak alur satu request), **Metrics** mencatat statistik performa sistem secara keseluruhan dalam format deret waktu (*time-series data*).

Kami memantau metrik menggunakan metode **RED**:
1. **Rate (Jumlah Request):** Berapa banyak request HTTP yang masuk per detik? (Key: `http_requests_total`).
2. **Error (Tingkat Kegagalan):** Berapa banyak request yang menghasilkan status 5xx atau 4xx?
3. **Duration (Durasi/Latency):** Berapa lama rata-rata server memproses request? (Key: `http_request_duration_seconds`).

Prometheus bertindak sebagai *scraper* (penarik data) yang secara periodik mendatangi endpoint `GET /metrics` di setiap service untuk mengambil angka statistik terbaru, lalu menyerahkannya ke Grafana untuk digambar menjadi grafik visual.

```
[ Grafana Dashboard ] ➔ (Query) ➔ [ Prometheus Server ]
                                          │
                                   (Scrape /metrics)
                                          ▼
                         [ API Gateway ]  [ Wallet Service ]
```

---

## 📦 Langkah-langkah

### Step 1: Tambahkan Prometheus & Grafana ke Docker Compose
Buka file `docker-compose.yml` di folder root workspace, tambahkan service `prometheus` dan `grafana`:

```yaml
# ... services mysql, redis, rabbitmq, mongodb, jaeger ...
  prometheus:
    image: prom/prometheus:v2.50.0
    container_name: gowallet-prometheus
    volumes:
      - ./deployments/prometheus/prometheus.yml:/etc/prometheus/prometheus.yml
    ports:
      - "9090:9090"
    restart: always

  grafana:
    image: grafana/grafana:10.3.3
    container_name: gowallet-grafana
    ports:
      - "3000:3000"
    restart: always
```

Buat file konfigurasi Prometheus di `./deployments/prometheus/prometheus.yml` di folder root workspace:

```yaml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'api-gateway'
    static_configs:
      - targets: ['host.docker.internal:8080'] # IP host lokal dari dalam docker

  - job_name: 'auth-service'
    static_configs:
      - targets: ['host.docker.internal:8081']

  - job_name: 'wallet-service'
    static_configs:
      - targets: ['host.docker.internal:8082']
```

Jalankan container: `docker compose up -d`. 
* Prometheus UI: `http://localhost:9090`
* Grafana UI: `http://localhost:3000` (username/password default: `admin`/`admin`).

### Step 2: Install Prometheus Go Client SDK
Di setiap service Go, unduh library client Prometheus:

```bash
go get github.com/prometheus/client_golang/prometheus
go get github.com/prometheus/client_golang/prometheus/promhttp
```

### Step 3: Implementasi Middleware Metrics & Endpoint `/metrics`
Buat middleware baru di `wallet-service/internal/middleware/metrics.go` (lakukan hal yang sama untuk service lainnya):

```go
package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// 1. Counter untuk menghitung jumlah total request
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests processed.",
		},
		[]string{"method", "path", "status"},
	)

	// 2. Histogram untuk merekam durasi kecepatan respons
	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds.",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}, // Interval bucket detik
		},
		[]string{"method", "path"},
	)
)

func init() {
	// Daftarkan metrik ke register default Prometheus
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpRequestDuration)
}

func PrometheusMetrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())
		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}

		// Update data statistik metrik
		httpRequestsTotal.WithLabelValues(c.Request.Method, path, status).Inc()
		httpRequestDuration.WithLabelValues(c.Request.Method, path).Observe(duration)
	}
}
```

### Step 4: Daftarkan Endpoint `/metrics` di Router Gin
Buka file `cmd/main.go` di masing-masing service, daftarkan middleware dan route prometheus handler:

```go
import "github.com/prometheus/client_golang/prometheus/promhttp"

// ...

func main() {
	r := gin.New()
	
	// Terapkan middleware metrik
	r.Use(middleware.PrometheusMetrics())
	
	// Daftarkan route metrics untuk di-scrape oleh Prometheus server
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	
	// ... routes lainnya ...
}
```

### Step 5: Setup Dashboard di Grafana
1. Buka Grafana (`http://localhost:3000`), masuk dengan `admin`/`admin`.
2. Klik **Connections** > **Data Sources** > **Add data source** > Pilih **Prometheus**.
3. Set URL Prometheus: `http://gowallet-prometheus:9090` (atau `http://prometheus:9090`). Klik **Save & Test**.
4. Buat Dashboard baru: klik **Dashboards** > **New Dashboard** > **Add Visualization**.
5. Pilih Prometheus Data Source, lalu masukkan Query PromQL:
   * **Request Rate (Total request/detik):**
     `rate(http_requests_total[5m])`
   * **Error Rate (Persentase error status 5xx):**
     `sum(rate(http_requests_total{status=~"5.."}[5m])) / sum(rate(http_requests_total[5m])) * 100`
   * **P95 Latency (95% request diselesaikan dalam durasi kurang dari detik):**
     `histogram_quantile(0.95, sum(rate(http_request_duration_seconds_bucket[5m])) by (le))`

---

## ✅ Acceptance Criteria
* [ ] Prometheus dan Grafana berjalan lancar di Docker Compose.
* [ ] Memanggil `GET http://localhost:8082/metrics` mengembalikan data metrik format Prometheus.
* [ ] Prometheus target page (`http://localhost:9090/targets`) menunjukkan status `UP` untuk seluruh microservices.
* [ ] Dashboard Grafana sukses menampilkan grafik realtime yang berfluktuasi saat kita melakukan load-testing API.

---

## 💡 Tips untuk Junior
* **PromQL:** PromQL (Prometheus Query Language) adalah bahasa query untuk memproses metrik time-series. Fungsi `rate` menghitung rata-rata kenaikan per detik dalam rentang waktu tertentu (misal `[5m]` = 5 menit terakhir), sangat ideal untuk mendeteksi lonjakan trafik mendadak.

---

## 📚 Referensi Belajar
* [Prometheus Get Started Guide](https://prometheus.io/docs/introduction/first_steps/)
* [RED Method Monitoring Principles](https://grafana.com/blog/2018/08/02/the-red-method-how-to-instrument-your-services/)
