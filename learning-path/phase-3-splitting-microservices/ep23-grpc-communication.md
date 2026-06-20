# Episode23: Komunikasi Internal gRPC

## 🎯 Tujuan
* Memahami apa itu **gRPC** dan mengapa protokol ini sangat cepat untuk komunikasi antar-service internal (*low latency*).
* Menulis berkas schema **Protocol Buffers** (`.proto`) untuk mendefinisikan interface data User.
* Men-generate kode Go menggunakan `protoc`.
* Membuat **gRPC Server** di `user-service` (port `:50052`) dan menghubungkannya dengan **gRPC Client** di `auth-service` (port `:8081`).

---

## 📐 Mengapa gRPC untuk Komunikasi Internal?
Dalam arsitektur microservices yang terdekomposisi penuh, Auth Service tidak lagi memiliki akses langsung ke database MySQL tabel `users` untuk mencari data email dan password hash saat user login.
* **Solusi Buruk:** Auth Service menembak REST API User Service. Ini lambat karena overhead HTTP/1.1 teks biasa (JSON).
* **Solusi Baik (gRPC):** Auth Service menghubungi User Service secara internal menggunakan protokol binary gRPC di atas HTTP/2 yang sangat cepat dan hemat bandwidth.

```
[ API Gateway ]
       │  (HTTP)
       ▼
[ Auth Service ] (Port 8081) ➔ (gRPC Call) ➔ [ User Service ] (gRPC Port 50052)
```

---

## 📦 Langkah-langkah

### Step 1: Install Protobuf Compiler & Go Plugins
Unduh plugin compiler Go untuk protobuf di terminal lokal:

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```
*Pastikan folder `$GOPATH/bin` sudah masuk dalam `$PATH` sistem operasi Anda.*

### Step 2: Membuat Definisi Protobuf (`proto/user/user.proto`)
Buat folder baru bernama `proto/user/` di root folder workspace:
```bash
mkdir -p proto/user
```

Buat file `proto/user/user.proto`:

```protobuf
syntax = "proto3";

package user;

option go_package = "github.com/emzhofb/gowallet/user-service/proto/user";

service UserService {
  rpc GetUserByID (GetUserRequest) returns (UserResponse);
  rpc GetUserByEmail (GetUserByEmailRequest) returns (UserResponse);
}

message GetUserRequest {
  string id = 1;
}

message GetUserByEmailRequest {
  string email = 1;
}

message UserResponse {
  string id = 1;
  string full_name = 2;
  string email = 3;
  string password_hash = 4; // Dibutuhkan oleh Auth Service untuk verifikasi login
}
```

### Step 3: Compile Proto File
Jalankan compiler `protoc` dari folder root workspace:

```bash
protoc --go_out=. --go-grpc_out=. proto/user/user.proto
```
Perintah ini akan menghasilkan file `.pb.go` dan `_grpc.pb.go` di dalam folder `user-service/proto/user/`.

### Step 4: Membuat gRPC Server di `user-service`
Kita akan membuat server gRPC yang mendengarkan port `:50052` di `user-service`.

Buat file baru di `user-service/internal/user/grpc/server.go`:

```go
package grpc

import (
	"context"

	"github.com/emzhofb/gowallet/user-service/internal/user/repository"
	pb "github.com/emzhofb/gowallet/user-service/proto/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type userGRPCServer struct {
	pb.UnimplementedUserServiceServer
	repo repository.UserRepository
}

func NewUserGRPCServer(repo repository.UserRepository) pb.UserServiceServer {
	return &userGRPCServer{repo: repo}
}

func (s *userGRPCServer) GetUserByID(ctx context.Context, req *pb.GetUserRequest) (*pb.UserResponse, error) {
	u, err := s.repo.GetByID(ctx, req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "user not found: %v", err)
	}

	return &pb.UserResponse{
		Id:       u.ID,
		FullName: u.FullName,
		Email:    u.Email,
	}, nil
}

