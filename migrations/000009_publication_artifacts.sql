-- Migration: 000009_publication_artifacts.sql
-- Persist immutable publication digests, versioned credential bindings, and AEAD-encrypted compiled artifacts.

ALTER TABLE publications ADD COLUMN revision_id TEXT NOT NULL DEFAULT '';
ALTER TABLE publications ADD COLUMN content_digest TEXT NOT NULL DEFAULT '';
ALTER TABLE publications ADD COLUMN credential_binding_digest TEXT NOT NULL DEFAULT '';
ALTER TABLE publications ADD COLUMN credential_bindings_json TEXT NOT NULL DEFAULT '';
ALTER TABLE publications ADD COLUMN content_type TEXT NOT NULL DEFAULT '';
ALTER TABLE publications ADD COLUMN filename TEXT NOT NULL DEFAULT '';
ALTER TABLE publications ADD COLUMN artifact_key_id TEXT NOT NULL DEFAULT '';
ALTER TABLE publications ADD COLUMN artifact_nonce BLOB NOT NULL DEFAULT X'';
ALTER TABLE publications ADD COLUMN artifact_ciphertext BLOB NOT NULL DEFAULT X'';
