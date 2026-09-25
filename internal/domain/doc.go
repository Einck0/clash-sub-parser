// Package domain defines the pure domain entities, value objects, domain errors,
// and port interfaces for Clash Sub Parser (CSP) 1.0.
//
// Architectural rule: domain MUST remain completely pure. It MUST NOT import
// any HTTP, SQLite, sing-box, database/sql, or UI types.
package domain
