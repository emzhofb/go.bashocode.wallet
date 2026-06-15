# Episode 11: RabbitMQ & Event-Driven Architecture

## 🎯 Tujuan
- Setup exchange, routing key, dan queues yang proper
- Implement Dead Letter Queue (DLQ)
- Retry queue dengan delay (TTL)
- Consumer acknowledgment (manual ACK)
- Poison message handling

## 📝 Prerequisites
- Episode 10 selesai (Payment Service)
- RabbitMQ container running

---

## 📦 Langkah-langkah

### Step 1: Konsep Event-Driven

```
TANPA event-driven:
  Transaction Service selesai transfer
  → Panggil Notification Service (REST) → tunggu response
  → Panggil Audit Service (REST) → tunggu response
  → Jika salah satu gagal? 😰
  
DENGAN event-driven:
  Transaction Service selesai transfer
  → Publish event "transfer.completed" ke RabbitMQ
  → Notification Service consume → kirim email (async)
  → Audit Service consume → simpan log (async)
  → Jika gagal? Retry queue → DLQ → admin inspect
```

### Step 2: Exchange & Queue Topology

```
┌─────────────────────────────────────────────────────────┐
│                   RabbitMQ                               │
│                                                          │
│  Exchange: gowallet.events (type: topic)                │
│  ┌─────────────────────────────────────────────┐        │
│  │  Routing Key            → Queue              │        │
│  │  ─────────────────────────────────────────── │        │
│  │  transfer.completed     → notif.transfer     │        │
│  │  transfer.completed     → audit.transfer     │        │
│  │  payment.completed      → notif.payment      │        │
│  │  payment.completed      → audit.payment      │        │
│  │  user.registered        → wallet.create      │        │
│  │  user.registered        → notif.welcome      │        │
│  │  auth.password_reset    → notif.reset        │        │
│  │  wallet.topup           → notif.topup        │        │
│  │  wallet.topup           → audit.topup        │        │
│  │  wallet.frozen          → audit.freeze       │        │
│  └─────────────────────────────────────────────┘        │
│                                                          │
│  Exchange: gowallet.retry (type: direct)                │
│  ┌─────────────────────────────────────────────┐        │
│  │  Queue: gowallet.retry.30s                   │        │
│  │    TTL: 30 seconds                            │        │
│  │    Dead Letter Exchange: gowallet.events      │        │
│  │    (setelah TTL, message kembali ke events)   │        │
│  └─────────────────────────────────────────────┘        │
│                                                          │
│  Exchange: gowallet.dlx (type: direct)                  │
│  ┌─────────────────────────────────────────────┐        │
│  │  Queue: gowallet.dead_letters                 │        │
│  │  (untuk manual inspection oleh admin)         │        │
│  └─────────────────────────────────────────────┘        │
└─────────────────────────────────────────────────────────┘
```

### Step 3: Setup Exchanges & Queues

Buat setup script atau init code:

