# Episode 17: Komunikasi Internal gRPC

## 🎯 Tujuan
* Memahami apa itu **gRPC** dan mengapa protokol ini sangat cepat untuk komunikasi antar-service internal (*low latency*).
* Menulis berkas schema **Protocol Buffers** (`.proto`) untuk mendefinisikan interface data User.
* Men-generate kode Go menggunakan `protoc`.
* Membuat **gRPC Server** di `auth-service` (port `:50051`) dan **gRPC Client** di service lain.

---

## 📐 Mengapa gRPC untuk Komunikasi Internal?
Dalam arsitektur microservices, antar-service sering sekali saling memanggil (misal: Wallet Service ingin tahu apakah ID user penerima transfer valid).
Jika kita menggunakan REST API HTTP/1.1 biasa untuk komunikasi internal:
* Payload JSON berukuran besar karena berupa teks biasa (*plain text*).
* Handshake koneksi HTTP berulang-ulang lambat.

Dengan **gRPC (HTTP/2)**:
* Payload dikompresi menjadi biner (*binary format*) yang sangat kecil.
* Koneksi bersifat persistent (*multiplexing* di atas HTTP/2).
* Menggunakan **Protobuf** sebagai kontrak yang ketat (*strongly typed*), mencegah salah parsing tipe data antar-bahasa.

---

## 📦 Langkah-langkah

### Step 1: Install Protobuf Compiler & Go Plugins
Unduh plugin compiler Go untuk protobuf di terminal lokal:

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```
*Pastikan folder `$GOPATH/bin` sudah masuk dalam `$PATH` sistem operasi Anda agar command compiler bisa dieksekusi.*

### Step 2: Membuat Definis Protobuf (`proto/user/user.proto`)
Buat folder baru bernama `proto/user/` di root folder workspace:
```bash
mkdir -p proto/user
```

Buat file `proto/user/user.proto`:

```protobuf
syntax = "proto3";

package user;

option go_package = "github.com/emzhofb/gowallet/auth-service/proto/user";

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
}
```

### Step 3: Compile Proto File
Jalankan compiler `protoc` dari folder root workspace:

```bash
protoc --go_out=. --go-grpc_out=. proto/user/user.proto
```
Perintah ini akan men-generate folder dan file baru bernama `proto/user/user.pb.go` dan `proto/user/user_grpc.pb.go` di dalam folder target `auth-service/proto/user/`.

### Step 4: Membuat gRPC Server di `auth-service`
Kita akan membuat server gRPC yang mendengarkan port `:50051` di `auth-service`.

Buat file baru di `auth-service/internal/user/grpc/server.go`:

```go
package grpc

import (
	"context"
	"net"

	"github.com/emzhofb/gowallet/auth-service/internal/user/repository"
	pb "github.com/emzhofb/gowallet/auth-service/proto/user"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type userGRPCServer struct {
	pb.UnimplementedUserServiceServer
	repo repository.UserRepository
}

func RegisterUserGRPCServer(grpcServer *grpc.Server, repo repository.UserRepository) {
	pb.RegisterUserServiceServer(grpcServer, &userGRPCServer{repo: repo})
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
		Id:       u.ID,
		FullName: u.FullName,
		Email:    u.Email,
	}, nil
}
```

### Step 5: Jalankan gRPC Server di `auth-service/cmd/main.go`
Jalankan server gRPC di latar belakang (*goroutine*) bersamaan dengan HTTP server:

```go
// Di dalam main.go auth-service:
	
	// ... inisialisasi repo & db ...

	// 1. Setup gRPC Server
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen gRPC port: %v", err)
	}
	
	grpcServer := grpc.NewServer()
	userGRPC.RegisterUserGRPCServer(grpcServer, uRepo)

	go func() {
		logger.Log.Info("gRPC Server running on port 50051...")
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve gRPC: %v", err)
		}
	}()

	// 2. Setup HTTP Server Gin di port 8081 ...
```

### Step 6: Contoh Membuat gRPC Client di Service Lain
Di microservice lain (seperti `wallet-service` nanti), kita cukup memanggil gRPC Server menggunakan client berikut:

```go
package main

import (
	"context"
	"log"

	pb "github.com/emzhofb/gowallet/auth-service/proto/user"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// Koneksi ke server gRPC auth-service
	conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Did not connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewUserServiceClient(conn)

	// Panggil method RPC GetUserByID
	resp, err := client.GetUserByID(context.Background(), &pb.GetUserRequest{Id: "user-uuid"})
	if err != nil {
		log.Fatalf("Error calling gRPC: %v", err)
	}

	log.Printf("User retrieved: %s (%s)", resp.GetFullName(), resp.GetEmail())
}
```

---

## ✅ Acceptance Criteria
* [ ] File `user.proto` berhasil di-compile menjadi file `.pb.go`.
* [ ] Server gRPC aktif mendengarkan port `50051` saat `auth-service` dijalankan.
* [ ] Service eksternal dapat melakukan pemanggilan RPC `GetUserByID` dan menerima detail data user sukses.

---

## 💡 Tips untuk Junior
* **Error Propagation in gRPC:** Jangan me-return error biasa dari gRPC. Gunakan package `google.golang.org/grpc/status` (contoh: `status.Errorf(codes.NotFound, ...)`) agar gRPC client di sisi seberang menerima error code standar gRPC (seperti `codes.InvalidArgument`, `codes.NotFound`, `codes.Unauthenticated`) yang mudah di-mapping ke HTTP status code.

---

## 📚 Referensi Belajar
* [gRPC Go Tutorial Quickstart](https://grpc.io/docs/languages/go/quickstart/)
* [Protocol Buffers Language Guide (proto3)](https://protobuf.dev/programming-guides/proto3/)
