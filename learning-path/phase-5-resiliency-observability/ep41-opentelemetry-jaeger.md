# Episode41: Distributed Tracing (OpenTelemetry & Jaeger)

## 🎯 Tujuan
* Mengenalkan konsep **Distributed Tracing** untuk memantau perjalanan satu request melewati banyak microservices.
* Menambahkan container **Jaeger** ke Docker Compose sebagai visualization collector.
* Mengimplementasikan SDK **OpenTelemetry** pada aplikasi Go.
* Melacak aliran request mulai dari API Gateway ➔ HTTP Proxy ➔ Wallet Service ➔ gRPC ➔ Auth Service ➔ Database MySQL.

---

## 📐 Konsep Distributed Tracing
Ketika client memanggil `/api/v1/transactions/transfer` dan terjadi error lambat (latency) hingga 5 detik, bagaimana kita tahu service mana yang bermasalah?
* Apakah Gateway yang lambat merouting?
* Apakah Wallet Service yang lambat mengupdate saldo?
* Ataukah gRPC call ke Auth Service memakan waktu lama?

**Distributed Tracing** memecahkan masalah ini dengan menyisipkan metadata pelacak (**Trace ID** dan **Span ID**) di setiap request header (baik HTTP Header maupun gRPC Metadata). 
Setiap kali request berpindah service, Trace ID tersebut disalin dan diteruskan. Semua service melaporkan durasi pemrosesan mereka ke penampung pusat (Jaeger), yang kemudian menyusunnya menjadi diagram visual berupa *waterfall chart*.

```
[ Client Request ] ➔ [ API Gateway ] (Trace-123, Span-A)
                         │ (Proxy HTTP)
                         ▼
                  [ Wallet Service ] (Trace-123, Span-B)
                         │ (gRPC Call)
                         ▼
                  [ Auth Service ]   (Trace-123, Span-C)
```

---

## 📦 Langkah-langkah

### Step 1: Tambahkan Jaeger ke Docker Compose
Buka file `docker-compose.yml` di folder root workspace, tambahkan service `jaeger`:

```yaml
# ... services mysql, redis, rabbitmq, mongodb ...
  jaeger:
    image: jaegertracing/all-in-one:1.55
    container_name: gowallet-jaeger
    ports:
      - "16686:16686" # Port Web UI Jaeger
      - "4317:4317"   # Port OTLP gRPC collector
      - "4318:4318"   # Port OTLP HTTP collector
    restart: always
```
Jalankan container: `docker compose up -d`. Buka dashboard Jaeger di `http://localhost:16686`.

### Step 2: Install Library OpenTelemetry
Di setiap service (Gateway, Auth, Wallet), unduh dependencies berikut:

```bash
go get go.opentelemetry.io/otel
go get go.opentelemetry.io/otel/sdk/trace
go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc
go get go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin
go get go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc
```

### Step 3: Membuat Shared Tracer Helper (`internal/tracing/tracing.go`)
Kita buat fungsi inisialisasi OTel Provider di folder shared package (atau di internal package masing-masing service).

Buat file di `api-gateway/internal/tracing/tracing.go` (lakukan hal yang sama untuk service lainnya):

```go
package tracing

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func InitTracer(serviceName string, collectorAddr string) (*sdktrace.TracerProvider, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Setup exporter untuk mengirim trace ke Jaeger via OTLP gRPC (port 4317)
	conn, err := grpc.DialContext(ctx, collectorAddr, 
		grpc.WithTransportCredentials(insecure.NewCredentials()), 
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, err
	}

	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, err
	}

	// 2. Setup Resource (Metadata Service)
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	if err != nil {
		return nil, err
	}

	// 3. Setup Tracer Provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()), // Catat seluruh traces
	)

	otel.SetTracerProvider(tp)
	// Setup TextMapPropagator agar Trace ID disalin otomatis ke header HTTP/gRPC
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp, nil
}
```

### Step 4: Instrumentasi HTTP Server Gin (Gateway & Services)
Buka router setup di `api-gateway/cmd/main.go` dan `wallet-service/cmd/main.go`. Daftarkan middleware OTel Gin:

```go
	// Inisialisasi Tracer
	tp, err := tracing.InitTracer("api-gateway", "localhost:4317")
	if err == nil {
		defer tp.Shutdown(context.Background())
	}

	r := gin.New()
	r.Use(gin.Recovery())
	
	// Gunakan Middleware OTel Gin
	r.Use(otelgin.Middleware("api-gateway"))
```

### Step 5: Instrumentasi gRPC Server & Client
Untuk meneruskan Trace ID saat gRPC dipanggil:

**Di gRPC Client (`wallet-service/cmd/main.go`):**
```go
	// Tambahkan Interceptor OTel
	conn, err := grpc.Dial("localhost:50051", 
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()), // DI SINI
	)
```

**Di gRPC Server (`auth-service/cmd/main.go`):**
```go
	// Tambahkan Interceptor OTel
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(otelgrpc.UnaryServerInterceptor()), // DI SINI
	)
```

---

## ✅ Acceptance Criteria
* [ ] Container Jaeger berjalan lancar di port `16686`.
* [ ] Melakukan transaksi transfer uang lewat API Gateway.
* [ ] Buka Jaeger UI (`http://localhost:16686`), pilih service `api-gateway` dan klik **Find Traces**.
* [ ] Tampil *Waterfall Span Chart* lengkap yang melacak request dari:
  `api-gateway` ➔ `wallet-service` (HTTP proxy) ➔ gRPC call ke `auth-service` (GetUserByEmail) beserta durasi masing-masing dalam mili-detik.

---

## 💡 Tips untuk Junior
* **Trace Propagation:** OpenTelemetry tidak bekerja secara magis. Kita **harus selalu meneruskan variabel `ctx` (Context)** di setiap parameter fungsi Go (termasuk query database `db.QueryContext(ctx, ...)` dan gRPC call `client.GetUserByID(ctx, ...)`). Jika Anda melewatkan variabel `ctx` dan menggantinya dengan `context.Background()`, rantai tracing akan terputus.

---

## 📚 Referensi Belajar
* [OpenTelemetry Go Documentation](https://opentelemetry.io/docs/languages/go/)
* [Jaeger Tracing official documentation](https://www.jaegertracing.io/)
