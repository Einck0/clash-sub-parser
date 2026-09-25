-- CSP Node Credential Vault Schema
-- Migration: 000005_node_credentials.sql

-- Stores versioned, AEAD-encrypted node credentials with associated data authenticated.
-- Plaintext secrets are NEVER written to the database.
CREATE TABLE IF NOT EXISTS node_credentials (
    logical_id TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    key_id TEXT NOT NULL,
    nonce BLOB NOT NULL,
    ciphertext BLOB NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (logical_id, version),
    FOREIGN KEY (logical_id) REFERENCES nodes(logical_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_node_credentials_logical_id ON node_credentials(logical_id);
