# Episode11: Unit Testing & Mocking

## 🎯 Tujuan
* Memahami pentingnya **Unit Testing** sejak awal development, terutama sebagai jaminan kode tidak rusak (*quality gate*) saat kita melakukan refactoring besar nanti (memecah monolith ke microservice).
* Mengenal konsep **Mocking** untuk mengisolasi logika bisnis agar tidak menyentuh database asli saat testing dijalankan.
* Menulis unit test pertama menggunakan library **testify** untuk menguji logika registrasi user.

---

## 🔬 Mengapa Perlu Unit Test & Mocking?
Unit test menguji satu unit terkecil kode (misalnya: fungsi `Register` di `userService`) secara cepat dan mandiri.
Namun, fungsi `Register` memanggil `userRepo.GetByEmail` dan `userRepo.Create` yang mengakses database MySQL asli. Jika kita menjalankan test dengan database asli:
* Test akan berjalan lambat.
* Data di database akan kotor.
* Test bisa gagal gara-gara database mati, padahal kode Go kita sebenarnya benar.

Sebagai solusinya, kita membuat **Mock** (objek tiruan) dari `UserRepository` dan `WalletRepository`. Mock ini akan berpura-pura menjadi repository asli dan memberikan respons palsu yang kita tentukan sendiri (misal: pura-pura email sudah terdaftar, atau pura-pura query database sukses).

---

## 📦 Langkah-langkah

### Step 1: Install Testify
Unduh library `testify` untuk asersi dan mocking:
```bash
go get github.com/stretchr/testify
```

### Step 2: Membuat Mock UserRepository (`internal/user/repository/mock_repository.go`)
Kita akan membuat implementasi tiruan dari `UserRepository` yang menggunakan library `mock` bawaan testify.

Buat file baru di `internal/user/repository/mock_repository.go`:

```go
package repository

import (
	"context"
	"database/sql"

	"github.com/emzhofb/gowallet/monolith/internal/user/model"
	"github.com/stretchr/testify/mock"
)

type MockUserRepository struct {
	mock.Mock
}

func (m *MockUserRepository) Create(ctx context.Context, u *model.User) error {
	args := m.Called(ctx, u)
	return args.Error(0)
}

func (m *MockUserRepository) CreateTx(ctx context.Context, tx *sql.Tx, u *model.User) error {
	args := m.Called(ctx, tx, u)
	return args.Error(0)
}

func (m *MockUserRepository) GetByID(ctx context.Context, id string) (*model.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.User), args.Error(1)
}

func (m *MockUserRepository) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.User), args.Error(1)
}

func (m *MockUserRepository) Update(ctx context.Context, u *model.User) error {
	args := m.Called(ctx, u)
	return args.Error(0)
}

func (m *MockUserRepository) SoftDelete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}
```

### Step 3: Membuat Mock WalletRepository (`internal/wallet/repository/mock_repository.go`)
Buat file baru di `internal/wallet/repository/mock_repository.go`:

```go
package repository

import (
	"context"
	"database/sql"

	"github.com/emzhofb/gowallet/monolith/internal/wallet/model"
	"github.com/stretchr/testify/mock"
)

type MockWalletRepository struct {
	mock.Mock
}

func (m *MockWalletRepository) CreateTx(ctx context.Context, tx *sql.Tx, w *model.Wallet) error {
	args := m.Called(ctx, tx, w)
	return args.Error(0)
}

func (m *MockWalletRepository) GetByUserID(ctx context.Context, userID string) (*model.Wallet, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Wallet), args.Error(1)
}

func (m *MockWalletRepository) UpdateBalanceTx(ctx context.Context, tx *sql.Tx, walletID string, newBalance float64, currentVersion int) error {
	args := m.Called(ctx, tx, walletID, newBalance, currentVersion)
	return args.Error(0)
}
```

### Step 4: Menulis Unit Test untuk Register (`internal/user/service/service_test.go`)
Mari buat file testing untuk menguji logic `Register`. Kita akan mengetes dua skenario:
1. **Skenario Sukses:** Email belum terdaftar, penyimpanan ke database sukses.
2. **Skenario Gagal:** Email sudah terdaftar.

Buat file baru di `internal/user/service/service_test.go`:

```go
package service

import (
	"context"
	"errors"
	"testing"

	userModel "github.com/emzhofb/gowallet/monolith/internal/user/model"
	userRepo "github.com/emzhofb/gowallet/monolith/internal/user/repository"
	walletRepo "github.com/emzhofb/gowallet/monolith/internal/wallet/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestRegister_Success(t *testing.T) {
	// 1. Inisialisasi Mock Repositories
	mockUserRepo := new(userRepo.MockUserRepository)
	mockWalletRepo := new(walletRepo.MockWalletRepository)

	// Kita tidak butuh koneksi sql.DB asli untuk test ini, jadi kirim nil saja
	svc := NewUserService(nil, mockUserRepo, mockWalletRepo)

	ctx := context.TODO()
	req := userModel.CreateUserRequest{
		FullName: "Budi Santoso",
		Email:    "budi@example.com",
		Password: "password123",
	}

	// 2. Tentukan Ekspektasi/Perilaku Mock (Stubs)
	// Ketika GetByEmail dipanggil dengan email budi@example.com, kembalikan error "user not found" (menandakan email belum terdaftar)
	mockUserRepo.On("GetByEmail", ctx, "budi@example.com").Return(nil, errors.New("user not found"))
	
	// Mock SQL Transaction (karena register membuka tx, dan kita mengontrol database dengan mock)
	// Kita tidak bisa men-stub db.BeginTx() dengan mudah tanpa database driver mock, tapi untuk simplifikasi unit test logika ini:
	// PENTING: Karena fungsi Register menggunakan s.db.BeginTx(), unit test kita akan melempar panic jika s.db nil.
	// Agar test ini berjalan lancar di tingkat unit, mari kita pastikan database driver di-mock menggunakan library "DATA-DOG/go-sqlmock".
}
```

