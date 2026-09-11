package frontend

import (
	"embed"
	"fmt"
	"io/fs"
)

// DistFS embeds all compiled assets under the dist/ directory.
//
//go:embed all:dist
var DistFS embed.FS

// FS returns an fs.FS sub-tree rooted at dist/.
func FS() (fs.FS, error) {
	sub, err := fs.Sub(DistFS, "dist")
	if err != nil {
		return nil, fmt.Errorf("failed to derive sub-filesystem for dist: %w", err)
	}
	return sub, nil
}