func (s *userGRPCServer) GetUserByEmail(ctx context.Context, req *pb.GetUserByEmailRequest) (*pb.UserResponse, error) {
	u, err := s.repo.GetByEmail(ctx, req.GetEmail())
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "user not found: %v", err)
	}

	return &pb.UserResponse{
		Id:           u.ID,
		FullName:     u.FullName,
		Email:        u.Email,
		PasswordHash: u.PasswordHash, // Dikirim secara aman via internal gRPC
	}, nil
}
```

Daftarkan dan jalankan gRPC Server ini di `user-service/cmd/main.go`:
```go
// Di user-service/cmd/main.go:
import (
	"net"
	"google.golang.org/grpc"
	userGRPC "github.com/emzhofb/gowallet/user-service/internal/user/grpc"
	pb "github.com/emzhofb/gowallet/user-service/proto/user"
)

// ... di dalam main() ...
	lis, err := net.Listen("tcp", ":50052")
	if err != nil {
		log.Fatalf("Failed to listen gRPC port: %v", err)
	}
	
	grpcServer := grpc.NewServer()
	pb.RegisterUserServiceServer(grpcServer, userGRPC.NewUserGRPCServer(uRepo))

	go func() {
		logger.Log.Info("User gRPC Server running on port 50052...")
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve gRPC: %v", err)
		}
	}()
```

### Step 5: Membuat gRPC Client di `auth-service`
Sekarang, hubungkan `auth-service` ke gRPC server `user-service` untuk mendapatkan data kredensial saat login.

Ubah file login service di `auth-service/internal/auth/service/service.go`:

```go
package service

import (
	"context"
	"net/http"
	"time"

	"github.com/emzhofb/gowallet/auth-service/internal/auth"
	customErr "github.com/emzhofb/gowallet/auth-service/internal/errors"
	pb "github.com/emzhofb/gowallet/user-service/proto/user" // Import proto dari user-service
	"golang.org/x/crypto/bcrypt"
)

type authService struct {
	userClient pb.UserServiceClient // gRPC Client
}

func NewAuthService(client pb.UserServiceClient) AuthService {
	return &authService{userClient: client}
}

func (s *authService) Login(ctx context.Context, email string, password string) (string, string, error) {
	// Panggil User Service via gRPC
	userResp, err := s.userClient.GetUserByEmail(ctx, &pb.GetUserByEmailRequest{Email: email})
	if err != nil {
		return "", "", customErr.NewAppError(http.StatusUnauthorized, "INVALID_CREDENTIALS", "Email atau password salah.")
	}

	// Bandingkan password hash
	err = bcrypt.CompareHashAndPassword([]byte(userResp.GetPasswordHash()), []byte(password))
	if err != nil {
		return "", "", customErr.NewAppError(http.StatusUnauthorized, "INVALID_CREDENTIALS", "Email atau password salah.")
	}

	// Generate JWT tokens ...
	accessToken, _ := auth.GenerateToken(userResp.GetId(), userResp.GetEmail(), 15*time.Minute)
	refreshToken, _ := auth.GenerateToken(userResp.GetId(), userResp.GetEmail(), 7*24*time.Hour)

	return accessToken, refreshToken, nil
}
```

*Inisialisasi koneksi `grpc.Dial("localhost:50052", ...)` di dalam `auth-service/cmd/main.go` dan suntikkan `userClient` ke `authService` saat startup.*

---

## ✅ Acceptance Criteria
* [ ] File `user.proto` berhasil di-compile menjadi file target `.pb.go` di dalam user-service.
* [ ] Server gRPC `user-service` aktif mendengarkan port `50052`.
* [ ] `auth-service` dapat memanggil RPC `GetUserByEmail` ke `user-service` untuk memverifikasi login user.

---

## 💡 Tips untuk Junior
* **Error Propagation:** Gunakan package `google.golang.org/grpc/status` (seperti `status.Errorf(codes.NotFound, ...)`) di gRPC server agar client dapat menangkap kode error internal dengan presisi.

---

## 📚 Referensi Belajar
* [gRPC Basics - Go](https://grpc.io/docs/languages/go/basics/)
