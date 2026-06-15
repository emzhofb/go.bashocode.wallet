# Episode 2: Database Migration & Schema Design

## 🎯 Tujuan
* Memahami pentingnya Database Migration dibanding manual SQL script Execution.
* Mendesain schema database untuk seluruh core domain: `users`, `wallets`, `ledger_entries`, dan `transactions`.
* Menginstal dan menjalankan `golang-migrate` untuk mengelola perubahan schema secara terkontrol.

---

## 📦 Langkah-langkah

### Step 1: Kenapa Harus Pakai Migration Tool?
Saat mendevelop aplikasi dalam tim atau saat deploy ke server staging/production, membuat tabel database secara manual menggunakan GUI (DBeaver, TablePlus, dll) sangat berbahaya. 
* Kita bisa lupa tabel apa saja yang baru dibuat.
* Rekan setim tidak mendapat perubahan schema terbaru secara otomatis.
* Kehilangan histori perubahan struktur database.

Dengan migration tool, setiap perubahan schema dicatat dalam file `.sql` berurutan (misalnya `000001_init.up.sql` dan `000001_init.down.sql`) yang di-commit ke Git.

### Step 2: Inisialisasi Folder Migrations
Di dalam folder `monolith`, buat folder baru untuk menyimpan file migrations:

```bash
mkdir -p db/migrations
```

### Step 3: Membuat File Migration Pertama (Schema Design)
Gunakan `golang-migrate` CLI untuk men-generate file migration kosong, atau buat manual dua file berikut di dalam direktori `db/migrations/`:

**File 1: [000001_init_schema.up.sql](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/monolith/db/migrations/000001_init_schema.up.sql)**
File ini berisi SQL untuk membuat struktur tabel dari awal:

```sql
CREATE TABLE users (
    id VARCHAR(36) PRIMARY KEY,
    full_name VARCHAR(100) NOT NULL,
    email VARCHAR(150) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL
);

CREATE TABLE wallets (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) UNIQUE NOT NULL,
    balance DECIMAL(15, 2) NOT NULL DEFAULT 0.00,
    currency VARCHAR(3) NOT NULL DEFAULT 'IDR',
    status VARCHAR(20) NOT NULL DEFAULT 'active', -- active, frozen
    version INT NOT NULL DEFAULT 1, -- Digunakan untuk Optimistic Locking
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE transactions (
    id VARCHAR(36) PRIMARY KEY,
    sender_wallet_id VARCHAR(36) NULL,
    receiver_wallet_id VARCHAR(36) NOT NULL,
    amount DECIMAL(15, 2) NOT NULL,
    description TEXT NULL,
    idempotency_key VARCHAR(100) UNIQUE NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'success', -- success, failed, pending
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (sender_wallet_id) REFERENCES wallets(id),
    FOREIGN KEY (receiver_wallet_id) REFERENCES wallets(id)
);

CREATE TABLE ledger_entries (
    id VARCHAR(36) PRIMARY KEY,
    wallet_id VARCHAR(36) NOT NULL,
    transaction_id VARCHAR(36) NOT NULL,
    entry_type VARCHAR(10) NOT NULL, -- credit (+), debit (-)
    amount DECIMAL(15, 2) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (wallet_id) REFERENCES wallets(id),
    FOREIGN KEY (transaction_id) REFERENCES transactions(id)
);
```

**File 2: [000001_init_schema.down.sql](file:///Users/ikhda/Documents/coding/golang/wallet-microservice/monolith/db/migrations/000001_init_schema.down.sql)**
File ini berisi script pembalik jika kita ingin me-rollback schema (urutan drop harus terbalik karena urutan foreign key):

```sql
DROP TABLE IF EXISTS ledger_entries;
DROP TABLE IF EXISTS transactions;
DROP TABLE IF EXISTS wallets;
DROP TABLE IF EXISTS users;
```

### Step 4: Menjalankan Migrasi
Kita akan menjalankan migrasi menggunakan CLI `migrate`. 

*Pastikan Anda sudah menginstal CLI `golang-migrate` (lihat Development Guide).*

Jalankan perintah berikut di terminal root folder `monolith/`:
```bash
# Menjalankan migrasi "UP" untuk membuat tabel-tabel di database
migrate -path db/migrations -database "mysql://gowallet_user:gowallet_password@tcp(localhost:3306)/gowallet" up
```

Jika berhasil, outputnya akan kosong atau menampilkan log sukses. Buka database Anda menggunakan client tool GUI favorit Anda, dan pastikan tabel `users`, `wallets`, `transactions`, dan `ledger_entries` sudah terbuat, lengkap dengan satu tabel bawaan bernama `schema_migrations` yang mencatat versi migrasi terakhir.

Jika ingin membatalkan/rollback migrasi:
```bash
# Menjalankan migrasi "DOWN" untuk menghapus kembali tabel (rollback 1 step)
migrate -path db/migrations -database "mysql://gowallet_user:gowallet_password@tcp(localhost:3306)/gowallet" down 1
```

---

## ✅ Acceptance Criteria
* [ ] Folder `db/migrations/` terisi dengan file `.up.sql` dan `.down.sql` bernomor urut.
* [ ] Perintah `migrate ... up` berjalan lancar tanpa syntax error SQL.
* [ ] Skema tabel MySQL sesuai dengan desain (terdapat constraint `FOREIGN KEY` dan tipe data `DECIMAL` untuk uang).
* [ ] Tabel `schema_migrations` berisi catatan versi terakhir (`1`).

---

## 💡 Tips untuk Junior
* **Gunakan DECIMAL untuk Finansial:** Jangan pernah menyimpan uang menggunakan tipe data `FLOAT` atau `DOUBLE` karena adanya masalah pembulatan biner (*floating-point imprecision*). Gunakan `DECIMAL(15, 2)` agar pembulatan presisi hingga 2 digit di belakang koma.
* **UUID sebagai String/Varchar:** Di awal kita memakai `VARCHAR(36)` untuk menampung UUID agar gampang dibaca secara plain text.
* **Idempotency Key:** Kolom `idempotency_key` di tabel transactions di-set `UNIQUE` agar request transaksi yang terduplikasi secara tidak sengaja tidak akan diproses dua kali oleh database.

---

## 📚 Referensi Belajar
* [golang-migrate GitHub Repository](https://github.com/golang-migrate/migrate)
* [Why floating-point arithmetic is bad for financial values](https://floating-point-gui.de/)
* [Database Schema Design Best Practices](https://dbdiagram.io/)
