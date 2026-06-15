# Episode 8: Database Transactions & Optimistic Locking

## 🎯 Tujuan
* Memahami ancaman **Race Condition** dalam transaksi saldo (misal: *double spending*).
* Mengatasi masalah concurrency menggunakan **Optimistic Locking** dengan kolom `version`.
* Mengamankan proses transfer uang antar-wallet menggunakan satu transaksi database utuh (`sql.Tx`).

---

## 🏎️ Race Condition & Solusinya
Bayangkan skenario ini terjadi pada mili-detik yang sama:
1. User A memiliki saldo Rp 100.000.
2. User A melakukan transfer ke User B sebesar Rp 70.000.
3. Di saat yang sama, User A melakukan withdraw Rp 50.000.

Jika server memproses keduanya secara paralel tanpa pengaman concurrency:
* Thread 1 membaca saldo A = 100.000, lalu menguranginya menjadi 30.000.
* Thread 2 membaca saldo A = 100.000 (sebelum Thread 1 selesai menulis), lalu menguranginya menjadi 50.000.
* Hasil akhirnya: Saldo A bisa menjadi 50.000 (atau 30.000), padahal total transaksi A adalah Rp 120.000! Ini adalah kebocoran uang bagi platform kita.

### Mengapa Memilih Optimistic Locking?
Ada dua cara mengunci data di MySQL:
1. **Pessimistic Locking (`SELECT ... FOR UPDATE`):** Mengunci baris database agar tidak bisa dibaca thread lain sampai transaksi selesai. Sangat aman, tapi lambat karena membuat thread lain mengantre (*blocking*).
2. **Optimistic Locking:** Tidak mengunci database saat membaca. Tapi saat menulis, kita mengecek apakah data sudah berubah sejak kita baca. Caranya dengan membandingkan kolom `version`. Ini jauh lebih cepat dan tidak memblokir query lain.

```sql
-- Query Optimistic Locking
UPDATE wallets 
SET balance = balance + ?, version = version + 1 
WHERE id = ? AND version = ?;
```
Jika ada thread lain yang mengupdate data tersebut lebih dulu, `version` di database sudah bertambah. Query kita akan menghasilkan `RowsAffected = 0`. Jika hal itu terjadi, aplikasi kita tahu ada perubahan konkuren, lalu membatalkan transaksi atau melakukan percobaan ulang (*retry*).

---

## 📦 Langkah-langkah

### Step 1: Modifikasi WalletRepository untuk Mendukung Update Tx & Versioning
Buka file `internal/wallet/repository/repository.go`. Tambahkan method `UpdateBalanceTx` ke interface dan implementasinya:

```go
// Tambah di interface WalletRepository:
// UpdateBalanceTx(ctx context.Context, tx *sql.Tx, walletID string, newBalance float64, currentVersion int) error

func (r *mysqlWalletRepository) UpdateBalanceTx(ctx context.Context, tx *sql.Tx, walletID string, newBalance float64, currentVersion int) error {
	query := `UPDATE wallets SET balance = ?, version = version + 1 WHERE id = ? AND version = ?`
	result, err := tx.ExecContext(ctx, query, newBalance, walletID, currentVersion)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	// Jika 0 rows affected, berarti versi database sudah berubah (concurrency conflict)
	if rowsAffected == 0 {
		return errors.New("concurrent update detected: version mismatch")
	}

	return nil
}
```

### Step 2: Buat Folder Domain Transaction
Buat struktur folder baru di dalam `internal/`:
```bash
mkdir -p internal/transaction/model internal/transaction/repository internal/transaction/service internal/transaction/handler
```

### Step 3: Membuat Model Transaction (`internal/transaction/model/tx.go`)
```go
package model

import "time"

type Transaction struct {
	ID               string    `json:"id"`
	SenderWalletID   *string   `json:"sender_wallet_id"` // nullable jika top up
	ReceiverWalletID string    `json:"receiver_wallet_id"`
	Amount           float64   `json:"amount"`
	Description      string    `json:"description"`
	IdempotencyKey   string    `json:"idempotency_key"`
	Status           string    `json:"status"` // success, failed, pending
	CreatedAt        time.Time `json:"created_at"`
}

type TransferRequest struct {
	ReceiverEmail  string  `json:"receiver_email" binding:"required,email"`
	Amount         float64 `json:"amount" binding:"required,gt=0"`
	Description    string  `json:"description"`
	IdempotencyKey string  `json:"idempotency_key" binding:"required"`
}
```

