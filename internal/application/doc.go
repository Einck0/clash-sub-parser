// Package application provides use cases and application orchestration services
// for CSP 1.0, coordinating domain models, repository ports, and external adapters.
//
// Architectural rule: application orchestrates domain and repository ports.
// transport/http -> application -> domain <- repository/sqlite
package application
