# Episode 3: Auth Service — JWT Authentication

## 🎯 Tujuan
- Implement Register, Login, Logout
- JWT Access Token + Refresh Token
- Password hashing dengan bcrypt
- RBAC foundation (user vs admin roles)
- Swagger/OpenAPI documentation
- Database migration
- Unit tests

## 📝 Prerequisites
- Episode 1 & 2 selesai
- Docker Compose running (MySQL, Redis)

---

## 📦 Langkah-langkah

### Step 1: Inisialisasi Service

```bash
mkdir -p auth-service/cmd
mkdir -p auth-service/internal/{handler,service,repository,model,dto,config,middleware}
mkdir -p auth-service/db/migrations
mkdir -p auth-service/docs

cd auth-service
go mod init github.com/emzhofb/gowallet/auth-service
cd ..
go work use ./auth-service

# Install dependencies
cd auth-service
go get github.com/gin-gonic/gin
go get github.com/go-sql-driver/mysql
go get github.com/golang-jwt/jwt/v5
go get golang.org/x/crypto/bcrypt
go get github.com/google/uuid
go get github.com/go-playground/validator/v10
go get go.uber.org/zap
go get github.com/swaggo/swag/cmd/swag
go get github.com/swaggo/gin-swagger
go get github.com/swaggo/files
go get github.com/stretchr/testify
cd ..
```

### Step 2: Database Migration

Buat migration file:

```bash
# Dari root project
migrate create -ext sql -dir auth-service/db/migrations -seq create_users
migrate create -ext sql -dir auth-service/db/migrations -seq create_refresh_tokens
```

