# Episode 6: User Service

## 🎯 Tujuan
- Get/update user profile
- Upload avatar
- Change password
- Soft delete account
- gRPC endpoint untuk internal communication
- RBAC: admin-only endpoints
- Pagination & filtering

## 📝 Prerequisites
- Episode 3-5 selesai (Auth Service lengkap)

---

## 📦 Langkah-langkah

### Step 1: Inisialisasi Service

```bash
mkdir -p user-service/cmd
mkdir -p user-service/internal/{handler,service,repository,model,dto,config,grpc}
mkdir -p user-service/db/migrations

cd user-service
go mod init github.com/emzhofb/gowallet/user-service
cd ..
go work use ./user-service

# Install dependencies
cd user-service
go get github.com/gin-gonic/gin
go get github.com/go-sql-driver/mysql
go get github.com/google/uuid
go get github.com/go-playground/validator/v10
go get go.uber.org/zap
go get google.golang.org/grpc
go get google.golang.org/protobuf
cd ..
```

### Step 2: Proto Definition

Buat `proto/user/user.proto`:

```protobuf
syntax = "proto3";

package user;

option go_package = "github.com/emzhofb/gowallet/proto/user";

service UserService {
    rpc GetUserByID(GetUserByIDRequest) returns (UserResponse);
    rpc GetUserByEmail(GetUserByEmailRequest) returns (UserResponse);
}

message GetUserByIDRequest {
    string id = 1;
}

message GetUserByEmailRequest {
    string email = 1;
}

message UserResponse {
    string id = 1;
    string email = 2;
    string full_name = 3;
    string role = 4;
    bool is_verified = 5;
    string avatar_url = 6;
    string created_at = 7;
}
```

Generate Go code:
```bash
protoc --go_out=. --go-grpc_out=. proto/user/user.proto
```

### Step 3: REST Endpoints

| Method | Path | Auth | RBAC | Description |
|---|---|---|---|---|
| GET | `/api/v1/users/me` | ✅ | Any | Get own profile |
| PUT | `/api/v1/users/me` | ✅ | Any | Update own profile |
| POST | `/api/v1/users/me/avatar` | ✅ | Any | Upload avatar |
| PUT | `/api/v1/users/me/password` | ✅ | Any | Change password |
| DELETE | `/api/v1/users/me` | ✅ | Any | Soft delete account |
| GET | `/api/v1/users` | ✅ | **Admin** | List all users |
| GET | `/api/v1/users/:id` | ✅ | **Admin** | Get user by ID |

### Step 4: DTOs

```go
// Request
type UpdateProfileRequest struct {
    FullName string `json:"full_name" binding:"omitempty,min=2,max=255"`
}

type ChangePasswordRequest struct {
    CurrentPassword string `json:"current_password" binding:"required"`
    NewPassword     string `json:"new_password" binding:"required,min=8,max=72"`
}

// Query parameters untuk list users (admin)
type ListUsersQuery struct {
    Page   int    `form:"page,default=1"`
    Limit  int    `form:"limit,default=10"`
    Sort   string `form:"sort,default=created_at"`
    Order  string `form:"order,default=desc"`     // asc atau desc
    Search string `form:"search"`                  // search by name/email
}

// Response
type UserResponse struct {
    ID         string    `json:"id"`
    Email      string    `json:"email"`
    FullName   string    `json:"full_name"`
    Role       string    `json:"role"`
    IsVerified bool      `json:"is_verified"`
    AvatarURL  *string   `json:"avatar_url"`
    CreatedAt  time.Time `json:"created_at"`
}

type PaginatedResponse struct {
    Data []UserResponse `json:"data"`
    Meta PaginationMeta `json:"meta"`
}

type PaginationMeta struct {
    Page       int `json:"page"`
    Limit      int `json:"limit"`
    Total      int `json:"total"`
    TotalPages int `json:"total_pages"`
}
```

### Step 5: Repository

```go
type UserRepository interface {
    GetByID(ctx context.Context, id string) (*model.User, error)
    GetByEmail(ctx context.Context, email string) (*model.User, error)
    Update(ctx context.Context, user *model.User) error
    UpdatePassword(ctx context.Context, id string, passwordHash string) error
    UpdateAvatar(ctx context.Context, id string, avatarURL string) error
    SoftDelete(ctx context.Context, id string) error
    List(ctx context.Context, query ListUsersQuery) ([]model.User, int, error) // returns users + total count
}
```

**Pagination SQL Pattern:**
```sql
-- Count total
SELECT COUNT(*) FROM users WHERE deleted_at IS NULL AND (full_name LIKE ? OR email LIKE ?);

-- Get paginated data
SELECT id, email, full_name, role, is_verified, avatar_url, created_at
FROM users
WHERE deleted_at IS NULL
  AND (full_name LIKE ? OR email LIKE ?)
ORDER BY created_at DESC  -- dynamic: sort + order
LIMIT ? OFFSET ?;          -- LIMIT = limit, OFFSET = (page-1) * limit
```

### Step 6: RBAC Middleware

Buat middleware untuk role checking:

```go
func RequireRole(roles ...string) gin.HandlerFunc {
    return func(c *gin.Context) {
        // 1. Get role dari context (sudah di-set oleh JWT middleware di Gateway)
        userRole := c.GetString("role")
        
        // 2. Cek apakah role user termasuk dalam roles yang diizinkan
        allowed := false
        for _, r := range roles {
            if userRole == r {
                allowed = true
                break
            }
        }
        
        if !allowed {
            c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
                "success": false,
                "error": gin.H{
                    "code":    "FORBIDDEN",
                    "message": "You don't have permission to access this resource",
                },
            })
            return
        }
        
        c.Next()
    }
}
```

