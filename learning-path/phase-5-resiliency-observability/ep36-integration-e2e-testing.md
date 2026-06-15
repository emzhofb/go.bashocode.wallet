# Episode 36: Integration & E2E Testing

## 🎯 Tujuan
* Memahami perbedaan **Unit Test** (menggunakan mock) dengan **Integration Test** (menggunakan database/infrastruktur asli).
* Menyiapkan database uji coba (*Test Database*) khusus untuk lingkungan pengujian terisolasi.
* Menulis kode Integration Test di Go yang melakukan koneksi database MySQL nyata untuk memvalidasi query repository.
* Menjalankan E2E API integration test menggunakan client HTTP bawaan Go.

---

## 📐 Mengapa Butuh Integration Test?
Unit Test sangat bagus untuk menguji logika bisnis murni (*logic/validation service*). Namun, Unit Test menggunakan **Mock DB Interface** sehingga tidak bisa mendeteksi error query SQL asli, seperti:
* Typo nama kolom di query string.
* Kesalahan tipe data constraint.
* Perilaku *foreign key relation* dan *transaction rollback* database yang sesungguhnya.

**Integration Test** mengatasi hal ini dengan menjalankan database nyata (biasanya di-run di kontainer docker terpisah) khusus selama sesi testing berlangsung, melakukan migrasi skema, mengeksekusi test case, lalu menghapus data uji coba tersebut (*clean database*).

```
[ Go Test Init ] ➔ Spin up Test DB ➔ Run Migrations ➔ Execute Tests ➔ Clean DB / Tear Down
```

---

## 📦 Langkah-langkah

### Step 1: Setup Database Test terpisah
Jangan pernah menjalankan integration test menggunakan database development lokal Anda karena data testing akan mengotori data kerja harian Anda.
Buat database baru di MySQL Docker kontainer Anda bernama `gowallet_test`:

```sql
CREATE DATABASE gowallet_test;
```

### Step 2: Menulis Helper Setup Testing (`internal/testutil/db.go`)
Kita akan membuat helper yang bertugas melakukan koneksi ke database test, menjalankan migrasi skema, dan mengembalikan instance `*sql.DB` untuk digunakan oleh testing.

Buat file baru di `wallet-service/internal/testutil/db.go`:

```go
package testutil

import (
	"database/sql"
	"log"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func SetupTestDB() (*sql.DB, func()) {
	// 1. Hubungkan ke database test
	dsn := "gowallet_user:gowallet_password@tcp(localhost:3306)/gowallet_test?parseTime=true"
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Failed to open test database: %v", err)
	}

	// 2. Jalankan migrasi schema secara programmatis
	driver, err := mysql.WithInstance(db, &mysql.Config{})
	if err != nil {
		log.Fatalf("Failed to create migration driver: %v", err)
	}

	// Sesuaikan path migrasi dengan lokasi file sql up Anda
	m, err := migrate.NewWithDatabaseInstance(
		"file://../../db/migrations", 
		"mysql", driver,
	)
	if err != nil {
		log.Fatalf("Failed to initialize migrations: %v", err)
	}

	// Jalankan migrasi ke versi terbaru
	_ = m.Up() 

	// 3. Kembalikan instance DB beserta fungsi "cleanup/tear down" untuk membersihkan database
	cleanup := func() {
		_ = m.Down() // Hapus semua tabel setelah test selesai
		db.Close()
	}

	return db, cleanup
}
```

### Step 3: Menulis Kode Integration Test (`repository_test.go`)
Buat file test untuk repository wallet di `wallet-service/internal/wallet/repository/mysql_impl_test.go`. Kita tidak memakai mock di sini, melainkan langsung memanggil fungsi MySQL asli.

```go
package repository

import (
	"context"
	"testing"

	"github.com/emzhofb/gowallet/wallet-service/internal/testutil"
	"github.com/emzhofb/gowallet/wallet-service/internal/wallet/model"
	"github.com/stretchr/testify/assert"
)

func TestMySQLWalletRepository_Integration(t *testing.T) {
	// Setup real test DB
	db, cleanup := testutil.SetupTestDB()
	defer cleanup() // Bersihkan tabel otomatis setelah test case kelar

	repo := NewMySQLWalletRepository(db)
	ctx := context.Background()

	// 1. Uji Coba Create Wallet
	w := &model.Wallet{
		ID:      "test-wallet-id",
		UserID:  "test-user-id",
		Balance: 100000.0,
		Version: 1,
	}

	err := repo.Create(ctx, w)
	assert.NoError(t, err)

	// 2. Uji Coba Get By User ID
	retrieved, err := repo.GetByUserID(ctx, "test-user-id")
	assert.NoError(t, err)
	assert.Equal(t, "test-wallet-id", retrieved.ID)
	assert.Equal(t, 100000.0, retrieved.Balance)

	// 3. Uji Coba Optimistic Locking (Update Balance)
	updated, err := repo.UpdateBalanceWithOwnerCheck(ctx, "test-user-id", -25000.0, 1)
	assert.NoError(t, err)
	assert.Equal(t, 75000.0, updated.Balance)
	assert.Equal(t, int32(2), updated.Version) // Version bertambah +1
}
```

### Step 4: Menjalankan Test
Jalankan test di terminal directory target:
```bash
go test -v ./internal/wallet/repository/...
```

---

## ✅ Acceptance Criteria
* [ ] Helper `SetupTestDB` berhasil menjalankan seluruh berkas migrasi ke database `gowallet_test`.
* [ ] Menjalankan test case sukses memicu operasi tulis-baca ke MySQL nyata dan mengembalikan status test passed (`PASS`).
* [ ] Fungsi `cleanup()` dijalankan di akhir testing untuk mereset kondisi database kembali bersih.

---

## 💡 Tips untuk Junior
* **Test Isolation:** Jangan pernah membiarkan data dari satu test case memengaruhi test case lainnya. Gunakan taktik pembungkusan transaksi database (`tx.Rollback()`) di akhir setiap pengujian unit jika Anda tidak ingin me-recreate seluruh tabel dari nol untuk menghemat waktu eksekusi test.
