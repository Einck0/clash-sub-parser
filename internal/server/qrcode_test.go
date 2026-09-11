package server_test

import (
	"bytes"
	"strings"
	"testing"

	"clash-sub-parser/internal/server"
)

func TestQRCode_Generation(t *testing.T) {
	t.Run("PNG generation", func(t *testing.T) {
		pngBytes, err := server.GenerateQRCodePNG("https://example.com/sub", 256)
		if err != nil {
			t.Fatalf("failed to generate QR PNG: %v", err)
		}
		if len(pngBytes) == 0 {
			t.Fatalf("expected non-empty PNG bytes")
		}
		// PNG magic bytes: \x89PNG\r\n\x1a\n
		pngMagic := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
		if !bytes.HasPrefix(pngBytes, pngMagic) {
			t.Errorf("expected PNG magic header, got %v", pngBytes[:8])
		}
	})

	t.Run("SVG generation", func(t *testing.T) {
		svgStr, err := server.GenerateQRCodeSVG("https://example.com/sub")
		if err != nil {
			t.Fatalf("failed to generate QR SVG: %v", err)
		}
		if !strings.HasPrefix(svgStr, "<svg") || !strings.HasSuffix(strings.TrimSpace(svgStr), "</svg>") {
			t.Errorf("expected valid SVG tags, got: %s", svgStr)
		}
		if !strings.Contains(svgStr, `fill="#000000"`) {
			t.Errorf("expected black fill rects in SVG: %s", svgStr)
		}
	})
}
