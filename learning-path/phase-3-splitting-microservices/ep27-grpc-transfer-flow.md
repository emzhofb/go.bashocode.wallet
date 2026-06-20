# Episode27: gRPC Communication for Transfer Flow

## 🎯 Tujuan
* Menghubungkan **Transaction Service** sebagai gRPC Client ke tiga target gRPC Server:
  * **User Service** (port `:50052`)
  * **Wallet Service** (port `:50053`)
  * **Ledger Service** (port `:50054`)
* Mengimplementasikan alur orkestrasi transaksi transfer saldo terdistribusi secara lengkap menggunakan gRPC calls.

---

## 📐 Alur Transaksi Terdistribusi via gRPC
Saat pengguna memicu API `/api/v1/transactions/transfer`, Transaction Service akan mengoordinasikan seluruh langkah mutasi secara berurutan:

```
[Client] ➔ API Gateway ➔ [Transaction Svc]
                            │
                            ├─ 1. GetUserByEmail ➔ [User Svc]
                            ├─ 2. Check & Update Balances ➔ [Wallet Svc]
                            └─ 3. Record Debits & Credits ➔ [Ledger Svc]
```

---

## 📦 Langkah-langkah

### Step 1: Daftarkan gRPC Clients di `transactionService`
Edit file `transaction-service/internal/transaction/service/service.go`. Tambahkan ketiga gRPC clients ke dalam struct service kita:

```go
package service

import (
	"context"
	"database/sql"
	"net/http"

	customErr "github.com/emzhofb/gowallet/transaction-service/internal/errors"
	"github.com/emzhofb/gowallet/transaction-service/internal/transaction/model"
	"github.com/emzhofb/gowallet/transaction-service/internal/transaction/repository"
	userPb "github.com/emzhofb/gowallet/user-service/proto/user"
	walletPb "github.com/emzhofb/gowallet/wallet-service/proto/wallet"
	ledgerPb "github.com/emzhofb/gowallet/ledger-service/proto/ledger"
	"github.com/google/uuid"
)

type transactionService struct {
	db           *sql.DB
	txRepo       repository.TransactionRepository
	userClient   userPb.UserServiceClient
	walletClient walletPb.WalletServiceClient
	ledgerClient ledgerPb.LedgerServiceClient
}

func NewTransactionService(
	db *sql.DB,
	txRepo repository.TransactionRepository,
	uClient userPb.UserServiceClient,
	wClient walletPb.WalletServiceClient,
	lClient ledgerPb.LedgerServiceClient,
) TransactionService {
	return &transactionService{
		db:           db,
		txRepo:       txRepo,
		userClient:   uClient,
		walletClient: wClient,
		ledgerClient: lClient,
	}
}
```

### Step 2: Implementasi Ulang Metode Transfer
Metode `Transfer` sekarang memanggil gRPC Server alih-alih melakukan query SQL database lokal:

