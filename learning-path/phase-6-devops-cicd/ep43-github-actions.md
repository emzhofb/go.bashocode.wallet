# Episode 43: CI/CD Pipeline dengan GitHub Actions

## 🎯 Tujuan
* Memahami prinsip **Continuous Integration (CI)** dan **Continuous Deployment (CD)**.
* Membuat pipeline otomatisasi menggunakan **GitHub Actions**.
* Menyusun alur pipeline untuk mengeksekusi:
  * **Static Analysis (Linting):** Menguji kerapian dan standar penulisan kode Go.
  * **Automated Unit Testing:** Menjalankan seluruh unit test Go.
  * **Test Coverage Gate:** Memblokir integrasi jika cakupan test di bawah target 70%.

---

## 📐 Konsep CI/CD Pipeline
Saat kita berkolaborasi dalam tim di Git, kita tidak ingin ada developer yang tidak sengaja mengirimkan (*push*) kode rusak atau penuh bug ke branch `master`.
* **CI (Continuous Integration)** otomatis mendeteksi kesalahan tersebut. Begitu ada Pull Request masuk, server GitHub Actions akan menyalakan container virtual Linux, menarik kode terbaru, lalu menjalankan linter dan seluruh unit test.
* Jika semua test lulus (`PASS`) dan cakupan pengujian memenuhi standar (misal: > 70% coverage), PR baru boleh di-merge.

---

## 📦 Langkah-langkah

### Step 1: Membuat File Workflow GitHub Actions
Di folder root workspace project, buat folder baru `.github/workflows/`:
```bash
mkdir -p .github/workflows
```

Buat file baru di `.github/workflows/ci.yml`:

```yaml
name: GoWallet CI Pipeline

on:
  push:
    branches: [ master, develop ]
  pull_request:
    branches: [ master, develop ]

jobs:
  # Job 1: Linting check untuk mengecek penulisan kode
  lint:
    name: Code Linting Check
    runs-on: ubuntu-latest
    steps:
      - name: Checkout Code
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.22'
          cache: true

      - name: Run golangci-lint
        uses: golangci/golangci-lint-action@v4
        with:
          version: latest
          args: --timeout=5m

  # Job 2: Running unit tests dengan spin up database container di runner
  test:
    name: Run Automated Tests
    runs-on: ubuntu-latest
    needs: lint # Jalankan test hanya jika linting lulus
    
    # Spin up database MySQL & Redis temporer di virtual runner GitHub
    services:
      mysql:
        image: mysql:8.0
        env:
          MYSQL_ROOT_PASSWORD: rootpassword
          MYSQL_DATABASE: gowallet_test
          MYSQL_USER: gowallet_user
          MYSQL_PASSWORD: gowallet_password
        ports:
          - 3306:3306
        options: >-
          --health-cmd="mysqladmin ping"
          --health-interval=10s
          --health-timeout=5s
          --health-retries=5

      redis:
        image: redis:7-alpine
        ports:
          - 6379:6379

    steps:
      - name: Checkout Code
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.22'
          cache: true

      - name: Run Unit Tests with Coverage Report
        env:
          DB_HOST: localhost
          DB_PORT: 3306
          DB_USER: gowallet_user
          DB_PASSWORD: gowallet_password
          DB_NAME: gowallet_test
          REDIS_HOST: localhost
          REDIS_PORT: 6379
        run: |
          # Jalankan test di seluruh sub-modul workspace dan generate file cover
          go test ./... -v -race -coverprofile=coverage.out

      - name: Verify Test Coverage Threshold (Min 70%)
        run: |
          # Baca total persentase coverage dari file output
          COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | tr -d '%')
          echo "Current Code Coverage is: ${COVERAGE}%"
          
          # Cek apakah coverage kurang dari 70% menggunakan bash comparison
          if (( $(echo "$COVERAGE < 70" | bc -l) )); then
            echo "Error: Code coverage is below threshold of 70%!"
            exit 1
          fi
          echo "Coverage check passed!"
```

### Step 2: Commit dan Push ke GitHub
Simpan file, commit ke Git, dan push ke repository GitHub Anda:

```bash
git add .github/
git commit -m "ci(ep30): add GitHub Actions CI workflow pipeline"
git push origin master
```

Buka tab **Actions** pada repository GitHub Anda di browser. Anda akan melihat pipeline `GoWallet CI Pipeline` ter-trigger secara otomatis. Lacak log perjalanannya untuk memastikan seluruh step hijau (`PASS`).

---

## 🏆 Selamat!
Jika Anda sampai di titik ini, Anda telah berhasil mendesain kurikulum belajar **dari Monolith terkecil hingga Microservices terdistribusi** yang mencakup:
* ✅ Evolusi arsitektur bertahap (Monolith ➔ Microservices).
* ✅ Desain database migration & skema presisi DECIMAL.
* ✅ Transaksi database SQL atomik dan Concurrency Control (Optimistic Locking).
* ✅ Caching & rate limiting performa tinggi berbasis Redis.
* ✅ Komunikasi RPC internal berkecepatan tinggi (gRPC).
* ✅ Pola transaksi terdistribusi yang andal (Outbox Pattern & RabbitMQ).
* ✅ Pengiriman email & audit asinkronus (Goroutines, Consumer, MongoDB).
* ✅ Penanganan kegagalan message broker tangguh (DLQ & Retries).
* ✅ Resiliensi & observabilitas standard industri (Circuit Breakers, Jaeger Tracing, Prometheus, Grafana, ELK Stack).
* ✅ Otomatisasi DevOps (Dockerfile multi-stage & CI/CD Pipeline).

Ini adalah modul pembelajaran yang sangat kuat untuk membimbing karir junior developer menuju tingkat *production-ready engineer*. 🚀

---

## ✅ Acceptance Criteria
* [ ] File pipeline `.github/workflows/ci.yml` tersimpan di repository.
* [ ] Push ke branch `master` memicu otomatisasi build di GitHub Actions.
* [ ] Step test coverage gate sukses memblokir build jika persentase pengujian berada di bawah target 70%.

---

## 📚 Referensi Belajar
* [GitHub Actions Official Documentation](https://docs.github.com/en/actions)
* [golangci-lint Configuration Guide](https://golangci-lint.run/)
