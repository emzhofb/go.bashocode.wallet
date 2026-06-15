# Episode 4: Auth Lanjutan — Refresh Token Rotation & Google OAuth

## 🎯 Tujuan
- Implement refresh token rotation (security best practice)
- Token reuse detection (anti-theft)
- Google OAuth integration
- Token revocation

## 📝 Prerequisites
- Episode 3 selesai (Auth Service dengan JWT sudah jalan)

---

## 📦 Langkah-langkah

### Step 1: Refresh Token Rotation

Saat client menggunakan refresh token untuk mendapatkan access token baru:

```
Flow:
1. Client kirim POST /auth/refresh dengan refresh_token_A
2. Server validasi refresh_token_A (hash, expiry, not revoked)
3. Server REVOKE refresh_token_A (set revoked = true)
4. Server buat refresh_token_B (baru)
5. Server buat access_token baru
6. Return access_token baru + refresh_token_B ke client
7. Client harus pakai refresh_token_B untuk refresh berikutnya
```

> ⚠️ **Token Reuse Detection:**
> Jika refresh_token_A (yang sudah di-revoke) digunakan lagi, ini indikasi token dicuri!
> 
> ```
> Jika revoked token digunakan:
> 1. REVOKE SEMUA refresh token milik user tersebut
> 2. Return 401 dengan message "Token has been revoked. All sessions terminated."
> 3. Log security event (nanti untuk Audit Service)
> ```

Update method di `auth_service.go`:
```go
func (s *authService) RefreshToken(ctx context.Context, req dto.RefreshTokenRequest) (*dto.AuthResponse, error) {
    // 1. Hash incoming refresh token
    tokenHash := hashToken(req.RefreshToken)
    
    // 2. Cari di database
    storedToken, err := s.tokenRepo.GetByTokenHash(ctx, tokenHash)
    if err != nil || storedToken == nil {
        return nil, ErrInvalidToken
    }
    
    // 3. Cek apakah sudah di-revoke → TOKEN REUSE DETECTION!
    if storedToken.Revoked {
        // BAHAYA: Token sudah pernah di-revoke tapi dipakai lagi
        // Revoke SEMUA token user ini
        s.tokenRepo.RevokeAllByUserID(ctx, storedToken.UserID)
        s.logger.Warn("refresh token reuse detected",
            zap.String("user_id", storedToken.UserID))
        return nil, ErrTokenReuse
    }
    
    // 4. Cek expiry
    if storedToken.ExpiresAt.Before(time.Now()) {
        return nil, ErrTokenExpired
    }
    
    // 5. Revoke token lama
    s.tokenRepo.RevokeByID(ctx, storedToken.ID)
    
    // 6. Get user data
    user, err := s.userRepo.GetByID(ctx, storedToken.UserID)
    // ...
    
    // 7. Generate new tokens
    accessToken, _ := s.generateAccessToken(user)
    newRefreshToken, newHash := s.generateRefreshToken()
    
    // 8. Save new refresh token
    s.tokenRepo.Create(ctx, &model.RefreshToken{
        ID:        uuid.New().String(),
        UserID:    user.ID,
        TokenHash: newHash,
        ExpiresAt: time.Now().Add(s.refreshExp),
    })
    
    // 9. Return
    return &dto.AuthResponse{
        AccessToken:  accessToken,
        RefreshToken: newRefreshToken,
        // ...
    }, nil
}
```

### Step 2: Google OAuth

#### 2a. Setup Google Cloud

