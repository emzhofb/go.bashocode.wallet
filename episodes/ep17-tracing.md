# Episode 17: OpenTelemetry & Jaeger (Distributed Tracing)

## 🎯 Tujuan
- Implement distributed tracing across all services
- Trace request dari Gateway sampai database
- Visualisasi trace di Jaeger UI

## 📝 Prerequisites
- Jaeger container running
- Multiple services berkomunikasi via gRPC/REST

---

## 📦 Langkah-langkah

### Step 1: Install Dependencies

```bash
# Di setiap service:
go get go.opentelemetry.io/otel
go get go.opentelemetry.io/otel/sdk/trace
go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp
go get go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin
go get go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc
```

### Step 2: Tracing Setup (`pkg/tracing/`)

```go
func InitTracer(serviceName string, jaegerEndpoint string) (*sdktrace.TracerProvider, error) {
    exporter, _ := otlptracehttp.New(context.Background(),
        otlptracehttp.WithEndpoint(jaegerEndpoint),
        otlptracehttp.WithInsecure(),
    )
    
    tp := sdktrace.NewTracerProvider(
        sdktrace.WithBatcher(exporter),
        sdktrace.WithResource(resource.NewWithAttributes(
            semconv.SchemaURL,
            semconv.ServiceName(serviceName),
            attribute.String("environment", "development"),
        )),
    )
    
    otel.SetTracerProvider(tp)
    otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
        propagation.TraceContext{},
        propagation.Baggage{},
    ))
    
    return tp, nil
}
```

### Step 3: Instrument Gin (HTTP)

```go
// Di setiap service yang pakai Gin:
router := gin.Default()
router.Use(otelgin.Middleware("api-gateway"))
```

### Step 4: Instrument gRPC

```go
// gRPC Server
grpcServer := grpc.NewServer(
    grpc.UnaryInterceptor(otelgrpc.UnaryServerInterceptor()),
)

// gRPC Client
conn, _ := grpc.Dial(addr,
    grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
)
```

### Step 5: Instrument Database

```go
// Custom span untuk database queries
tracer := otel.Tracer("database")

func (r *repo) GetByID(ctx context.Context, id string) (*model.User, error) {
    ctx, span := tracer.Start(ctx, "UserRepo.GetByID",
        trace.WithAttributes(attribute.String("user.id", id)),
    )
    defer span.End()
    
    // ... query database ...
    
    if err != nil {
        span.SetStatus(codes.Error, err.Error())
        span.RecordError(err)
    }
    
    return user, err
}
```

### Step 6: Instrument Redis & RabbitMQ

```go
// Redis
ctx, span := tracer.Start(ctx, "Redis.Get",
    trace.WithAttributes(attribute.String("redis.key", key)),
)
defer span.End()
result := redis.Get(ctx, key)

// RabbitMQ Publish
ctx, span := tracer.Start(ctx, "RabbitMQ.Publish",
    trace.WithAttributes(
        attribute.String("messaging.system", "rabbitmq"),
        attribute.String("messaging.destination", routingKey),
    ),
)
defer span.End()
```

### Step 7: Correlation ID = Trace ID

Unify correlation ID dengan trace ID:
```go
func CorrelationIDMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        span := trace.SpanFromContext(c.Request.Context())
        traceID := span.SpanContext().TraceID().String()
        
        c.Set("correlation_id", traceID)
        c.Header("X-Correlation-ID", traceID)
        
        c.Next()
    }
}
```

### Step 8: Verify di Jaeger

```bash
# 1. Buka Jaeger UI: http://localhost:16686

# 2. Trigger transfer request

# 3. Di Jaeger UI:
#    - Select service: "api-gateway"
#    - Find Traces
#    - Klik trace → lihat semua spans dari gateway sampai database
```

---

## ✅ Acceptance Criteria

- [ ] Trace ID propagated across all services
- [ ] Jaeger UI menampilkan end-to-end trace
- [ ] Spans untuk: HTTP, gRPC, database, Redis, RabbitMQ
- [ ] Correlation ID = Trace ID
- [ ] Error spans ditandai dengan warna merah di Jaeger

---

## 📚 Referensi

- [OpenTelemetry Go](https://opentelemetry.io/docs/languages/go/)
- [Jaeger Documentation](https://www.jaegertracing.io/docs/)