```go
func (s *transactionService) Transfer(ctx context.Context, senderUserID string, req model.TransferRequest) (*model.Transaction, error) {
	// 1. Cek Idempotency Key (keamanan transaksi ganda)
	existing, _ := s.txRepo.GetByIdempotencyKey(ctx, req.IdempotencyKey)
	if existing != nil {
		return existing, nil
	}

	// 2. Cari & Validasi User Penerima via User Service gRPC
	receiverUser, err := s.userClient.GetUserByEmail(ctx, &userPb.GetUserByEmailRequest{Email: req.ReceiverEmail})
	if err != nil {
		return nil, customErr.NewAppError(http.StatusNotFound, "RECEIVER_NOT_FOUND", "Penerima tidak ditemukan.")
	}

	// 3. Ambil Detail Dompet Pengirim & Penerima via Wallet Service gRPC
	senderWallet, err := s.walletClient.GetWalletByUserID(ctx, &walletPb.GetWalletRequest{UserId: senderUserID})
	if err != nil {
		return nil, customErr.NewAppError(http.StatusNotFound, "SENDER_WALLET_NOT_FOUND", "Dompet pengirim tidak ditemukan.")
	}

	if senderWallet.GetBalance() < req.Amount {
		return nil, customErr.NewAppError(http.StatusBadRequest, "INSUFFICIENT_BALANCE", "Saldo tidak mencukupi.")
	}

	// 4. Buat record transaksi dengan status PENDING
	txID := uuid.New().String()
	txRecord := &model.Transaction{
		ID:               txID,
		SenderWalletID:   &senderWallet.Id,
		ReceiverWalletID: receiverUser.GetId(), // Menggunakan ID User sebagai WalletID tujuan
		Amount:           req.Amount,
		Description:      req.Description,
		IdempotencyKey:   req.IdempotencyKey,
		Status:           "PENDING",
	}
	s.txRepo.Create(ctx, txRecord)

	// 5. Kurangi Saldo Pengirim (Debet) via Wallet Service gRPC
	_, err = s.walletClient.UpdateWalletBalance(ctx, &walletPb.UpdateBalanceRequest{
		UserId:          senderUserID,
		Amount:          -req.Amount,
		ExpectedVersion: senderWallet.GetVersion(),
	})
	if err != nil {
		s.txRepo.UpdateStatus(ctx, txID, "FAILED")
		return nil, customErr.NewAppError(http.StatusConflict, "CONCURRENT_ERROR", "Gagal memproses transaksi. Coba lagi.")
	}

	// 6. Tambahkan Saldo Penerima (Kredit) via Wallet Service gRPC
	receiverWallet, err := s.walletClient.GetWalletByUserID(ctx, &walletPb.GetWalletRequest{UserId: receiverUser.GetId()})
	if err != nil {
		// Kompensasi: kembalikan saldo pengirim jika gagal mencari wallet penerima
		s.walletClient.UpdateWalletBalance(ctx, &walletPb.UpdateBalanceRequest{
			UserId:          senderUserID,
			Amount:          req.Amount,
			ExpectedVersion: senderWallet.GetVersion() + 1,
		})
		s.txRepo.UpdateStatus(ctx, txID, "FAILED")
		return nil, customErr.NewAppError(http.StatusNotFound, "RECEIVER_WALLET_NOT_FOUND", "Dompet penerima tidak ditemukan.")
	}

	_, err = s.walletClient.UpdateWalletBalance(ctx, &walletPb.UpdateBalanceRequest{
		UserId:          receiverUser.GetId(),
		Amount:          req.Amount,
		ExpectedVersion: receiverWallet.GetVersion(),
	})
	if err != nil {
		// Kompensasi: kembalikan saldo pengirim jika gagal kredit ke penerima
		s.walletClient.UpdateWalletBalance(ctx, &walletPb.UpdateBalanceRequest{
			UserId:          senderUserID,
			Amount:          req.Amount,
			ExpectedVersion: senderWallet.GetVersion() + 1,
		})
		s.txRepo.UpdateStatus(ctx, txID, "FAILED")
		return nil, customErr.ErrInternalServer
	}

	// 7. Catat Jejak Audit Finansial di Ledger Service gRPC (dan Verifikasi Status Error!)
	_, err = s.ledgerClient.RecordLedgerEntry(ctx, &ledgerPb.RecordEntryRequest{
		TransactionId: txID,
		WalletId:      senderWallet.GetId(),
		Type:          "DEBIT",
		Amount:        req.Amount,
	})
	if err != nil {
		// Kompensasi: kembalikan saldo penerima & pengirim karena gagal mencatat audit ledger
		s.walletClient.UpdateWalletBalance(ctx, &walletPb.UpdateBalanceRequest{
			UserId:          receiverUser.GetId(),
			Amount:          -req.Amount,
			ExpectedVersion: receiverWallet.GetVersion() + 1,
		})
		s.walletClient.UpdateWalletBalance(ctx, &walletPb.UpdateBalanceRequest{
			UserId:          senderUserID,
			Amount:          req.Amount,
			ExpectedVersion: senderWallet.GetVersion() + 2,
		})
		s.txRepo.UpdateStatus(ctx, txID, "FAILED")
		return nil, customErr.NewAppError(http.StatusInternalServerError, "LEDGER_ERROR", "Gagal mencatat audit log. Transaksi dibatalkan.")
	}

	_, err = s.ledgerClient.RecordLedgerEntry(ctx, &ledgerPb.RecordEntryRequest{
		TransactionId: txID,
		WalletId:      receiverWallet.GetId(),
		Type:          "CREDIT",
		Amount:        req.Amount,
	})
	if err != nil {
		// Kompensasi: kembalikan saldo penerima & pengirim
		s.walletClient.UpdateWalletBalance(ctx, &walletPb.UpdateBalanceRequest{
			UserId:          receiverUser.GetId(),
			Amount:          -req.Amount,
			ExpectedVersion: receiverWallet.GetVersion() + 1,
		})
		s.walletClient.UpdateWalletBalance(ctx, &walletPb.UpdateBalanceRequest{
			UserId:          senderUserID,
			Amount:          req.Amount,
			ExpectedVersion: senderWallet.GetVersion() + 2,
		})
		// Kompensasi: karena DEBIT ledger pengirim di atas sudah terlanjur dicatat, kita harus menulis CREDIT ledger penyeimbang (ledger bersifat immutable)
		s.ledgerClient.RecordLedgerEntry(ctx, &ledgerPb.RecordEntryRequest{
			TransactionId: txID,
			WalletId:      senderWallet.GetId(),
			Type:          "CREDIT", // Menetralkan DEBIT sebelumnya
			Amount:        req.Amount,
		})
		s.txRepo.UpdateStatus(ctx, txID, "FAILED")
		return nil, customErr.NewAppError(http.StatusInternalServerError, "LEDGER_ERROR", "Gagal mencatat audit log penerima. Transaksi dibatalkan.")
	}

	// 8. Update status transaksi menjadi SUCCESS
	txRecord.Status = "SUCCESS"
	s.txRepo.UpdateStatus(ctx, txID, "SUCCESS")

	return txRecord, nil
}
```

