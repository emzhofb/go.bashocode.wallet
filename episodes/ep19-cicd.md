# Episode 19: CI/CD & GitHub Actions

## 🎯 Tujuan
- Dockerfile untuk setiap service (multi-stage build)
- GitHub Actions pipeline: lint, test, build
- Multi-environment configuration
- Secret management

## 📝 Prerequisites
- Semua services sudah berjalan dan testable
- GitHub repository sudah dibuat

---

## 📦 Langkah-langkah

### Step 1: Dockerfile (Multi-stage Build)

Buat Dockerfile di setiap service directory. Pattern yang sama:

```dockerfile
# ==========================================
# Stage 1: Build
# ==========================================
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Copy go.mod dan go.sum terlebih dahulu (layer caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /service ./cmd/main.go

# ==========================================
# Stage 2: Run
# ==========================================
FROM alpine:3.19

# Install certificates (untuk HTTPS calls)
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN adduser -D -g '' appuser

WORKDIR /app

# Copy binary from builder
COPY --from=builder /service .

# Copy migration files (jika ada)
COPY --from=builder /app/db/migrations ./db/migrations

# Use non-root user
USER appuser

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run binary
CMD ["./service"]
```

### Step 2: Update Docker Compose (Full Stack)

Tambahkan semua services ke docker-compose:

```yaml
  # Application Services
  api-gateway:
    build: ./api-gateway
    ports: ["8080:8080"]
    env_file: .env.dev
    depends_on: [mysql, redis, rabbitmq]
    networks: [gowallet-network]

  auth-service:
    build: ./auth-service
    ports: ["8081:8081"]
    env_file: .env.dev
    depends_on: [mysql, redis]
    networks: [gowallet-network]

  # ... same pattern for all services ...
```

Test full stack:
```bash
docker compose up --build
```

### Step 3: GitHub Actions — Lint

**`.github/workflows/lint.yml`:**
```yaml
name: Lint

on:
  pull_request:
    branches: [develop, main]

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      
      - name: golangci-lint
        uses: golangci/golangci-lint-action@v4
        with:
          version: latest
          args: --timeout=5m
      
      - name: Check go mod tidy
        run: |
          go mod tidy
          git diff --exit-code go.mod go.sum
```

### Step 4: GitHub Actions — Test

**`.github/workflows/test.yml`:**
```yaml
name: Test

on:
  pull_request:
    branches: [develop, main]

jobs:
  test:
    runs-on: ubuntu-latest
    
    services:
      mysql:
        image: mysql:8.0
        env:
          MYSQL_ROOT_PASSWORD: rootpassword
          MYSQL_DATABASE: gowallet_test
          MYSQL_USER: gowallet
          MYSQL_PASSWORD: secret
        ports: ['3306:3306']
        options: >-
          --health-cmd="mysqladmin ping"
          --health-interval=10s
          --health-timeout=5s
          --health-retries=5
      
      redis:
        image: redis:7-alpine
        ports: ['6379:6379']
    
    steps:
      - uses: actions/checkout@v4
      
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      
      - name: Run tests
        env:
          MYSQL_HOST: localhost
          MYSQL_PORT: 3306
          REDIS_HOST: localhost
        run: |
          go test ./... -v -race -coverprofile=coverage.out
      
      - name: Check coverage
        run: |
          COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | tr -d '%')
          echo "Coverage: ${COVERAGE}%"
          if (( $(echo "$COVERAGE < 70" | bc -l) )); then
            echo "Coverage below 70%!"
            exit 1
          fi
      
      - name: Upload coverage
        uses: actions/upload-artifact@v4
        with:
          name: coverage-report
          path: coverage.out
```

### Step 5: GitHub Actions — Build & Push

**`.github/workflows/deploy.yml`:**
```yaml
name: Build & Deploy

on:
  push:
    branches: [main, staging]

jobs:
  build:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        service:
          - api-gateway
          - auth-service
          - user-service
          - wallet-service
          - ledger-service
          - transaction-service
          - payment-service
          - notification-service
          - audit-service
          - scheduler-service
    
    steps:
      - uses: actions/checkout@v4
      
      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3
      
      - name: Login to Container Registry
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      
      - name: Build and push
        uses: docker/build-push-action@v5
        with:
          context: ./${{ matrix.service }}
          push: true
          tags: |
            ghcr.io/${{ github.repository }}/${{ matrix.service }}:${{ github.sha }}
            ghcr.io/${{ github.repository }}/${{ matrix.service }}:latest
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

### Step 6: Multi-Environment

| Environment | Branch | Config File | Deploy Target |
|---|---|---|---|
| Development | `develop` | `.env.dev` | Docker Compose local |
| Staging | `staging` | `.env.staging` | Remote server |
| Production | `main` | `.env.prod` | Kubernetes / VPS |

### Step 7: Secret Management

```bash
# GitHub Secrets (Settings → Secrets and variables → Actions):
# - MYSQL_PASSWORD
# - JWT_SECRET
# - GOOGLE_CLIENT_ID
# - GOOGLE_CLIENT_SECRET
# - SMTP_PASSWORD

# JANGAN commit .env files!
# .gitignore harus berisi:
# .env
# .env.dev
# .env.staging
# .env.prod
```

---

## ✅ Acceptance Criteria

- [ ] Setiap service punya Dockerfile working
- [ ] `docker compose up --build` menjalankan seluruh stack
- [ ] GitHub Actions lint pass
- [ ] GitHub Actions test pass dengan coverage ≥ 70%
- [ ] GitHub Actions build + push Docker images
- [ ] Secrets tidak ter-expose di logs
- [ ] Multi-environment configs terpisah

---

## 📚 Referensi

- [Multi-stage Docker Build](https://docs.docker.com/build/building/multi-stage/)
- [GitHub Actions](https://docs.github.com/en/actions)
- [GitHub Container Registry](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)
