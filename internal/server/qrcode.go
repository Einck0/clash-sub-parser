package server

import (
	"fmt"
	"strings"

	"github.com/skip2/go-qrcode"
)

// GenerateQRCodePNG generates a PNG-encoded QR code byte slice.
func GenerateQRCodePNG(content string, size int) ([]byte, error) {
	if size <= 0 {
		size = 256
	}
	if size > 1024 {
		size = 1024
	}
	return qrcode.Encode(content, qrcode.Medium, size)
}

// GenerateQRCodeSVG generates a clean, scalable SVG string representing the QR code.
func GenerateQRCodeSVG(content string) (string, error) {
	qr, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		return "", err
	}

	bitmap := qr.Bitmap()
	size := len(bitmap)
	if size == 0 {
		return "", fmt.Errorf("empty qr bitmap")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges">`, size, size))
	sb.WriteString(fmt.Sprintf(`<rect width="%d" height="%d" fill="#ffffff"/>`, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if bitmap[y][x] {
				sb.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="1" height="1" fill="#000000"/>`, x, y))
			}
		}
	}
	sb.WriteString(`</svg>`)
	return sb.String(), nil
}
