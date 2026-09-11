package compiler

import (
	"context"

	"clash-sub-parser/internal/domain"
)

// Bundle packages domain inputs needed to compile a client configuration.
type Bundle struct {
	Nodes          []*domain.Node
	Groups         []*domain.NodeGroup
	Rules          []*domain.Rule
	DNS            *domain.DNSConfig
	GenerateConfig *domain.GenerateConfig
}

// Result holds the serialized configuration bytes and corresponding MIME content type.
type Result struct {
	Content     []byte
	ContentType string
	Filename    string
}

// Compiler defines the contract for producing multi-client configurations.
type Compiler interface {
	Compile(ctx context.Context, bundle *Bundle, target domain.ExportTarget) (*Result, error)
}
