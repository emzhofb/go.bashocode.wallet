.PHONY: help build docker-up docker-down docker-reset test-e2e

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build all services locally
	@echo "Building all services..."
	cd api-gateway && go build -o ../bin/api-gateway cmd/main.go
	cd auth-service && go build -o ../bin/auth-service cmd/main.go
	cd user-service && go build -o ../bin/user-service cmd/main.go
	cd wallet-service && go build -o ../bin/wallet-service cmd/main.go
	cd ledger-service && go build -o ../bin/ledger-service cmd/main.go
	cd transaction-service && go build -o ../bin/transaction-service cmd/main.go
	cd payment-service && go build -o ../bin/payment-service cmd/main.go
	cd notification-service && go build -o ../bin/notification-service cmd/main.go
	cd audit-service && go build -o ../bin/audit-service cmd/main.go
	cd scheduler-service && go build -o ../bin/scheduler-service cmd/main.go
	@echo "All services built successfully! Binaries are in the implementation/bin/ folder."

docker-up: ## Start all docker containers and services
	docker compose up -d

docker-down: ## Stop all docker containers
	docker compose down

docker-reset: ## Hard reset docker compose volumes and recreate containers
	docker compose down -v
	docker compose up -d --build

test-e2e: ## Run end-to-end integration and verification script
	bash ./scripts/test-e2e.sh
