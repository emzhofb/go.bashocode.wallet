# Episode 35: Postman Collection & API Test Automation

## 🎯 Tujuan
* Mendesain **Postman Collection** terstruktur untuk menguji seluruh endpoint microservices.
* Memanfaatkan **Postman Environments** (Variabel global/lokal) agar host endpoint bersifat dinamis.
* Menulis skrip testing otomatis menggunakan bahasa Javascript di dalam Postman untuk menangkap dan menyuntikkan JWT token secara dinamis.
* Menjalankan tes otomatis di terminal menggunakan runner **Newman CLI**.

---

## 📐 Mengapa Butuh Postman Automation?
Saat kita memiliki 9 microservices, menguji endpoint satu per satu secara manual sangat melelahkan dan rentan kesalahan.
* **Tantangan:** Setiap kali token kedaluwarsa setelah 15 menit, kita harus login ulang, menyalin token baru, dan menempelkannya (*paste*) manual ke tab Authorization di setiap request.
* **Solusinya (Automation):** Kita menggunakan tab **Tests** di Postman untuk secara otomatis mengekstrak token dari respons API login, menyimpannya di variabel environment, dan menggunakannya di semua request terlindungi secara global.

```
[ POST /auth/login ] ➔ (Auto extract token via JS) ➔ Save as {{token}}
                                                           │
                                                           ▼
                                                [ Use globally in header ]
                                                Authorization: Bearer {{token}}
```

---

## 📦 Langkah-langkah

### Step 1: Struktur Folder Postman Collection
Buka aplikasi Postman Desktop. Buat collection baru bernama **GoWallet API** dengan struktur folder:
```
GoWallet API/
├── 🔑 Auth & User/
│   ├── Register User (POST {{base_url}}/api/v1/users/register)
│   ├── Login User (POST {{base_url}}/api/v1/auth/login)
│   └── Get My Profile (GET {{base_url}}/api/v1/users/me)
├── 💳 Wallets/
│   ├── Check Balance (GET {{base_url}}/api/v1/wallets/me)
│   └── Transfer Saldo (POST {{base_url}}/api/v1/transactions/transfer)
└── 🔌 Payments (Webhook)/
    └── Webhook Callback Stripe (POST {{base_url}}/api/v1/payments/callback)
```

### Step 2: Membuat Environment Variables
Buat Environment di Postman bernama **GoWallet Local Dev** dan tambahkan variabel berikut:
* `base_url` ➔ `http://localhost:8080` (Arahkan ke API Gateway)
* `token` ➔ (Biarkan kosong, akan diisi otomatis oleh skrip)

### Step 3: Menulis Script Auto-Authentication di Postman
Buka request **Login User**. Masuk ke tab **Tests** (di bawah URL request), dan tulis kode Javascript ini:

```javascript
// 1. Pastikan response mengembalikan HTTP 200 OK
pm.test("Status code is 200", function () {
    pm.response.to.have.status(200);
});

// 2. Parse body JSON response
var responseData = pm.response.json();

// 3. Simpan access_token ke environment variable
if (responseData.success && responseData.data.access_token) {
    pm.environment.set("token", responseData.data.access_token);
    console.log("Access Token updated successfully in Environment!");
} else {
    pm.expect.fail("Failed to retrieve access token from response.");
}
```

Di level folder parent **GoWallet API** (klik kanan collection ➔ Edit), masuk ke tab **Authorization**:
* **Type:** Bearer Token
* **Token:** `{{token}}`
*Dengan ini, seluruh request di dalam collection otomatis menggunakan token dinamis tersebut tanpa perlu input manual.*

### Step 4: Menjalankan Test via Command Line (Newman CLI)
Export Postman Collection Anda sebagai file JSON: `gowallet.postman_collection.json` dan environment sebagai `development.postman_environment.json`, letakkan di folder `postman/` root project.

Instal runner Newman menggunakan NPM:
```bash
npm install -g newman
```

Jalankan test suite dari terminal:
```bash
newman run postman/gowallet.postman_collection.json -e postman/development.postman_environment.json
```
Newman akan mengeksekusi semua request secara berurutan, melakukan pengujian assertions JavaScript, dan mencetak laporan tabel hasil testing sukses/gagal di terminal Anda.

---

## ✅ Acceptance Criteria
* [ ] Collection berhasil di-export ke subfolder `postman/`.
* [ ] Melakukan login otomatis memperbarui isi variabel `token` di Postman Environment.
* [ ] Menjalankan perintah `newman run ...` berhasil mengeksekusi seluruh pengujian tanpa kegagalan assertion (status code 200/201).

---

## 💡 Tips untuk Junior
* **Variabel Dinamis Postman:** Gunakan syntax bawaan Postman untuk menghasilkan data palsu yang dinamis, seperti `{{$randomEmail}}` untuk input body register agar Anda tidak perlu mengubah email manual setiap kali me-run test.