### Step 4: Membuat Repository Transaction (`internal/transaction/repository/repository.go`)
```go
package repository

import (
	"context"
	"database/sql"

	"github.com/emzhofb/gowallet/monolith/internal/transaction/model"
)

type TransactionRepository interface {
	CreateTx(ctx context.Context, tx *sql.Tx, t *model.Transaction) error
	GetByIdempotencyKey(ctx context.Context, key string) (*model.Transaction, error)
}

type mysqlTransactionRepository struct {
	db *sql.DB
}

func NewMySQLTransactionRepository(db *sql.DB) TransactionRepository {
	return &mysqlTransactionRepository{db: db}
}

func (r *mysqlTransactionRepository) CreateTx(ctx context.Context, tx *sql.Tx, t *model.Transaction) error {
	query := `INSERT INTO transactions (id, sender_wallet_id, receiver_wallet_id, amount, description, idempotency_key, status) VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err := tx.ExecContext(ctx, query, t.ID, t.SenderWalletID, t.ReceiverWalletID, t.Amount, t.Description, t.IdempotencyKey, t.Status)
	return err
}

func (r *mysqlTransactionRepository) GetByIdempotencyKey(ctx context.Context, key string) (*model.Transaction, error) {
	query := `SELECT id, sender_wallet_id, receiver_wallet_id, amount, description, idempotency_key, status, created_at FROM transactions WHERE idempotency_key = ?`
	t := &model.Transaction{}
	var sender sql.NullString
	err := r.db.QueryRowContext(ctx, query, key).Scan(&t.ID, &sender, &t.ReceiverWalletID, &t.Amount, &t.Description, &t.IdempotencyKey, &t.Status, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	if sender.Valid {
		t.SenderWalletID = &sender.String
	}
	return t, nil
}
```

### Step 5: Membuat Service Transfer dengan Full Transaction (`internal/transaction/service/service.go`)
Di sini logika bisnis transfer dirangkai:
1. Mulai DB transaction.
2. Cari wallet pengirim & penerima.
3. Validasi saldo pengirim cukup.
4. Kurangi saldo pengirim & tambah saldo penerima (memakai `UpdateBalanceTx` dengan checking version).
5. Buat data transaction record.
6. Buat 2 baris ledger (Debit untuk pengirim, Credit untuk penerima).
7. Commit.

```go
package service

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	customErr "github.com/emzhofb/gowallet/monolith/internal/errors"
	ledgerModel "github.com/emzhofb/gowallet/monolith/internal/ledger/model"
	ledgerRepo "github.com/emzhofb/gowallet/monolith/internal/ledger/repository"
	"github.com/emzhofb/gowallet/monolith/internal/transaction/model"
	"github.com/emzhofb/gowallet/monolith/internal/transaction/repository"
	userRepo "github.com/emzhofb/gowallet/monolith/internal/user/repository"
	walletRepo "github.com/emzhofb/gowallet/monolith/internal/wallet/repository"
	"github.com/google/uuid"
)

type TransactionService interface {
	Transfer(ctx context.Context, senderUserID string, req model.TransferRequest) (*model.Transaction, error)
}

type transactionService struct {
	db          *sql.DB
	txRepo      repository.TransactionRepository
	userRepo    userRepo.UserRepository
	walletRepo  walletRepo.WalletRepository
	ledgerRepo  ledgerRepo.LedgerRepository
}

func NewTransactionService(
	db *sql.DB,
	txRepo repository.TransactionRepository,
	uRepo userRepo.UserRepository,
	wRepo walletRepo.WalletRepository,
	lRepo ledgerRepo.LedgerRepository,
) TransactionService {
	return &transactionService{
		db:          db,
		txRepo:      txRepo,
		userRepo:    uRepo,
		walletRepo:  wRepo,
		ledgerRepo:  lRepo,
	}
}