### Step 3: Koneksikan Dial gRPC Server di `cmd/main.go`
Buka `transaction-service/cmd/main.go`. Setup koneksi dial ke tiga port gRPC server tujuan:

```go
// Di dalam main() transaction-service:
	// ...

	// Koneksi gRPC ke User Service
	userConn, _ := grpc.Dial("localhost:50052", grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer userConn.Close()
	userClient := userPb.NewUserServiceClient(userConn)

	// Koneksi gRPC ke Wallet Service
	walletConn, _ := grpc.Dial("localhost:50053", grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer walletConn.Close()
	walletClient := walletPb.NewWalletServiceClient(walletConn)

	// Koneksi gRPC ke Ledger Service
	ledgerConn, _ := grpc.Dial("localhost:50054", grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer ledgerConn.Close()
	ledgerClient := ledgerPb.NewLedgerServiceClient(ledgerConn)

	// Inisialisasi service dengan menyuntikkan semua gRPC clients
	txSvc := transactionService.NewTransactionService(db, txRepo, userClient, walletClient, ledgerClient)
	
	// ... jalankan HTTP router ...
```

---

## ✅ Acceptance Criteria
* [ ] Memanggil endpoint `POST /api/v1/transactions/transfer` sukses memicu pemanggilan gRPC berantai ke User, Wallet, dan Ledger services.
* [ ] Mutasi saldo ter-update dengan aman di database `wallet-service` dan data mutasi pembukuan tercatat di `ledger-service`.
* [ ] Jika proses perekaman ledger atau salah satu mutasi saldo gagal di tengah jalan, sistem melakukan panggilan gRPC kompensasi untuk mengembalikan saldo seperti semula dan menandai transaksi sebagai `FAILED`.

---

## 💡 Tips untuk Junior
* **Saga Orchestration Pattern:** Logika transfer di atas adalah contoh nyata dari **Saga Orchestrator**. Karena kita tidak memiliki transaksi global terdistribusi (seperti 2PC/Two-Phase Commit yang berat dan memblokir database), kita memproses langkah demi langkah secara berurutan. Jika salah satu langkah gagal, kita wajib memanggil **Compensating Transactions** (transaksi kompensasi/pembatalan) untuk mengembalikan keadaan data di service lain ke posisi semula guna menjaga *eventual consistency*.
* **Immutable Ledger Compensation:** Perhatikan bahwa kita tidak menghapus baris database ledger saat membatalkan transaksi, karena ledger bersifat *immutable* (tidak boleh didelete/update). Kita menulis entry jurnal baru yang berlawanan arah (misalnya mencatat CREDIT untuk menetralkan DEBIT yang keliru).
