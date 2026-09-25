package sqlite

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Config encapsulates SQLite database connection and runtime parameters.
// Zero hardcoded absolute paths: Path must be supplied dynamically from environment, flags, or caller.
type Config struct {
	Path            string
	BusyTimeout     time.Duration
	ForeignKeys     bool
	WALMode         bool
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// DefaultConfig returns a production-ready SQLite configuration for the given path.
func DefaultConfig(path string) Config {
	isMem := isMemoryPath(path)
	return Config{
		Path:            path,
		BusyTimeout:     5 * time.Second,
		ForeignKeys:     true,
		WALMode:         !isMem,
		MaxOpenConns:    1, // Default single connection for SQLite file safety, or pool if needed
		MaxIdleConns:    1,
		ConnMaxLifetime: 0,
	}
}

func isMemoryPath(path string) bool {
	if path == ":memory:" || path == "" {
		return true
	}
	if strings.HasPrefix(path, "file:") && strings.Contains(path, "mode=memory") {
		return true
	}
	return false
}

// BuildDSN constructs a modernc.org/sqlite compliant DSN string.
func (c Config) BuildDSN() string {
	path := c.Path
	if path == "" {
		path = ":memory:"
	}

	isMem := isMemoryPath(path)
	pragmas := make([]string, 0, 4)

	if c.ForeignKeys {
		pragmas = append(pragmas, "_pragma=foreign_keys(1)")
	}

	timeoutMS := int64(5000)
	if c.BusyTimeout > 0 {
		timeoutMS = c.BusyTimeout.Milliseconds()
	}
	pragmas = append(pragmas, fmt.Sprintf("_pragma=busy_timeout(%d)", timeoutMS))

	if c.WALMode && !isMem {
		pragmas = append(pragmas, "_pragma=journal_mode(WAL)")
		pragmas = append(pragmas, "_pragma=synchronous(NORMAL)")
	} else if isMem {
		pragmas = append(pragmas, "_pragma=journal_mode(MEMORY)")
	}

	queryString := strings.Join(pragmas, "&")

	// If path is already a URI like "file:foo?mode=memory"
	if strings.HasPrefix(path, "file:") {
		if strings.Contains(path, "?") {
			return path + "&" + queryString
		}
		return path + "?" + queryString
	}

	// Plain path or :memory:
	if path == ":memory:" {
		return "file::memory:?" + queryString
	}

	// Plain file path
	return fmt.Sprintf("file:%s?%s", url.PathEscape(path), queryString)
}
