CREATE DATABASE IF NOT EXISTS gowallet_auth;
CREATE DATABASE IF NOT EXISTS gowallet_user;
CREATE DATABASE IF NOT EXISTS gowallet_wallet;
CREATE DATABASE IF NOT EXISTS gowallet_ledger;
CREATE DATABASE IF NOT EXISTS gowallet_transactions;

-- Grant permissions
GRANT ALL PRIVILEGES ON gowallet_auth.* TO 'gowallet_user'@'%';
GRANT ALL PRIVILEGES ON gowallet_user.* TO 'gowallet_user'@'%';
GRANT ALL PRIVILEGES ON gowallet_wallet.* TO 'gowallet_user'@'%';
GRANT ALL PRIVILEGES ON gowallet_ledger.* TO 'gowallet_user'@'%';
GRANT ALL PRIVILEGES ON gowallet_transactions.* TO 'gowallet_user'@'%';
FLUSH PRIVILEGES;

-- -----------------------------------------------------
-- Table structures for gowallet_user
-- -----------------------------------------------------
USE gowallet_user;

CREATE TABLE IF NOT EXISTS users (
    id VARCHAR(36) PRIMARY KEY,
    email VARCHAR(255) NOT NULL UNIQUE,
    password VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    role VARCHAR(50) NOT NULL DEFAULT 'user',
    is_verified TINYINT(1) NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL
);

-- -----------------------------------------------------
-- Table structures for gowallet_wallet
-- -----------------------------------------------------
USE gowallet_wallet;

CREATE TABLE IF NOT EXISTS wallets (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL UNIQUE,
    balance BIGINT NOT NULL DEFAULT 0,
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

-- -----------------------------------------------------
-- Table structures for gowallet_ledger
-- -----------------------------------------------------
USE gowallet_ledger;

CREATE TABLE IF NOT EXISTS ledger_entries (
    id VARCHAR(36) PRIMARY KEY,
    transaction_id VARCHAR(36) NOT NULL,
    wallet_id VARCHAR(36) NOT NULL,
    type VARCHAR(10) NOT NULL, -- 'debit' or 'credit'
    amount BIGINT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- -----------------------------------------------------
-- Table structures for gowallet_transactions
-- -----------------------------------------------------
USE gowallet_transactions;

CREATE TABLE IF NOT EXISTS transactions (
    id VARCHAR(36) PRIMARY KEY,
    sender_wallet_id VARCHAR(36) NOT NULL,
    receiver_wallet_id VARCHAR(36) NOT NULL,
    amount BIGINT NOT NULL,
    status VARCHAR(50) NOT NULL, -- 'pending', 'completed', 'failed', 'reversed'
    idempotency_key VARCHAR(255) NOT NULL UNIQUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS outbox_events (
    id VARCHAR(36) PRIMARY KEY,
    event_type VARCHAR(100) NOT NULL,
    payload TEXT NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending', -- 'pending', 'processed'
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