### 🛠️ Mengatasi Dependency `*sql.DB` menggunakan SQLMock
Untuk mensimulasikan transaksi SQL asli (`db.BeginTx()`), kita butuh library `github.com/DATA-DOG/go-sqlmock`.

```bash
go get github.com/DATA-DOG/go-sqlmock
```

Mari tulis ulang file test `internal/user/service/service_test.go` secara lengkap dan benar menggunakan SQLMock:

```go
package service

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	userModel "github.com/emzhofb/gowallet/monolith/internal/user/model"
	userRepo "github.com/emzhofb/gowallet/monolith/internal/user/repository"
	walletRepo "github.com/emzhofb/gowallet/monolith/internal/wallet/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestRegister_Success(t *testing.T) {
	// 1. Buat SQL Mock
	db, dbMock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	mockUserRepo := new(userRepo.MockUserRepository)
	mockWalletRepo := new(walletRepo.MockWalletRepository)
	svc := NewUserService(db, mockUserRepo, mockWalletRepo)

	ctx := context.TODO()
	req := userModel.CreateUserRequest{
		FullName: "Budi Santoso",
		Email:    "budi@example.com",
		Password: "password123",
	}

	// 2. Ekspektasi SQL Transaction
	dbMock.ExpectBegin()
	dbMock.ExpectCommit()

	// 3. Ekspektasi Behavior Mock Repositories
	mockUserRepo.On("GetByEmail", ctx, "budi@example.com").Return(nil, errors.New("user not found"))
	mockUserRepo.On("CreateTx", ctx, mock.Anything, mock.Anything).Return(nil)
	mockWalletRepo.On("CreateTx", ctx, mock.Anything, mock.Anything).Return(nil)
	
	expectedUser := &userModel.User{
		ID:       "some-uuid",
		FullName: req.FullName,
		Email:    req.Email,
	}
	mockUserRepo.On("GetByID", ctx, mock.Anything).Return(expectedUser, nil)

	// 4. Jalankan Fungsi
	user, err := svc.Register(ctx, req)

	// 5. Verifikasi Hasil
	assert.NoError(t, err)
	assert.NotNil(t, user)
	assert.Equal(t, req.FullName, user.FullName)
	assert.Equal(t, req.Email, user.Email)

	// Pastikan semua mock terpanggil sesuai ekspektasi
	mockUserRepo.AssertExpectations(t)
	mockWalletRepo.AssertExpectations(t)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestRegister_EmailAlreadyExists(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	mockUserRepo := new(userRepo.MockUserRepository)
	mockWalletRepo := new(walletRepo.MockWalletRepository)
	svc := NewUserService(db, mockUserRepo, mockWalletRepo)

	ctx := context.TODO()
	req := userModel.CreateUserRequest{
		FullName: "Budi Santoso",
		Email:    "budi@example.com",
		Password: "password123",
	}

	// Email sudah terdaftar di database
	existingUser := &userModel.User{
		ID:    "existing-uuid",
		Email: "budi@example.com",
	}
	mockUserRepo.On("GetByEmail", ctx, "budi@example.com").Return(existingUser, nil)

	// Jalankan Fungsi
	user, err := svc.Register(ctx, req)

	// Verifikasi: Harus error dan tidak boleh mengembalikan objek user
	assert.Error(t, err)
	assert.Nil(t, user)
	assert.Equal(t, "Email ini sudah terdaftar.", err.Error())
}
```

### Step 5: Menjalankan Test
Jalankan test dari terminal di dalam folder `monolith/`:
```bash
go test -v ./internal/user/service/...
```
Output akan menunjukkan `PASS` untuk kedua skenario test di atas.

---

## ✅ Acceptance Criteria
* [ ] Paket `testify` dan `go-sqlmock` berhasil terinstal dan diimpor.
* [ ] Menulis mock repository terpisah tanpa menyentuh file implementasi database asli.
* [ ] Menjalankan `go test -v ./...` berhasil me-run semua unit test dan menghasilkan status `PASS` 100%.

---

## 💡 Tips untuk Junior
* **Test Isolation:** Jangan biarkan unit test Anda bergantung pada koneksi internet, database Docker, atau file eksternal lainnya. Jika test Anda bergantung pada database eksternal, itu dinamakan **Integration Test**, bukan **Unit Test**.
* **Mocking Interface Only:** Kita hanya bisa men-mock komponen yang memiliki interface (seperti `UserRepository`). Ini salah satu alasan kuat mengapa kita selalu mendefinisikan interface untuk setiap repositori dan service.

---

## 📚 Referensi Belajar
* [stretchr/testify Mock package documentation](https://pkg.go.dev/github.com/stretchr/testify/mock)
* [DATA-DOG/go-sqlmock documentation](https://github.com/DATA-DOG/go-sqlmock)
* [Testing Techniques in Go (Official)](https://go.dev/doc/tutorial/add-a-test)