func (s *transactionService) Transfer(ctx context.Context, senderUserID string, req model.TransferRequest) (*model.Transaction, error) {
	// 1. Cek Idempotency Key (cekan awal agar tidak memproses ulang transaksi lama)
	existing, _ := s.txRepo.GetByIdempotencyKey(ctx, req.IdempotencyKey)
	if existing != nil {
		return existing, nil
	}

	// 2. Cari data penerima berdasarkan email
	receiverUser, err := s.userRepo.GetByEmail(ctx, req.ReceiverEmail)
	if err != nil {
		return nil, customErr.NewAppError(http.StatusNotFound, "RECEIVER_NOT_FOUND", "Email penerima tidak ditemukan.")
	}

	// 3. Mulai Transaksi Database
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, customErr.ErrInternalServer
	}
	defer tx.Rollback()

	// 4. Cari wallet pengirim & penerima
	senderWallet, err := s.walletRepo.GetByUserID(ctx, senderUserID)
	if err != nil {
		return nil, customErr.NewAppError(http.StatusNotFound, "SENDER_WALLET_NOT_FOUND", "Wallet pengirim tidak ditemukan.")
	}

	receiverWallet, err := s.walletRepo.GetByUserID(ctx, receiverUser.ID)
	if err != nil {
		return nil, customErr.NewAppError(http.StatusNotFound, "RECEIVER_WALLET_NOT_FOUND", "Wallet penerima tidak ditemukan.")
	}

	if senderWallet.ID == receiverWallet.ID {
		return nil, customErr.NewAppError(http.StatusBadRequest, "INVALID_TRANSFER", "Tidak dapat mentransfer ke diri sendiri.")
	}

	// 5. Validasi kecukupan saldo
	if senderWallet.Balance < req.Amount {
		return nil, customErr.NewAppError(http.StatusUnprocessableEntity, "INSUFFICIENT_BALANCE", "Saldo Anda tidak mencukupi.")
	}

	// 6. Update saldo dengan Optimistic Locking
	newSenderBalance := senderWallet.Balance - req.Amount
	err = s.walletRepo.UpdateBalanceTx(ctx, tx, senderWallet.ID, newSenderBalance, senderWallet.Version)
	if err != nil {
		// Jika gagal karena version mismatch, return error khusus agar bisa diretry di client
		return nil, customErr.NewAppError(http.StatusConflict, "CONCURRENCY_CONFLICT", "Transaksi sedang sibuk, silakan coba beberapa saat lagi.")
	}

	newReceiverBalance := receiverWallet.Balance + req.Amount
	err = s.walletRepo.UpdateBalanceTx(ctx, tx, receiverWallet.ID, newReceiverBalance, receiverWallet.Version)
	if err != nil {
		return nil, customErr.NewAppError(http.StatusConflict, "CONCURRENCY_CONFLICT", "Transaksi sedang sibuk, silakan coba beberapa saat lagi.")
	}

	// 7. Simpan record transaksi
	transactionID := uuid.New().String()
	transaction := &model.Transaction{
		ID:               transactionID,
		SenderWalletID:   &senderWallet.ID,
		ReceiverWalletID: receiverWallet.ID,
		Amount:           req.Amount,
		Description:      req.Description,
		IdempotencyKey:   req.IdempotencyKey,
		Status:           "success",
	}
	if err := s.txRepo.CreateTx(ctx, tx, transaction); err != nil {
		return nil, customErr.ErrInternalServer
	}

	// 8. Catat Ledger Entry (Debit untuk pengirim, Credit untuk penerima)
	debitEntry := &ledgerModel.LedgerEntry{
		ID:            uuid.New().String(),
		WalletID:      senderWallet.ID,
		TransactionID: transactionID,
		EntryType:     "debit",
		Amount:        req.Amount,
	}
	if err := s.ledgerRepo.CreateTx(ctx, tx, debitEntry); err != nil {
		return nil, customErr.ErrInternalServer
	}

	creditEntry := &ledgerModel.LedgerEntry{
		ID:            uuid.New().String(),
		WalletID:      receiverWallet.ID,
		TransactionID: transactionID,
		EntryType:     "credit",
		Amount:        req.Amount,
	}
	if err := s.ledgerRepo.CreateTx(ctx, tx, creditEntry); err != nil {
		return nil, customErr.ErrInternalServer
	}

	// 9. Commit transaksi
	if err := tx.Commit(); err != nil {
		return nil, customErr.ErrInternalServer
	}

	transaction.CreatedAt = senderWallet.UpdatedAt // Taksiran waktu sukses
	return transaction, nil
}
```

---

## ✅ Acceptance Criteria
* [ ] Mengirim `POST /api/v1/transactions/transfer` berhasil mendebit saldo pengirim dan mengkredit saldo penerima.
* [ ] Detail transaksi tercatat dengan benar di tabel `transactions`.
* [ ] Tabel `ledger_entries` memuat tepat 2 baris baru (debit sender, credit receiver) untuk setiap transfer yang sukses.
* [ ] Jika saldo pengirim kurang dari nominal transfer, transaksi otomatis dibatalkan tanpa mengubah data apapun (Rollback).
* [ ] Mengirim request dengan idempotency key yang sama tidak memicu pemrosesan saldo ulang (menghindari double transfer).

---

## 💡 Tips untuk Junior
* **Idempotency Key:** Kunci idempotensi biasanya digenerate oleh client (misal aplikasi mobile/frontend) berupa UUID unik untuk setiap tombol klik "Kirim". Ini penting karena jika koneksi internet terputus sesaat saat klik kirim, client bisa mencoba mengirim ulang request yang sama dengan key yang sama tanpa takut uang terpotong dua kali.

---

## 📚 Referensi Belajar
* [Optimistic vs Pessimistic Locking](https://www.baeldung.com/cs/optimistic-vs-pessimistic-locking)
* [Designing Idempotent APIs](https://stripe.com/blog/idempotency)