```go
func SetupRabbitMQ(ch *amqp.Channel) error {
    // ═══════════════════════════════════════
    // 1. Declare Exchanges
    // ═══════════════════════════════════════
    
    // Main event exchange (topic)
    ch.ExchangeDeclare("gowallet.events", "topic", true, false, false, false, nil)
    
    // Retry exchange (direct)
    ch.ExchangeDeclare("gowallet.retry", "direct", true, false, false, false, nil)
    
    // Dead letter exchange (direct)
    ch.ExchangeDeclare("gowallet.dlx", "direct", true, false, false, false, nil)
    
    // ═══════════════════════════════════════
    // 2. Declare Queues
    // ═══════════════════════════════════════
    
    // Notification queues
    declareQueue(ch, "notif.transfer", "gowallet.events", "transfer.completed")
    declareQueue(ch, "notif.payment", "gowallet.events", "payment.completed")
    declareQueue(ch, "notif.welcome", "gowallet.events", "user.registered")
    declareQueue(ch, "notif.reset", "gowallet.events", "auth.password_reset")
    declareQueue(ch, "notif.topup", "gowallet.events", "wallet.topup")
    
    // Audit queues
    declareQueue(ch, "audit.transfer", "gowallet.events", "transfer.completed")
    declareQueue(ch, "audit.payment", "gowallet.events", "payment.completed")
    declareQueue(ch, "audit.topup", "gowallet.events", "wallet.topup")
    declareQueue(ch, "audit.freeze", "gowallet.events", "wallet.frozen")
    
    // Wallet queue (auto-create wallet on user registered)
    declareQueue(ch, "wallet.create", "gowallet.events", "user.registered")
    
    // Retry queue (TTL 30 seconds → dead letter back to main exchange)
    ch.QueueDeclare("gowallet.retry.30s", true, false, false, false, amqp.Table{
        "x-message-ttl":             int32(30000), // 30 seconds
        "x-dead-letter-exchange":    "gowallet.events",
    })
    ch.QueueBind("gowallet.retry.30s", "retry", "gowallet.retry", false, nil)
    
    // Dead letter queue
    ch.QueueDeclare("gowallet.dead_letters", true, false, false, false, nil)
    ch.QueueBind("gowallet.dead_letters", "dead", "gowallet.dlx", false, nil)
    
    return nil
}

func declareQueue(ch *amqp.Channel, queueName, exchange, routingKey string) {
    ch.QueueDeclare(queueName, true, false, false, false, nil)
    ch.QueueBind(queueName, routingKey, exchange, false, nil)
}
```

### Step 4: Publisher (Update shared pkg/rabbitmq)

```go
type Publisher struct {
    channel *amqp.Channel
    exchange string
}

func (p *Publisher) Publish(ctx context.Context, routingKey string, body []byte) error {
    return p.channel.PublishWithContext(ctx,
        p.exchange,  // exchange
        routingKey,  // routing key
        false,       // mandatory
        false,       // immediate
        amqp.Publishing{
            ContentType:  "application/json",
            Body:         body,
            DeliveryMode: amqp.Persistent, // Message survives broker restart
            Timestamp:    time.Now(),
            MessageId:    uuid.New().String(),
            Headers: amqp.Table{
                "x-retry-count": int32(0),
            },
        },
    )
}
```

### Step 5: Consumer Pattern

```go
type MessageHandler func(ctx context.Context, body []byte) error

type Consumer struct {
    channel    *amqp.Channel
    queueName  string
    handler    MessageHandler
    maxRetries int
    logger     *zap.Logger
}

func (c *Consumer) Start(ctx context.Context) {
    msgs, _ := c.channel.Consume(
        c.queueName,
        "",     // consumer tag (auto-generated)
        false,  // auto-ack = FALSE (manual ACK!)
        false,  // exclusive
        false,  // no-local
        false,  // no-wait
        nil,
    )
    
    for {
        select {
        case <-ctx.Done():
            return
        case msg, ok := <-msgs:
            if !ok {
                return
            }
            c.processMessage(ctx, msg)
        }
    }
}

func (c *Consumer) processMessage(ctx context.Context, msg amqp.Delivery) {
    // 1. Get retry count from headers
    retryCount := int32(0)
    if val, ok := msg.Headers["x-retry-count"]; ok {
        retryCount = val.(int32)
    }
    
    c.logger.Info("processing message",
        zap.String("queue", c.queueName),
        zap.String("message_id", msg.MessageId),
        zap.Int32("retry_count", retryCount),
    )
    
    // 2. Process message
    err := c.handler(ctx, msg.Body)
    
    if err == nil {
        // 3a. Success → ACK
        msg.Ack(false)
        c.logger.Info("message processed successfully",
            zap.String("message_id", msg.MessageId))
        return
    }
    
    c.logger.Error("failed to process message",
        zap.String("message_id", msg.MessageId),
        zap.Error(err),
    )
    
    // 3b. Failed → check retry count
    if retryCount >= int32(c.maxRetries) {
        // Max retries exceeded → send to DLQ
        c.sendToDLQ(msg, err)
        msg.Ack(false) // ACK original (karena sudah masuk DLQ)
        return
    }
    
    // 3c. Send to retry queue (akan kembali setelah TTL)
    c.sendToRetry(msg, retryCount+1)
    msg.Ack(false) // ACK original
}

func (c *Consumer) sendToRetry(msg amqp.Delivery, retryCount int32) {
    c.channel.Publish(
        "gowallet.retry",
        "retry",
        false, false,
        amqp.Publishing{
            ContentType:  msg.ContentType,
            Body:         msg.Body,
            DeliveryMode: amqp.Persistent,
            Headers: amqp.Table{
                "x-retry-count":  retryCount,
                "x-original-queue": c.queueName,
                "x-original-routing-key": msg.RoutingKey,
            },
        },
    )
}

func (c *Consumer) sendToDLQ(msg amqp.Delivery, err error) {
    c.channel.Publish(
        "gowallet.dlx",
        "dead",
        false, false,
        amqp.Publishing{
            ContentType:  msg.ContentType,
            Body:         msg.Body,
            DeliveryMode: amqp.Persistent,
            Headers: amqp.Table{
                "x-original-queue":      c.queueName,
                "x-original-routing-key": msg.RoutingKey,
                "x-retry-count":         msg.Headers["x-retry-count"],
                "x-error":               err.Error(),
                "x-dead-at":             time.Now().Format(time.RFC3339),
            },
        },
    )
    c.logger.Warn("message sent to DLQ",
        zap.String("message_id", msg.MessageId),
        zap.Error(err))
}
```

