# Episode 13: Audit Service

## 🎯 Tujuan
- Mencatat semua aktivitas penting ke MongoDB
- Consume events dari RabbitMQ
- REST endpoints untuk admin query audit logs
- Correlation ID tracking end-to-end

## 📝 Prerequisites
- Episode 11 selesai (RabbitMQ)
- MongoDB container running

---

## 📦 Langkah-langkah

### Step 1: Inisialisasi Service

```bash
mkdir -p audit-service/cmd
mkdir -p audit-service/internal/{handler,service,repository,model,consumer,config}

cd audit-service
go mod init github.com/emzhofb/gowallet/audit-service
cd ..
go work use ./audit-service

cd audit-service
go get go.mongodb.org/mongo-driver/mongo
cd ..
```

### Step 2: MongoDB Collection Schema

```go
// Collection: audit_logs
type AuditLog struct {
    ID            primitive.ObjectID     `bson:"_id,omitempty" json:"id"`
    EventType     string                 `bson:"event_type" json:"event_type"`
    ActorID       string                 `bson:"actor_id" json:"actor_id"`
    ActorRole     string                 `bson:"actor_role" json:"actor_role"`
    TargetType    string                 `bson:"target_type" json:"target_type"`
    TargetID      string                 `bson:"target_id" json:"target_id"`
    Action        string                 `bson:"action" json:"action"`
    Details       map[string]interface{} `bson:"details" json:"details"`
    CorrelationID string                 `bson:"correlation_id" json:"correlation_id"`
    IPAddress     string                 `bson:"ip_address" json:"ip_address"`
    UserAgent     string                 `bson:"user_agent" json:"user_agent"`
    CreatedAt     time.Time              `bson:"created_at" json:"created_at"`
}
```

Event types yang di-consume:
| Event | Audit Record |
|---|---|
| `transfer.completed` | Transfer detail |
| `payment.completed` | Payment detail |
| `wallet.topup` | Top up detail |
| `wallet.frozen` | Admin freeze action |

Selain itu, Auth Service bisa langsung insert via REST endpoint:
| Direct Insert | Audit Record |
|---|---|
| Login success/failure | Login attempt |
| Password change | Security event |
| Google OAuth login | OAuth event |

### Step 3: Repository (MongoDB)

```go
type AuditRepository interface {
    Create(ctx context.Context, log *model.AuditLog) error
    GetByID(ctx context.Context, id string) (*model.AuditLog, error)
    ListByActorID(ctx context.Context, actorID string, page, limit int) ([]model.AuditLog, int64, error)
    ListByEventType(ctx context.Context, eventType string, page, limit int) ([]model.AuditLog, int64, error)
    List(ctx context.Context, filter AuditFilter) ([]model.AuditLog, int64, error)
}

type AuditFilter struct {
    ActorID    string
    EventType  string
    StartDate  *time.Time
    EndDate    *time.Time
    Page       int
    Limit      int
}
```

MongoDB query example:
```go
func (r *auditRepo) List(ctx context.Context, filter AuditFilter) ([]model.AuditLog, int64, error) {
    collection := r.db.Collection("audit_logs")
    
    // Build filter
    mongoFilter := bson.M{}
    if filter.ActorID != "" {
        mongoFilter["actor_id"] = filter.ActorID
    }
    if filter.EventType != "" {
        mongoFilter["event_type"] = filter.EventType
    }
    if filter.StartDate != nil && filter.EndDate != nil {
        mongoFilter["created_at"] = bson.M{
            "$gte": filter.StartDate,
            "$lte": filter.EndDate,
        }
    }
    
    // Count total
    total, _ := collection.CountDocuments(ctx, mongoFilter)
    
    // Find with pagination
    opts := options.Find().
        SetSort(bson.D{{Key: "created_at", Value: -1}}).
        SetSkip(int64((filter.Page - 1) * filter.Limit)).
        SetLimit(int64(filter.Limit))
    
    cursor, _ := collection.Find(ctx, mongoFilter, opts)
    
    var logs []model.AuditLog
    cursor.All(ctx, &logs)
    
    return logs, total, nil
}
```

### Step 4: REST Endpoints (Admin Only)

| Method | Path | Auth | RBAC | Description |
|---|---|---|---|---|
| GET | `/api/v1/audit/logs` | ✅ | Admin | List audit logs |
| GET | `/api/v1/audit/logs/:id` | ✅ | Admin | Detail audit log |
| GET | `/api/v1/audit/users/:id` | ✅ | Admin | Logs per user |
| POST | `/api/v1/audit/logs` | ✅ (internal) | - | Create audit log (called by other services) |

Query parameters untuk list:
```
GET /api/v1/audit/logs?page=1&limit=20&event_type=transfer.completed&start_date=2024-01-01&end_date=2024-01-31
```

### Step 5: Consumer

```go
func (s *AuditService) HandleTransferCompleted(ctx context.Context, body []byte) error {
    var event Event
    json.Unmarshal(body, &event)
    
    return s.auditRepo.Create(ctx, &model.AuditLog{
        EventType:     "transfer.completed",
        ActorID:       event.Data["sender_user_id"].(string),
        ActorRole:     "user",
        TargetType:    "transaction",
        TargetID:      event.Data["transaction_id"].(string),
        Action:        "transfer",
        Details:       event.Data,
        CorrelationID: event.Data["correlation_id"].(string),
        CreatedAt:     time.Now(),
    })
}
```

### Step 6: Create Indexes di MongoDB

```go
// Buat indexes untuk query performance
func createIndexes(collection *mongo.Collection) {
    collection.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
        {Keys: bson.D{{Key: "actor_id", Value: 1}}},
        {Keys: bson.D{{Key: "event_type", Value: 1}}},
        {Keys: bson.D{{Key: "created_at", Value: -1}}},
        {Keys: bson.D{{Key: "correlation_id", Value: 1}}},
    })
}
```

---

## ✅ Acceptance Criteria

- [ ] Events di-consume dan tersimpan di MongoDB
- [ ] Admin bisa query audit logs (paginated, filtered)
- [ ] Correlation ID terlacak dari request awal sampai audit
- [ ] MongoDB indexes terbuat
- [ ] Auth events (login, password change) tercatat

---

## 📚 Referensi

- [MongoDB Go Driver](https://www.mongodb.com/docs/drivers/go/current/)
- [MongoDB Indexes](https://www.mongodb.com/docs/manual/indexes/)
