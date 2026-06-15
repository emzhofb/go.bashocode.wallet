# Episode 15: Redis Enhancement

## 🎯 Tujuan
- Enhanced rate limiter (tier-based)
- Cache strategy optimization
- Token blacklist (logout)
- Redis monitoring

## 📝 Prerequisites
- Episode 2 (basic rate limiter) & Episode 7 (basic cache)

---

## 📦 Langkah-langkah

### Step 1: Rate Limiter Tiers

Update API Gateway rate limiter:

| Tier | Limit | Window | Identifier |
|---|---|---|---|
| Anonymous | 30 req | 1 menit | IP address |
| Authenticated | 200 req | 1 menit | User ID |
| Admin | 1000 req | 1 menit | User ID |

```go
func (rl *RateLimiter) Allow(ctx context.Context, identifier string, tier string) (bool, int, error) {
    limits := map[string]int{
        "anonymous":     30,
        "authenticated": 200,
        "admin":         1000,
    }
    
    limit := limits[tier]
    window := time.Minute
    key := fmt.Sprintf("rate_limit:%s:%s:%d", tier, identifier, time.Now().Unix()/60)
    
    count, _ := rl.redis.Incr(ctx, key).Result()
    if count == 1 {
        rl.redis.Expire(ctx, key, window)
    }
    
    remaining := limit - int(count)
    if remaining < 0 {
        remaining = 0
    }
    
    return int(count) <= limit, remaining, nil
}
```

Response headers:
```
X-RateLimit-Limit: 200
X-RateLimit-Remaining: 150
X-RateLimit-Reset: 1705312860
```

### Step 2: Cache Strategy

| Data | Key Pattern | TTL | Invalidation |
|---|---|---|---|
| Wallet balance | `wallet:<user_id>:balance` | 5m | On mutation |
| User profile | `user:<user_id>:profile` | 15m | On update |
| Transaction detail | `tx:<tx_id>` | 1h | Immutable |
| JWT blacklist | `blacklist:<jti>` | Remaining TTL | On logout |

### Step 3: Token Blacklist

Saat user logout atau admin revoke token:

```go
func (s *authService) Logout(ctx context.Context, accessToken string) error {
    // 1. Parse JWT untuk mendapatkan JTI (JWT ID) dan expiry
    claims, _ := parseJWT(accessToken)
    
    // 2. Hitung sisa waktu expiry
    ttl := time.Until(claims.ExpiresAt.Time)
    if ttl <= 0 {
        return nil // Token sudah expired, tidak perlu blacklist
    }
    
    // 3. Tambahkan ke Redis blacklist
    key := fmt.Sprintf("blacklist:%s", claims.ID) // JTI
    s.redis.Set(ctx, key, "1", ttl)
    
    // 4. Revoke refresh token (di database)
    s.tokenRepo.RevokeByUserID(ctx, claims.UserID)
    
    return nil
}

// Di API Gateway JWT middleware:
func (m *JWTMiddleware) Validate(c *gin.Context) {
    // ... parse token ...
    
    // Cek blacklist
    key := fmt.Sprintf("blacklist:%s", claims.ID)
    exists, _ := m.redis.Exists(ctx, key).Result()
    if exists > 0 {
        c.AbortWithStatusJSON(401, gin.H{"error": "token has been revoked"})
        return
    }
    
    // ... continue ...
}
```

### Step 4: Monitor Redis

```bash
# Cek Redis stats
docker exec -it gowallet-redis redis-cli INFO stats

# Monitor commands real-time
docker exec -it gowallet-redis redis-cli MONITOR

# Cek memory usage
docker exec -it gowallet-redis redis-cli INFO memory
```

---

## ✅ Acceptance Criteria

- [ ] Rate limiter tier-based bekerja
- [ ] Rate limit headers ada di response
- [ ] Cache hit rate > 50%
- [ ] Token blacklist bekerja saat logout
- [ ] Blacklisted token ditolak oleh Gateway
- [ ] Cache invalidation konsisten

---

## 📚 Referensi

- [Redis Rate Limiting](https://redis.io/glossary/rate-limiting/)
- [Cache Aside Pattern](https://docs.microsoft.com/en-us/azure/architecture/patterns/cache-aside)
