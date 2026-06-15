# Episode 9: Pagination, Sorting, & Soft Delete

## 🎯 Tujuan
* Mengenalkan konsep **Soft Delete** (menghapus data secara logis tanpa menghapusnya dari disk fisik).
* Membuat fitur **Pagination** dan **Sorting** di endpoint riwayat transaksi agar respons API tetap cepat saat data sudah mencapai ribuan baris.
* Menyusun standard response metadata pagination yang bersih dan informatif.

---

## 🗑️ Kenapa Memakai Soft Delete?
Dalam aplikasi finansial, kita **tidak pernah benar-benar menghapus data penting** menggunakan command `DELETE FROM users`. 
Jika kita menghapus data user secara fisik, bagaimana nasib riwayat transfer yang pernah dikirim/diterima user tersebut? Laporan keuangan kita akan error karena adanya relasi tabel (*foreign key constraint*) yang rusak.

Sebagai solusinya, kita menggunakan kolom `deleted_at` (tipe `TIMESTAMP`).
* Jika `deleted_at IS NULL` ➔ Data dianggap aktif.
* Jika `deleted_at` berisi tanggal ➔ Data dianggap terhapus.
* Semua query `SELECT` data aktif harus menyertakan filter `WHERE deleted_at IS NULL`.

---

## 📦 Langkah-langkah

### Step 1: Modifikasi Repository untuk Soft Delete User
Buka file `internal/user/repository/repository.go`. Tambahkan method `SoftDelete` ke interface dan implementasinya:

```go
// Tambah di interface UserRepository:
// SoftDelete(ctx context.Context, id string) error

func (r *mysqlUserRepository) SoftDelete(ctx context.Context, id string) error {
	// Isi kolom deleted_at dengan waktu saat ini
	query := `UPDATE users SET deleted_at = NOW() WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}
```

### Step 2: Menambahkan Query Params Pagination di Transaction Domain
Kita akan membuat API untuk mengambil riwayat transaksi user. Request ini harus memuat query params:
* `page` (default: 1)
* `limit` (default: 10, max: 100)
* `sort` (default: "created_at")
* `order` (default: "desc")

Buat file baru untuk model pagination di `internal/transaction/model/pagination.go`:

```go
package model

type PaginationParams struct {
	Page  int    `form:"page,default=1"`
	Limit int    `form:"limit,default=10"`
	Sort  string `form:"sort,default=created_at"`
	Order string `form:"order,default=desc"`
}

func (p *PaginationParams) Offset() int {
	return (p.Page - 1) * p.Limit
}

type PaginationMeta struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

type PaginatedResponse struct {
	Success bool            `json:"success"`
	Data    any             `json:"data"`
	Meta    PaginationMeta  `json:"meta"`
}
```

### Step 3: Implementasi Pagination di TransactionRepository
Buka `internal/transaction/repository/repository.go`. Tambahkan method untuk mengambil riwayat transaksi terpaginasi:

```go
// Tambah di interface TransactionRepository:
// GetHistory(ctx context.Context, walletID string, params model.PaginationParams) ([]model.Transaction, int64, error)

func (r *mysqlTransactionRepository) GetHistory(ctx context.Context, walletID string, params model.PaginationParams) ([]model.Transaction, int64, error) {
	// 1. Hitung total data untuk meta pagination
	countQuery := `SELECT COUNT(*) FROM transactions WHERE sender_wallet_id = ? OR receiver_wallet_id = ?`
	var total int64
	err := r.db.QueryRowContext(ctx, countQuery, walletID, walletID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// 2. Ambil data terpaginasi (gunakan sort & order yang aman)
	// Penting: Lakukan whitelist untuk nilai sort & order untuk mencegah SQL Injection via query param!
	sortColumn := "created_at"
	if params.Sort == "amount" {
		sortColumn = "amount"
	}
	
	sortOrder := "DESC"
	if params.Order == "asc" {
		sortOrder = "ASC"
	}

	query := `
		SELECT id, sender_wallet_id, receiver_wallet_id, amount, description, idempotency_key, status, created_at 
		FROM transactions 
		WHERE sender_wallet_id = ? OR receiver_wallet_id = ? 
		ORDER BY ` + sortColumn + ` ` + sortOrder + ` 
		LIMIT ? OFFSET ?`

	rows, err := r.db.QueryContext(ctx, query, walletID, walletID, params.Limit, params.Offset())
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var txs []model.Transaction
	for rows.Next() {
		var t model.Transaction
		var sender sql.NullString
		err := rows.Scan(&t.ID, &sender, &t.ReceiverWalletID, &t.Amount, &t.Description, &t.IdempotencyKey, &t.Status, &t.CreatedAt)
		if err != nil {
			return nil, 0, err
		}
		if sender.Valid {
			t.SenderWalletID = &sender.String
		}
		txs = append(txs, t)
	}

	return txs, total, nil
}
```

### Step 4: Menambahkan Logic di TransactionService & Handler
Edit `internal/transaction/service/service.go`. Tambahkan method GetHistory ke interface & implementasinya:

```go
// Tambah di interface TransactionService:
// GetHistory(ctx context.Context, userID string, params model.PaginationParams) ([]model.Transaction, *model.PaginationMeta, error)

func (s *transactionService) GetHistory(ctx context.Context, userID string, params model.PaginationParams) ([]model.Transaction, *model.PaginationMeta, error) {
	wallet, err := s.walletRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, nil, customErr.NewAppError(http.StatusNotFound, "WALLET_NOT_FOUND", "Wallet tidak ditemukan.")
	}

	// Batasi max limit
	if params.Limit > 100 {
		params.Limit = 100
	}

	txs, total, err := s.txRepo.GetHistory(ctx, wallet.ID, params)
	if err != nil {
		return nil, nil, customErr.ErrInternalServer
	}

	totalPages := int(total / int64(params.Limit))
	if total % int64(params.Limit) != 0 {
		totalPages++
	}

	meta := &model.PaginationMeta{
		Page:       params.Page,
		Limit:      params.Limit,
		Total:      total,
		TotalPages: totalPages,
	}

	return txs, meta, nil
}
```

Buat file handler `internal/transaction/handler/handler.go`:

```go
package handler