Usage:
```go
// Semua authenticated user
users.GET("/me", handler.GetProfile)

// Admin only
users.GET("/", RequireRole("admin"), handler.ListUsers)
users.GET("/:id", RequireRole("admin"), handler.GetUserByID)
```

### Step 7: Avatar Upload

```go
func (h *UserHandler) UploadAvatar(c *gin.Context) {
    // 1. Parse multipart form
    file, header, err := c.Request.FormFile("avatar")
    if err != nil {
        // Return 400
    }
    defer file.Close()
    
    // 2. Validasi file type
    allowedTypes := map[string]bool{
        "image/jpeg": true,
        "image/png":  true,
        "image/webp": true,
    }
    contentType := header.Header.Get("Content-Type")
    if !allowedTypes[contentType] {
        // Return 400 "Only JPEG, PNG, and WebP images are allowed"
    }
    
    // 3. Validasi file size (max 2MB)
    if header.Size > 2*1024*1024 {
        // Return 400 "File size must not exceed 2MB"
    }
    
    // 4. Generate unique filename
    ext := filepath.Ext(header.Filename)
    filename := fmt.Sprintf("%s%s", uuid.New().String(), ext)
    
    // 5. Save file
    // Development: simpan ke ./uploads/avatars/
    // Production: upload ke S3/GCS
    savePath := filepath.Join("uploads", "avatars", filename)
    if err := c.SaveUploadedFile(header, savePath); err != nil {
        // Return 500
    }
    
    // 6. Update user avatar URL di database
    avatarURL := fmt.Sprintf("/uploads/avatars/%s", filename)
    userService.UpdateAvatar(ctx, userID, avatarURL)
    
    // 7. Return success
}
```

### Step 8: gRPC Server

**`user-service/internal/grpc/server.go`:**

```go
type UserGRPCServer struct {
    pb.UnimplementedUserServiceServer
    userService service.UserService
}

func (s *UserGRPCServer) GetUserByID(ctx context.Context, req *pb.GetUserByIDRequest) (*pb.UserResponse, error) {
    user, err := s.userService.GetByID(ctx, req.Id)
    if err != nil {
        return nil, status.Error(codes.NotFound, "user not found")
    }
    
    return &pb.UserResponse{
        Id:         user.ID,
        Email:      user.Email,
        FullName:   user.FullName,
        Role:       user.Role,
        IsVerified: user.IsVerified,
    }, nil
}
```

Start gRPC server di `main.go` (di goroutine terpisah dari HTTP server):
```go
// HTTP server di port 8082
// gRPC server di port 9082
go func() {
    lis, _ := net.Listen("tcp", ":9082")
    grpcServer := grpc.NewServer()
    pb.RegisterUserServiceServer(grpcServer, grpcHandler)
    grpcServer.Serve(lis)
}()
```

### Step 9: Soft Delete

```go
// Soft delete TIDAK menghapus row dari database
// Hanya set deleted_at = NOW()

// SQL:
// UPDATE users SET deleted_at = NOW() WHERE id = ?

// Semua query lain harus filter:
// WHERE deleted_at IS NULL
```

### Step 10: Test Manual

```bash
# 1. Get profile
curl -H "Authorization: Bearer $TOKEN" http://localhost:8082/api/v1/users/me

# 2. Update profile
curl -X PUT -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  http://localhost:8082/api/v1/users/me \
  -d '{"full_name":"Updated Name"}'

# 3. Upload avatar
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -F "avatar=@/path/to/photo.jpg" \
  http://localhost:8082/api/v1/users/me/avatar

# 4. Change password
curl -X PUT -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  http://localhost:8082/api/v1/users/me/password \
  -d '{"current_password":"password123","new_password":"newpassword123"}'

# 5. List users (admin only)
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
  "http://localhost:8082/api/v1/users?page=1&limit=10&search=john"

# 6. Test gRPC
# Install grpcurl: go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
grpcurl -plaintext -d '{"id":"user-uuid"}' \
  localhost:9082 user.UserService/GetUserByID
```

---

## ✅ Acceptance Criteria

- [ ] Get/update profile bekerja
- [ ] Avatar upload: validasi type (jpg/png/webp) dan size (max 2MB)
- [ ] Change password: validasi password lama sebelum update
- [ ] Soft delete: user.deleted_at terisi, tapi row tidak hilang
- [ ] RBAC: user biasa hanya bisa `/me`, admin bisa list semua
- [ ] Pagination, sorting, filtering bekerja di list users
- [ ] gRPC `GetUserByID` dan `GetUserByEmail` berhasil
- [ ] Swagger docs terupdate
- [ ] Unit test untuk service layer

---

## 💡 Tips & Common Pitfalls

1. **gRPC dan HTTP di port berbeda** — Jangan campur! HTTP: 8082, gRPC: 9082.
2. **Soft delete gotcha** — SEMUA query harus ada `WHERE deleted_at IS NULL`. Ini sering lupa!
3. **Avatar storage** — Di development pakai local folder. Serve static files via Gin: `router.Static("/uploads", "./uploads")`
4. **Pagination** — Validasi `page >= 1` dan `limit <= 100` (jangan biarkan client request unlimited).
5. **Change password** — SELALU validasi current password dulu. Jangan langsung update.

---

## 📚 Referensi Belajar

- [gRPC Go Quickstart](https://grpc.io/docs/languages/go/quickstart/)
- [Gin File Upload](https://gin-gonic.com/docs/examples/upload-file/single-file/)
- [Pagination in Go](https://blog.logrocket.com/pagination-in-go/)
