# Episode 7: Double-Entry Ledger System (Introduction)

## 🎯 Tujuan
* Mengenalkan dasar **Double-Entry Bookkeeping** (pembukuan berpasangan) sederhana.
* Memahami prinsip **Immutability** (data tidak boleh diedit atau dihapus).
* Membuat fungsi repository untuk menyimpan catatan mutasi uang (`ledger_entries`).
* Menghitung dan merekonsiliasi saldo akhir berdasarkan kalkulasi riwayat ledger.

---

## 🏦 Mengapa Saldo Disimpan Sebagai Ledger?
Dalam sistem finansial standar industri, kita tidak boleh hanya mengandalkan kolom `balance` di tabel `wallets` yang bisa di-update berkali-kali (`UPDATE wallets SET balance = balance + 100`). 
* Bagaimana jika data di-hack?
* Bagaimana cara melacak dari mana asal uang Rp 10.000.000 yang tiba-tiba ada di akun user?

Solusinya adalah **Ledger System**. Ledger mencatat setiap peristiwa pergerakan uang:
* Setiap uang masuk dicatat sebagai **Credit** (+).
* Setiap uang keluar dicatat sebagai **Debit** (-).
* Catatan ledger bersifat **immutable** (tidak boleh ada query `UPDATE` atau `DELETE` pada tabel `ledger_entries`).
* Jika ada kesalahan transaksi, kita tidak menghapus baris lama, melainkan membuat baris ledger baru untuk membalikkan nilai tersebut (misalnya, koreksi transaksi).

> **Rumus Utama Finansial:**
> Saldo Wallet Saat Ini = Total Credit - Total Debit

---

## 📦 Langkah-langkah

### Step 1: Membuat Folder Ledger
Buat struktur folder baru di dalam `internal/`:
```bash
mkdir -p internal/ledger/model internal/ledger/repository internal/ledger/service
```

### Step 2: Membuat Model Ledger Entry (`internal/ledger/model/ledger.go`)
```go
package model

import "time"

type LedgerEntry struct {
	ID            string    `json:"id"`
	WalletID      string    `json:"wallet_id"`
	TransactionID string    `json:"transaction_id"`
	EntryType     string    `json:"entry_type"` // credit (+), debit (-)
	Amount        float64   `json:"amount"`
	CreatedAt     time.Time `json:"created_at"`
}
```

### Step 3: Membuat Repository Ledger (`internal/ledger/repository/repository.go`)
Repository ledger memiliki fungsi untuk mencatat baris mutasi baru menggunakan Database Transaction (`*sql.Tx`), mencari riwayat mutasi, dan menghitung total saldo dari ledger.

```go
package repository

import (
	"context"
	"database/sql"

	"github.com/emzhofb/gowallet/monolith/internal/ledger/model"
)

type LedgerRepository interface {
	CreateTx(ctx context.Context, tx *sql.Tx, entry *model.LedgerEntry) error
	GetBalanceByWalletID(ctx context.Context, walletID string) (float64, error)
	GetEntriesByWalletID(ctx context.Context, walletID string) ([]model.LedgerEntry, error)
}

type mysqlLedgerRepository struct {
	db *sql.DB
}

func NewMySQLLedgerRepository(db *sql.DB) LedgerRepository {
	return &mysqlLedgerRepository{db: db}
}

func (r *mysqlLedgerRepository) CreateTx(ctx context.Context, tx *sql.Tx, entry *model.LedgerEntry) error {
	query := `INSERT INTO ledger_entries (id, wallet_id, transaction_id, entry_type, amount) VALUES (?, ?, ?, ?, ?)`
	_, err := tx.ExecContext(ctx, query, entry.ID, entry.WalletID, entry.TransactionID, entry.EntryType, entry.Amount)
	return err
}

func (r *mysqlLedgerRepository) GetBalanceByWalletID(ctx context.Context, walletID string) (float64, error) {
	// Saldo = Sum(Credit) - Sum(Debit)
	query := `
		SELECT 
			COALESCE(SUM(CASE WHEN entry_type = 'credit' THEN amount ELSE 0 END), 0) -
			COALESCE(SUM(CASE WHEN entry_type = 'debit' THEN amount ELSE 0 END), 0)
		FROM ledger_entries 
		WHERE wallet_id = ?`
	
	var balance float64
	err := r.db.QueryRowContext(ctx, query, walletID).Scan(&balance)
	if err != nil {
		return 0, err
	}
	return balance, nil
}

func (r *mysqlLedgerRepository) GetEntriesByWalletID(ctx context.Context, walletID string) ([]model.LedgerEntry, error) {
	query := `SELECT id, wallet_id, transaction_id, entry_type, amount, created_at FROM ledger_entries WHERE wallet_id = ? ORDER BY created_at DESC`
	rows, err := r.db.QueryContext(ctx, query, walletID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []model.LedgerEntry
	for rows.Next() {
		var e model.LedgerEntry
		if err := rows.Scan(&e.ID, &e.WalletID, &e.TransactionID, &e.EntryType, &e.Amount, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, nil
}
```

