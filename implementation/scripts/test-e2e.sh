#!/bin/bash
cd "$(dirname "$0")/.." || exit 1
export PATH=$PATH:/usr/local/bin:/opt/homebrew/bin
echo "Starting E2E Integration Test..."
go run scripts/test-e2e.go
