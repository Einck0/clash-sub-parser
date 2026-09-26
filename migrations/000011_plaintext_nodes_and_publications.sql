-- Migration: 000011_plaintext_nodes_and_publications.sql
-- Store plaintext node connection/credentials in nodes and plaintext compiled content in publications; drop node_credentials.

ALTER TABLE nodes ADD COLUMN server TEXT NOT NULL DEFAULT '';
ALTER TABLE nodes ADD COLUMN port INTEGER NOT NULL DEFAULT 0;
ALTER TABLE nodes ADD COLUMN config_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE nodes DROP COLUMN normalized_config_secret_ref;
ALTER TABLE nodes DROP COLUMN credential_version;

ALTER TABLE publications ADD COLUMN content BLOB NOT NULL DEFAULT X'';
ALTER TABLE publications DROP COLUMN artifact_key_id;
ALTER TABLE publications DROP COLUMN artifact_nonce;
ALTER TABLE publications DROP COLUMN artifact_ciphertext;
ALTER TABLE publications DROP COLUMN credential_binding_digest;
ALTER TABLE publications DROP COLUMN credential_bindings_json;

DROP TABLE IF EXISTS node_credentials;