import (
	"net/http"

	customErr "github.com/emzhofb/gowallet/monolith/internal/errors"
	"github.com/emzhofb/gowallet/monolith/internal/transaction/model"
	"github.com/emzhofb/gowallet/monolith/internal/transaction/service"
	"github.com/gin-gonic/gin"
)

type TransactionHandler struct {
	svc service.TransactionService
}

func NewTransactionHandler(svc service.TransactionService) *TransactionHandler {
	return &TransactionHandler{svc: svc}
}

func (h *TransactionHandler) Transfer(c *gin.Context) {
	userID, _ := c.Get("user_id")
	var req model.TransferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(customErr.NewAppError(http.StatusBadRequest, "INVALID_INPUT", err.Error()))
		return
	}

	tx, err := h.svc.Transfer(c.Request.Context(), userID.(string), req)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    tx,
	})
}

func (h *TransactionHandler) GetHistory(c *gin.Context) {
	userID, _ := c.Get("user_id")
	var params model.PaginationParams
	if err := c.ShouldBindQuery(&params); err != nil {
		c.Error(customErr.NewAppError(http.StatusBadRequest, "INVALID_INPUT", err.Error()))
		return
	}

	txs, meta, err := h.svc.GetHistory(c.Request.Context(), userID.(string), params)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, model.PaginatedResponse{
		Success: true,
		Data:    txs,
		Meta:    *meta,
	})
}
```

### Step 5: Update `cmd/main.go`
Daftarkan handler transaksi baru di `main.go`:

```go
    // ...
	txRepo := transactionRepository.NewMySQLTransactionRepository(db)
	txSvc := transactionService.NewTransactionService(db, txRepo, uRepo, wRepo, lRepo)
	txHandler := transactionHandler.NewTransactionHandler(txSvc)
    
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.ErrorHandler())
    
	// Routes Protected
	protected := r.Group("/api/v1")
	protected.Use(middleware.AuthMiddleware())
	{
		// ...
		protected.POST("/transactions/transfer", txHandler.Transfer)
		protected.GET("/transactions/history", txHandler.GetHistory)
	}
    // ...
```

---

## ✅ Acceptance Criteria
* [ ] Memanggil API `DELETE /api/v1/users/me` (opsional) sukses memperbarui kolom `deleted_at` dengan timestamp waktu sekarang di database.
* [ ] Endpoint `GET /api/v1/users/:id` tidak lagi menampilkan user yang memiliki data `deleted_at` tidak kosong (filter logic `deleted_at IS NULL` aktif).
* [ ] Mengakses `GET /api/v1/transactions/history?page=2&limit=5` menghasilkan list data transaksi ke-6 sampai ke-10, lengkap dengan metadata `Meta` di response JSON.

---

## 💡 Tips untuk Junior
* **SQL Injection pada Dynamic Order By:** Hati-hati saat menyusun dynamic query seperti `ORDER BY ` + `params.Sort`. Kita tidak boleh memasukkan string query parameter mentah dari user secara bebas ke dalam string query SQL karena bisa dimanfaatkan untuk SQL Injection. Selalu gunakan **whitelist** (validasi ketat bahwa string tersebut hanya boleh bernilai `"created_at"` atau `"amount"`, sisanya ditolak).

---

## 📚 Referensi Belajar
* [What is Soft Delete and why use it](https://www.prisma.io/dataguide/datamodels/soft-deletes)
* [API Design: Pagination and Filtering](https://cloud.google.com/apis/design/design_patterns#list_pagination)