### Step 6: Event Payload Standard

```go
// Semua event harus punya format yang konsisten
type Event struct {
    EventID   string                 `json:"event_id"`
    EventType string                 `json:"event_type"`
    Timestamp string                 `json:"timestamp"`
    Data      map[string]interface{} `json:"data"`
}

// Contoh:
// {
//   "event_id": "uuid",
//   "event_type": "transfer.completed",
//   "timestamp": "2024-01-15T10:30:00Z",
//   "data": {
//     "transaction_id": "uuid",
//     "sender_user_id": "uuid",
//     "receiver_user_id": "uuid",
//     "amount": 50000,
//     "description": "Bayar makan"
//   }
// }
```

### Step 7: Test

```bash
# 1. Pastikan RabbitMQ running
docker compose ps | grep rabbitmq

# 2. Buka RabbitMQ Management UI
# http://localhost:15672 (gowallet / secret)

# 3. Cek exchanges dan queues sudah terbuat
# Exchanges tab → gowallet.events, gowallet.retry, gowallet.dlx
# Queues tab → notif.transfer, audit.transfer, dll

# 4. Trigger event (via transfer)
# Lakukan transfer → cek di RabbitMQ UI apakah message masuk ke queue

# 5. Test DLQ
# Buat consumer yang selalu error → message harus masuk retry → DLQ
```

---

## ✅ Acceptance Criteria

- [ ] Exchanges terbuat: `gowallet.events`, `gowallet.retry`, `gowallet.dlx`
- [ ] Queues terbuat sesuai topology
- [ ] Publisher berhasil publish event
- [ ] Consumer menerima dan process event
- [ ] Manual ACK bekerja (bukan auto-ack)
- [ ] Failed message → retry queue (30s delay) → kembali ke queue asal
- [ ] Setelah max retry → masuk DLQ
- [ ] DLQ bisa diinspect via RabbitMQ Management UI
- [ ] Event payload format konsisten

---

## 💡 Tips

1. **Manual ACK wajib** — Auto-ack = message hilang jika consumer crash sebelum selesai proses
2. **Persistent messages** — Set `DeliveryMode: 2` agar message survive broker restart
3. **Consumer harus idempotent** — Message bisa datang lebih dari sekali
4. **Monitor DLQ** — DLQ yang penuh = ada masalah yang harus diinvestigasi

---

## 📚 Referensi Belajar

- [RabbitMQ Tutorials](https://www.rabbitmq.com/tutorials)
- [Dead Letter Exchanges](https://www.rabbitmq.com/dlx.html)
- [Message Acknowledgment](https://www.rabbitmq.com/confirms.html)