### Step 4: Membuat Rekonsiliasi Saldo di LedgerService
Rekonsiliasi saldo adalah tugas auditing untuk membandingkan saldo aktual di tabel `wallets` dengan total mutasi di tabel `ledger_entries`. Keduanya **harus sama**. Jika berbeda, berarti terjadi inkonsistensi data serius (misalnya, ada transaksi ilegal atau bug).

Buat file `internal/ledger/service/service.go`:

```go
package service

import (
	"context"
	"errors"

	"github.com/emzhofb/gowallet/monolith/internal/ledger/model"
	"github.com/emzhofb/gowallet/monolith/internal/ledger/repository"
	walletRepo "github.com/emzhofb/gowallet/monolith/internal/wallet/repository"
)

type LedgerService interface {
	ReconcileWalletBalance(ctx context.Context, userID string) (bool, float64, float64, error)
	GetMutationHistory(ctx context.Context, userID string) ([]model.LedgerEntry, error)
}

type ledgerService struct {
	ledgerRepo repository.LedgerRepository
	walletRepo walletRepo.WalletRepository
}

func NewLedgerService(lRepo repository.LedgerRepository, wRepo walletRepo.WalletRepository) LedgerService {
	return &ledgerService{
		ledgerRepo: lRepo,
		walletRepo: wRepo,
	}
}

func (s *ledgerService) ReconcileWalletBalance(ctx context.Context, userID string) (bool, float64, float64, error) {
	// 1. Ambil data wallet user
	wallet, err := s.walletRepo.GetByUserID(ctx, userID)
	if err != nil {
		return false, 0, 0, err
	}

	// 2. Hitung total saldo berdasarkan ledger entries
	calculatedBalance, err := s.ledgerRepo.GetBalanceByWalletID(ctx, wallet.ID)
	if err != nil {
		return false, 0, 0, err
	}

	// 3. Bandingkan
	isConsistent := wallet.Balance == calculatedBalance
	return isConsistent, wallet.Balance, calculatedBalance, nil
}

func (s *ledgerService) GetMutationHistory(ctx context.Context, userID string) ([]model.LedgerEntry, error) {
	wallet, err := s.walletRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.ledgerRepo.GetEntriesByWalletID(ctx, wallet.ID)
}
```

---

## ✅ Acceptance Criteria
* [ ] Tabel `ledger_entries` sukses terhubung di repository.
* [ ] Fungsi `GetBalanceByWalletID` mampu menghitung saldo gabungan debit dan kredit dengan benar.
* [ ] Logika rekonsiliasi sukses membandingkan saldo real-time di tabel wallets dengan akumulasi ledger entries.

---

## 💡 Tips untuk Junior
* **COALESCE:** Perhatikan fungsi `COALESCE` di query SQL SQL. Jika tabel ledger masih kosong (belum ada transaksi), query `SUM` akan mengembalikan nilai `NULL`. Di Go, tipe data `float64` tidak dapat menampung `NULL` dan akan melempar error. `COALESCE(SUM(...), 0)` memastikan nilai default `0` dikembalikan alih-alih `NULL`.

---

## 📚 Referensi Belajar
* [Database Ledger Architecture Pattern](https://www.moderntreasury.com/journal/what-is-a-ledger)
* [Double Entry Bookkeeping Basics](https://www.accountingcoach.com/double-entry-bookkeeping/explanation)
