package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const (
	gatewayURL    = "http://localhost:8080"
	webhookSecret = "super-secret-key-change-this"
	dbDSN         = "gowallet_user:gowallet_password@tcp(localhost:3306)/"
)

func main() {
	fmt.Println("🚀 Starting GoWallet End-to-End Integration Test...")

	// 1. Database Cleanups
	cleanupDatabases()

	// 2. Register Users
	userAEmail := "usera@example.com"
	userBEmail := "userb@example.com"
	fmt.Println("➡️ Registering User A and User B...")
	userA := registerUser("User A", userAEmail, "password123")
	userB := registerUser("User B", userBEmail, "password123")
	fmt.Printf("✅ Registered User A (%s) and User B (%s)\n", userA.ID, userB.ID)

	// 3. Login
	fmt.Println("➡️ Authenticating users...")
	tokenA := loginUser(userAEmail, "password123")
	tokenB := loginUser(userBEmail, "password123")
	fmt.Println("✅ Successfully generated JWT tokens for both users")

	// 4. Create Wallets
	fmt.Println("➡️ Creating wallets...")
	walletA := createWallet(tokenA)
	walletB := createWallet(tokenB)
	fmt.Printf("✅ Created Wallet A (%s) and Wallet B (%s)\n", walletA.ID, walletB.ID)

	// 5. Verify Balance (should be 0)
	fmt.Println("➡️ Verifying initial balances (should be 0)...")
	checkBalance(tokenA, 0)
	checkBalance(tokenB, 0)
	fmt.Println("✅ Initial balances verified as 0")

	// 6. Simulate Top-up via Webhook (requires HMAC Signature)
	fmt.Println("➡️ Simulating Top-up of 100,000 for User A via Webhook...")
	simulateTopup(userA.ID, 100000)

	// Wait for RabbitMQ event processing
	fmt.Println("⏳ Waiting 3 seconds for RabbitMQ to deliver payment event...")
	time.Sleep(3 * time.Second)

	// Verify User A Balance is 100,000
	fmt.Println("➡️ Verifying User A balance after top-up...")
	checkBalance(tokenA, 100000)
	fmt.Println("✅ User A balance is 100,000!")

	// 7. Perform Transfer (User A -> User B of 40,000)
	fmt.Println("➡️ Initiating transfer of 40,000 from User A to User B...")
	transferFunds(tokenA, userBEmail, 40000, "tx-idempotency-key-123")

	// Wait for Outbox/Ledger writes
	fmt.Println("⏳ Waiting 3 seconds for Ledger/Transactions and Outbox events...")
	time.Sleep(3 * time.Second)

	// Verify Balances
	fmt.Println("➡️ Verifying final balances (A should be 60,000, B should be 40,000)...")
	checkBalance(tokenA, 60000)
	checkBalance(tokenB, 40000)

	fmt.Println("\n🎉 🎉 🎉 ALL END-TO-END TESTS PASSED SUCCESSFULLY! 🎉 🎉 🎉")
}

type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type Wallet struct {
	ID string `json:"id"`
}

func cleanupDatabases() {
	db, err := sql.Open("mysql", dbDSN)
	if err != nil {
		log.Fatalf("Failed to connect to MySQL for cleanup: %v", err)
	}
	defer db.Close()

	fmt.Println("🧹 Cleaning up old test data...")
	_, _ = db.Exec("DELETE FROM gowallet_user.users WHERE email IN ('usera@example.com', 'userb@example.com')")
	// Clean up wallets, ledgers, transactions, outbox
	_, _ = db.Exec("DELETE FROM gowallet_wallet.wallets")
	_, _ = db.Exec("DELETE FROM gowallet_ledger.ledger_entries")
	_, _ = db.Exec("DELETE FROM gowallet_transactions.transactions")
	_, _ = db.Exec("DELETE FROM gowallet_transactions.outbox_events")
}

func registerUser(name, email, password string) *User {
	reqBody, _ := json.Marshal(map[string]string{
		"name":     name,
		"email":    email,
		"password": password,
	})

	resp, err := http.Post(gatewayURL+"/api/v1/users/register", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		log.Fatalf("Register request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("Failed to register user. Status: %d, Body: %s", resp.StatusCode, string(body))
	}

	var res struct {
		Data User `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&res)
	return &res.Data
}

func loginUser(email, password string) string {
	reqBody, _ := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})

	resp, err := http.Post(gatewayURL+"/api/v1/auth/login", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		log.Fatalf("Login request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("Failed to login user. Status: %d, Body: %s", resp.StatusCode, string(body))
	}

	var res struct {
		Data map[string]interface{} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&res)
	return res.Data["access_token"].(string)
}

func createWallet(token string) *Wallet {
	req, _ := http.NewRequest("POST", gatewayURL+"/api/v1/wallets/create", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Create wallet request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("Failed to create wallet. Status: %d, Body: %s", resp.StatusCode, string(body))
	}

	var res struct {
		Data Wallet `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&res)
	return &res.Data
}

func checkBalance(token string, expected int64) {
	req, _ := http.NewRequest("GET", gatewayURL+"/api/v1/wallets/balance", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Get balance request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("Failed to get balance. Status: %d, Body: %s", resp.StatusCode, string(body))
	}

	var res struct {
		Data map[string]interface{} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&res)
	balance := int64(res.Data["balance"].(float64))

	if balance != expected {
		log.Fatalf("Balance mismatch! Expected %d, got %d", expected, balance)
	}
	fmt.Printf("   -> Verified balance: %d\n", balance)
}

func simulateTopup(userID string, amount int64) {
	reqPayload := map[string]interface{}{
		"user_id":   userID,
		"amount":    amount,
		"reference": "TOPUP-E2E-TEST",
		"type":      "topup",
	}
	bodyBytes, _ := json.Marshal(reqPayload)

	// Generate HMAC signature
	mac := hmac.New(sha256.New, []byte(webhookSecret))
	mac.Write(bodyBytes)
	signature := hex.EncodeToString(mac.Sum(nil))

	req, _ := http.NewRequest("POST", gatewayURL+"/api/v1/payments/webhook", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature", signature)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Simulate top-up webhook failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("Webhook top-up returned non-200. Status: %d, Body: %s", resp.StatusCode, string(body))
	}
}

func transferFunds(token, receiverEmail string, amount int64, idempotencyKey string) {
	reqPayload := map[string]interface{}{
		"receiver_email":  receiverEmail,
		"amount":          amount,
		"idempotency_key": idempotencyKey,
	}
	bodyBytes, _ := json.Marshal(reqPayload)

	req, _ := http.NewRequest("POST", gatewayURL+"/api/v1/transactions/transfer", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Transfer request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("Transfer returned non-200. Status: %d, Body: %s", resp.StatusCode, string(body))
	}

	fmt.Println("✅ Transfer request succeeded!")
}