**`auth-service/db/migrations/000001_create_users.up.sql`:**
```sql
CREATE TABLE IF NOT EXISTS users (
    id CHAR(36) PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    full_name VARCHAR(255) NOT NULL DEFAULT '',
    role ENUM('user', 'admin') NOT NULL DEFAULT 'user',
    is_verified BOOLEAN NOT NULL DEFAULT FALSE,
    avatar_url VARCHAR(500) DEFAULT NULL,
    provider ENUM('local', 'google') NOT NULL DEFAULT 'local',
    provider_id VARCHAR(255) DEFAULT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL DEFAULT NULL,
    
    INDEX idx_email (email),
    INDEX idx_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

**`auth-service/db/migrations/000001_create_users.down.sql`:**
```sql
DROP TABLE IF EXISTS users;
```

**`auth-service/db/migrations/000002_create_refresh_tokens.up.sql`:**
```sql
CREATE TABLE IF NOT EXISTS refresh_tokens (
    id CHAR(36) PRIMARY KEY,
    user_id CHAR(36) NOT NULL,
    token_hash VARCHAR(255) NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    revoked BOOLEAN NOT NULL DEFAULT FALSE,
    revoked_at TIMESTAMP NULL DEFAULT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    INDEX idx_user_id (user_id),
    INDEX idx_token_hash (token_hash),
    INDEX idx_expires_at (expires_at),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

**`auth-service/db/migrations/000002_create_refresh_tokens.down.sql`:**
```sql
DROP TABLE IF EXISTS refresh_tokens;
```

Jalankan migration:
```bash
make migrate-up s=auth
# atau manual:
migrate -path ./auth-service/db/migrations \
  -database "mysql://gowallet:secret@tcp(localhost:3306)/gowallet" up
```

### Step 3: Model (`auth-service/internal/model/`)

**`user.go`:**
```go
type User struct {
    ID           string     `json:"id"`
    Email        string     `json:"email"`
    PasswordHash string     `json:"-"`              // Jangan pernah expose!
    FullName     string     `json:"full_name"`
    Role         string     `json:"role"`           // "user" atau "admin"
    IsVerified   bool       `json:"is_verified"`
    AvatarURL    *string    `json:"avatar_url"`
    Provider     string     `json:"provider"`       // "local" atau "google"
    ProviderID   *string    `json:"provider_id"`
    CreatedAt    time.Time  `json:"created_at"`
    UpdatedAt    time.Time  `json:"updated_at"`
    DeletedAt    *time.Time `json:"deleted_at,omitempty"`
}
```

**`token.go`:**
```go
type RefreshToken struct {
    ID        string     `json:"id"`
    UserID    string     `json:"user_id"`
    TokenHash string     `json:"-"`
    ExpiresAt time.Time  `json:"expires_at"`
    Revoked   bool       `json:"revoked"`
    RevokedAt *time.Time `json:"revoked_at,omitempty"`
    CreatedAt time.Time  `json:"created_at"`
}
```

### Step 4: DTO (`auth-service/internal/dto/`)

**`request.go`:**
```go
type RegisterRequest struct {
    Email    string `json:"email" binding:"required,email"`
    Password string `json:"password" binding:"required,min=8,max=72"`
    FullName string `json:"full_name" binding:"required,min=2,max=255"`
}

type LoginRequest struct {
    Email    string `json:"email" binding:"required,email"`
    Password string `json:"password" binding:"required"`
}

type RefreshTokenRequest struct {
    RefreshToken string `json:"refresh_token" binding:"required"`
}

type LogoutRequest struct {
    RefreshToken string `json:"refresh_token" binding:"required"`
}
```

**`response.go`:**
```go
type AuthResponse struct {
    AccessToken  string       `json:"access_token"`
    RefreshToken string       `json:"refresh_token"`
    ExpiresIn    int          `json:"expires_in"`    // seconds
    TokenType    string       `json:"token_type"`    // "Bearer"
    User         UserResponse `json:"user"`
}

type UserResponse struct {
    ID         string    `json:"id"`
    Email      string    `json:"email"`
    FullName   string    `json:"full_name"`
    Role       string    `json:"role"`
    IsVerified bool      `json:"is_verified"`
    AvatarURL  *string   `json:"avatar_url"`
    CreatedAt  time.Time `json:"created_at"`
}
```

### Step 5: Repository Layer (`auth-service/internal/repository/`)

**`interface.go`:**
```go
type UserRepository interface {
    Create(ctx context.Context, user *model.User) error
    GetByID(ctx context.Context, id string) (*model.User, error)
    GetByEmail(ctx context.Context, email string) (*model.User, error)
    Update(ctx context.Context, user *model.User) error
    SoftDelete(ctx context.Context, id string) error
}

type RefreshTokenRepository interface {
    Create(ctx context.Context, token *model.RefreshToken) error
    GetByTokenHash(ctx context.Context, tokenHash string) (*model.RefreshToken, error)
    RevokeByID(ctx context.Context, id string) error
    RevokeAllByUserID(ctx context.Context, userID string) error
    DeleteExpired(ctx context.Context) (int64, error)
}
```

**`user_mysql.go`** — Implementasi dengan raw SQL:

```go
type userRepository struct {
    db *sql.DB
}

func NewUserRepository(db *sql.DB) UserRepository {
    return &userRepository{db: db}
}

func (r *userRepository) Create(ctx context.Context, user *model.User) error {
    query := `INSERT INTO users (id, email, password_hash, full_name, role, is_verified, provider) 
              VALUES (?, ?, ?, ?, ?, ?, ?)`
    _, err := r.db.ExecContext(ctx, query,
        user.ID, user.Email, user.PasswordHash, user.FullName,
        user.Role, user.IsVerified, user.Provider,
    )
    return err
}

func (r *userRepository) GetByEmail(ctx context.Context, email string) (*model.User, error) {
    query := `SELECT id, email, password_hash, full_name, role, is_verified, 
              avatar_url, provider, provider_id, created_at, updated_at, deleted_at
              FROM users WHERE email = ? AND deleted_at IS NULL`
    
    user := &model.User{}
    err := r.db.QueryRowContext(ctx, query, email).Scan(
        &user.ID, &user.Email, &user.PasswordHash, &user.FullName,
        &user.Role, &user.IsVerified, &user.AvatarURL, &user.Provider,
        &user.ProviderID, &user.CreatedAt, &user.UpdatedAt, &user.DeletedAt,
    )
    if err == sql.ErrNoRows {
        return nil, nil // User not found
    }
    return user, err
}

// ... implement sisanya (GetByID, Update, SoftDelete)
```

**`refresh_token_mysql.go`** — implementasi serupa untuk RefreshTokenRepository.

### Step 6: Service Layer (`auth-service/internal/service/`)

**`interface.go`:**
```go
type AuthService interface {
    Register(ctx context.Context, req dto.RegisterRequest) (*dto.AuthResponse, error)
    Login(ctx context.Context, req dto.LoginRequest) (*dto.AuthResponse, error)
    RefreshToken(ctx context.Context, req dto.RefreshTokenRequest) (*dto.AuthResponse, error)
    Logout(ctx context.Context, req dto.LogoutRequest) error
}
```

**`auth_service.go`:**

```go
type authService struct {
    userRepo    repository.UserRepository
    tokenRepo   repository.RefreshTokenRepository
    jwtSecret   string
    accessExp   time.Duration
    refreshExp  time.Duration
    logger      *zap.Logger
}

func NewAuthService(
    userRepo repository.UserRepository,
    tokenRepo repository.RefreshTokenRepository,
    jwtSecret string,
    accessExp time.Duration,
    refreshExp time.Duration,
    logger *zap.Logger,
) AuthService {
    return &authService{
        userRepo:   userRepo,
        tokenRepo:  tokenRepo,
        jwtSecret:  jwtSecret,
        accessExp:  accessExp,
        refreshExp: refreshExp,
        logger:     logger,
    }
}
```

#### Register Flow:
```
1. Validasi request (email format, password min 8 char)
2. Cek apakah email sudah terdaftar → jika ya, return error
3. Hash password menggunakan bcrypt (cost = 12)
4. Generate UUID untuk user ID
5. Create user di database (role = "user", is_verified = false)
6. Generate JWT access token
7. Generate refresh token, simpan hash-nya di database
8. Return AuthResponse
```

#### Login Flow:
```
1. Cek email ada di database → jika tidak, return "invalid credentials"
2. Cek apakah user sudah di-soft-delete → jika ya, return error
3. Bandingkan password dengan hash → jika tidak cocok, return "invalid credentials"
   PENTING: Jangan kasih tahu apakah email atau password yang salah (security)
4. Generate JWT access token
5. Generate refresh token, simpan hash-nya di database
6. Return AuthResponse
```

#### JWT Token Generation:
```go
// Access Token Claims:
type JWTClaims struct {
    UserID string `json:"user_id"`
    Email  string `json:"email"`
    Role   string `json:"role"`
    jwt.RegisteredClaims
}

func (s *authService) generateAccessToken(user *model.User) (string, error) {
    claims := JWTClaims{
        UserID: user.ID,
        Email:  user.Email,
        Role:   user.Role,
        RegisteredClaims: jwt.RegisteredClaims{
            ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.accessExp)),
            IssuedAt:  jwt.NewNumericDate(time.Now()),
            Issuer:    "gowallet",
            Subject:   user.ID,
            ID:        uuid.New().String(), // JTI untuk blacklist
        },
    }
    
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString([]byte(s.jwtSecret))
}

// Refresh Token:
// Generate random string (32 bytes → base64)
// Simpan HASH dari token di database (jangan simpan plain text!)
// Return plain text ke client
func (s *authService) generateRefreshToken() (plainToken string, hash string) {
    bytes := make([]byte, 32)
    rand.Read(bytes)
    plainToken = base64.URLEncoding.EncodeToString(bytes)
    
    hasher := sha256.Sum256([]byte(plainToken))
    hash = hex.EncodeToString(hasher[:])
    
    return plainToken, hash
}
```

#### Password Hashing:
```go
import "golang.org/x/crypto/bcrypt"

func HashPassword(password string) (string, error) {
    bytes, err := bcrypt.GenerateFromPassword([]byte(password), 12)
    return string(bytes), err
}

func CheckPassword(password, hash string) bool {
    err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
    return err == nil
}
```

### Step 7: Handler Layer (`auth-service/internal/handler/`)

**`auth_handler.go`:**

```go
type AuthHandler struct {
    authService service.AuthService
    logger      *zap.Logger
}

func NewAuthHandler(authService service.AuthService, logger *zap.Logger) *AuthHandler {
    return &AuthHandler{
        authService: authService,
        logger:      logger,
    }
}

// @Summary Register new user
// @Description Register a new user account
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body dto.RegisterRequest true "Register request"
// @Success 201 {object} dto.AuthResponse
// @Failure 400 {object} errors.ErrorResponse
// @Failure 409 {object} errors.ErrorResponse "Email already exists"
// @Router /api/v1/auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
    var req dto.RegisterRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        // Return 400 Bad Request dengan detail validasi
        return
    }
    
    resp, err := h.authService.Register(c.Request.Context(), req)
    if err != nil {
        // Handle error (409 jika email duplikat, 500 jika internal error)
        return
    }
    
    c.JSON(http.StatusCreated, gin.H{
        "success": true,
        "data":    resp,
    })
}

// Implement: Login, RefreshToken, Logout dengan pattern serupa
```

### Step 8: Entry Point (`cmd/main.go`)

```go
func main() {
    // 1. Load config
    cfg := config.Load()
    
    // 2. Initialize logger (Zap)
    logger := initLogger(cfg.Env)
    defer logger.Sync()
    
    // 3. Connect to MySQL
    db := database.NewMySQLConnection(cfg.MySQL)
    defer db.Close()
    
    // 4. Initialize repositories
    userRepo := repository.NewUserRepository(db)
    tokenRepo := repository.NewRefreshTokenRepository(db)
    
    // 5. Initialize services
    authService := service.NewAuthService(userRepo, tokenRepo, cfg.JWT.Secret, 
        cfg.JWT.AccessExpiry, cfg.JWT.RefreshExpiry, logger)
    
    // 6. Initialize handlers
    authHandler := handler.NewAuthHandler(authService, logger)
    
    // 7. Setup Gin router
    router := gin.Default()
    
    // 8. Register routes
    v1 := router.Group("/api/v1")
    {
        auth := v1.Group("/auth")
        {
            auth.POST("/register", authHandler.Register)
            auth.POST("/login", authHandler.Login)
            auth.POST("/refresh", authHandler.RefreshToken)
            auth.POST("/logout", authHandler.Logout) // Perlu auth middleware
        }
    }
    
    // 9. Health checks
    router.GET("/health", healthHandler.Health)
    router.GET("/ready", healthHandler.Ready)
    router.GET("/live", healthHandler.Live)
    
    // 10. Swagger
    router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
    
    // 11. Start server + graceful shutdown (sama pattern dengan API Gateway)
}
```

### Step 9: Generate Swagger

```bash
cd auth-service
swag init -g cmd/main.go -o docs
```

### Step 10: Unit Tests

Buat test di `auth-service/internal/service/auth_service_test.go`:

Yang harus ditest:
1. **Register**
   - ✅ Register dengan email valid → success
   - ❌ Register dengan email yang sudah ada → error duplicate
   - ❌ Register dengan password terlalu pendek → error validation
   - ❌ Register dengan email format salah → error validation

2. **Login**
   - ✅ Login dengan credentials benar → success, return tokens
   - ❌ Login dengan email tidak terdaftar → error invalid credentials
   - ❌ Login dengan password salah → error invalid credentials
   - ❌ Login dengan user yang sudah di-soft-delete → error

3. **RefreshToken**
   - ✅ Refresh dengan token valid → success, return new tokens
   - ❌ Refresh dengan token expired → error
   - ❌ Refresh dengan token yang sudah di-revoke → error

4. **Logout**
   - ✅ Logout → refresh token di-revoke
   - ❌ Logout dengan token invalid → error

Tips untuk testing:
```go
// Gunakan testify untuk assertions
import "github.com/stretchr/testify/assert"

// Buat mock repository
type mockUserRepo struct {
    users map[string]*model.User
}

// Atau gunakan interface-based mocking
```

```bash
cd auth-service
go test ./internal/service/... -v -cover
```

### Step 11: Test Manual

```bash
# 1. Run Auth Service
cd auth-service
go run cmd/main.go

# 2. Register
curl -X POST http://localhost:8081/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "password123",
    "full_name": "Test User"
  }'

# Expected: 201 Created dengan access_token dan refresh_token

# 3. Login
curl -X POST http://localhost:8081/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "password123"
  }'

# Expected: 200 OK dengan access_token dan refresh_token

# 4. Refresh Token
curl -X POST http://localhost:8081/api/v1/auth/refresh \
  -H "Content-Type: application/json" \
  -d '{
    "refresh_token": "<refresh_token_dari_login>"
  }'

# 5. Decode JWT token
# Copy access_token, paste di https://jwt.io
# Verifikasi claims: user_id, email, role

# 6. Cek Swagger UI
# Buka browser: http://localhost:8081/swagger/index.html
```

### Step 12: Commit

```bash
git add .
git commit -m "feat(ep03): auth service with register, login, JWT, and refresh token"
```

---

## 🔌 API Endpoints

| Method | Path | Auth | Request Body | Response |
|---|---|---|---|---|
| POST | `/api/v1/auth/register` | ❌ | `{email, password, full_name}` | `{access_token, refresh_token, user}` |
| POST | `/api/v1/auth/login` | ❌ | `{email, password}` | `{access_token, refresh_token, user}` |
| POST | `/api/v1/auth/refresh` | ❌ | `{refresh_token}` | `{access_token, refresh_token, user}` |
| POST | `/api/v1/auth/logout` | ✅ | `{refresh_token}` | `{message: "logged out"}` |

---

## ✅ Acceptance Criteria

- [ ] Register → user tersimpan di DB dengan password ter-hash
- [ ] Register → return access token + refresh token
- [ ] Login → validasi credentials → return tokens
- [ ] Login → "invalid credentials" untuk email/password salah (tanpa leak info)
- [ ] Access token berisi claims: user_id, email, role, exp
- [ ] Refresh token hash tersimpan di database
- [ ] Refresh → return access token baru
- [ ] Logout → refresh token di-revoke
- [ ] Role `user` default saat register
- [ ] Swagger UI bisa diakses di `/swagger/index.html`
- [ ] Database migration up/down bekerja
- [ ] Unit test coverage ≥ 70%
- [ ] Password tersimpan sebagai bcrypt hash (bukan plain text!)

---

## 💡 Tips & Common Pitfalls

1. **JANGAN simpan password plain text!** Selalu gunakan bcrypt dengan cost ≥ 12.

2. **JANGAN simpan refresh token plain text!** Hash dengan SHA256 sebelum simpan ke DB. Return plain text ke client.

3. **Error message harus generic untuk login** — "Invalid credentials" bukan "Email not found" atau "Wrong password". Ini mencegah email enumeration attack.

4. **JWT secret harus panjang** — Minimal 32 karakter random. Jangan pakai "secret" di production.

5. **Validasi input di handler, bukan di service** — Handler = validasi format (email valid, password min length). Service = validasi bisnis (email unik, password cocok).

6. **bcrypt cost 12** — Di laptop mungkin lambat (~250ms). Ini intentional — brute force jadi sangat lambat.

---

## 📚 Referensi Belajar

- [JWT Handbook](https://auth0.com/resources/ebooks/jwt-handbook)
- [bcrypt Best Practices](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html)
- [Swaggo Documentation](https://github.com/swaggo/swag)
- [OWASP Authentication Cheatsheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)