1. Buka [Google Cloud Console](https://console.cloud.google.com)
2. Buat project baru (atau pakai existing)
3. Buka **APIs & Services → Credentials**
4. Klik **Create Credentials → OAuth 2.0 Client IDs**
5. Application type: **Web application**
6. Authorized redirect URIs: `http://localhost:8080/api/v1/auth/google/callback`
7. Copy **Client ID** dan **Client Secret** ke `.env`

#### 2b. Endpoints Baru

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/auth/google` | ❌ | Redirect ke Google consent screen |
| GET | `/api/v1/auth/google/callback` | ❌ | Handle callback dari Google |

#### 2c. Implementation

**`auth-service/internal/service/oauth_service.go`:**

```go
// Dependencies:
// go get golang.org/x/oauth2
// go get golang.org/x/oauth2/google

// Google OAuth Config
oauthConfig := &oauth2.Config{
    ClientID:     cfg.GoogleClientID,
    ClientSecret: cfg.GoogleClientSecret,
    RedirectURL:  cfg.GoogleRedirectURL,
    Scopes:       []string{"openid", "email", "profile"},
    Endpoint:     google.Endpoint,
}
```

**Flow: GET `/auth/google`:**
```go
func (h *AuthHandler) GoogleLogin(c *gin.Context) {
    // 1. Generate random state (CSRF protection)
    state := uuid.New().String()
    
    // 2. Simpan state di Redis/cookie (TTL 5 menit)
    // Key: "oauth_state:<state>" Value: "1" TTL: 5m
    
    // 3. Redirect ke Google
    url := oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)
    c.Redirect(http.StatusTemporaryRedirect, url)
}
```

**Flow: GET `/auth/google/callback`:**
```go
func (h *AuthHandler) GoogleCallback(c *gin.Context) {
    // 1. Validasi state (CSRF check)
    state := c.Query("state")
    // Cek state ada di Redis → jika tidak ada, return error
    
    // 2. Exchange code untuk token
    code := c.Query("code")
    token, err := oauthConfig.Exchange(ctx, code)
    
    // 3. Get user info dari Google
    client := oauthConfig.Client(ctx, token)
    resp, _ := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
    // Parse response → { id, email, name, picture }
    
    // 4. Cari user di database by email
    user, _ := userRepo.GetByEmail(ctx, googleUser.Email)
    
    if user == nil {
        // 5a. User belum ada → Create user baru
        user = &model.User{
            ID:         uuid.New().String(),
            Email:      googleUser.Email,
            FullName:   googleUser.Name,
            AvatarURL:  &googleUser.Picture,
            Provider:   "google",
            ProviderID: &googleUser.ID,
            IsVerified: true,   // Google sudah verifikasi email
            Role:       "user",
            PasswordHash: "",   // Tidak ada password untuk Google OAuth
        }
        userRepo.Create(ctx, user)
    } else {
        // 5b. User sudah ada → Login
        // Optional: update avatar dan nama dari Google
    }
    
    // 6. Generate JWT tokens (sama seperti login biasa)
    accessToken, _ := authService.generateAccessToken(user)
    refreshToken, hash := authService.generateRefreshToken()
    // Save refresh token...
    
    // 7. Redirect ke frontend dengan tokens
    // Option A: Redirect dengan token di query params (kurang secure)
    // Option B: Redirect ke frontend, frontend exchange code (lebih secure)
    // Option C: Return JSON response (untuk testing)
    
    c.JSON(http.StatusOK, dto.AuthResponse{
        AccessToken:  accessToken,
        RefreshToken: refreshToken,
        // ...
    })
}
```

### Step 3: Test Manual

```bash
# 1. Test Refresh Token Rotation
# Login → dapat token A
TOKEN_A=$(curl -s -X POST http://localhost:8081/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"password123"}' | jq -r '.data.refresh_token')

# Refresh → dapat token B, token A di-revoke
TOKEN_B=$(curl -s -X POST http://localhost:8081/api/v1/auth/refresh \
  -H "Content-Type: application/json" \
  -d "{\"refresh_token\":\"$TOKEN_A\"}" | jq -r '.data.refresh_token')

# Coba pakai token A lagi → harus GAGAL + semua token di-revoke
curl -X POST http://localhost:8081/api/v1/auth/refresh \
  -H "Content-Type: application/json" \
  -d "{\"refresh_token\":\"$TOKEN_A\"}"
# Expected: 401 "Token has been revoked"

# Token B juga harus invalid sekarang (semua token di-revoke)
curl -X POST http://localhost:8081/api/v1/auth/refresh \
  -H "Content-Type: application/json" \
  -d "{\"refresh_token\":\"$TOKEN_B\"}"
# Expected: 401

# 2. Test Google OAuth
# Buka browser: http://localhost:8081/api/v1/auth/google
# Akan redirect ke Google consent screen
# Setelah approve, akan redirect ke callback
```

---

## ✅ Acceptance Criteria

- [ ] Refresh token rotation: token lama di-revoke, token baru dibuat
- [ ] Token reuse detection: semua token user di-revoke jika token lama dipakai
- [ ] Google OAuth: redirect ke Google berhasil
- [ ] Google OAuth callback: create/login user berhasil
- [ ] User Google OAuth punya `provider = "google"` dan `is_verified = true`
- [ ] State parameter (CSRF) divalidasi pada callback
- [ ] Unit test untuk refresh token rotation
- [ ] Unit test untuk token reuse detection

---

## 💡 Tips & Common Pitfalls

1. **Google OAuth memerlukan HTTPS di production** — Di development, `http://localhost` diizinkan.

2. **Simpan state di Redis, bukan cookie** — Lebih aman karena server-side, dengan TTL pendek (5 menit).

3. **User Google tidak punya password** — Jika user register via Google lalu mau login via email/password, mereka harus set password dulu.

4. **Jangan lupakan CSRF** — Tanpa state parameter validation, attacker bisa forge callback URL.

---

## 📚 Referensi Belajar

- [OAuth 2.0 Simplified](https://aaronparecki.com/oauth-2-simplified/)
- [Refresh Token Rotation](https://auth0.com/docs/secure/tokens/refresh-tokens/refresh-token-rotation)
- [Google OAuth2 in Go](https://developers.google.com/identity/protocols/oauth2/web-server)
