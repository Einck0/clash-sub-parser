package inventory

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

// RedactTokenURL masks query parameters and path segments containing credentials or tokens.
func RedactTokenURL(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return ""
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return "[REDACTED]"
	}
	// Mask sensitive query parameters
	q := u.Query()
	for k := range q {
		lower := strings.ToLower(k)
		if lower == "token" || lower == "key" || lower == "secret" || lower == "password" || lower == "auth" || lower == "token_id" {
			q.Set(k, "[REDACTED]")
		}
	}
	u.RawQuery = q.Encode()

	// Mask secret path tokens (e.g. /sub/<long-token>/...)
	pathSegments := strings.Split(u.Path, "/")
	for i, seg := range pathSegments {
		if len(seg) >= 20 && !strings.Contains(seg, ".") && !strings.Contains(seg, "subscribe") {
			pathSegments[i] = "REDACTED_PATH_TOKEN"
		}
	}
	u.Path = strings.Join(pathSegments, "/")
	return u.String()
}

// strippedBaseURL returns URL with raw query and fragment stripped, lowercased scheme and host.
func strippedBaseURL(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return ""
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return trimmed
	}
	clean := *u
	clean.RawQuery = ""
	clean.Fragment = ""
	clean.RawFragment = ""
	return strings.ToLower(clean.Scheme) + "://" + strings.ToLower(clean.Host) + clean.Path
}

// nonTokenQueryParams returns normalized query string excluding authentication tokens.
func nonTokenQueryParams(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	q := u.Query()
	q.Del("token")
	q.Del("key")
	q.Del("secret")
	q.Del("password")
	q.Del("auth")
	return q.Encode()
}

// isProvenBrokenURL checks if a subscription URL has lost its required authentication token in the query.
func isProvenBrokenURL(rawURL string) bool {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return false
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return false
	}
	q := u.Query()
	// If query already contains token/auth, it is not broken
	if q.Get("token") != "" || q.Get("key") != "" || q.Get("secret") != "" || q.Get("password") != "" || q.Get("auth") != "" {
		return false
	}
	// Path-token based subscriptions (like dogegg /sub/<token>/clash) are not query broken
	for _, seg := range strings.Split(u.Path, "/") {
		if len(seg) >= 20 && !strings.Contains(seg, ".") && !strings.Contains(seg, "subscribe") {
			return false
		}
	}
	return strings.HasPrefix(strings.ToLower(u.Scheme), "http")
}

// archivedEvidence represents stable provenance from cold archive DB.
type archivedEvidence struct {
	Name          string
	FullURL       string
	BaseURL       string
	NonTokenQuery string
	Enabled       int
}

// RestoreSourceTokensFromArchive inspects cold archive DB and restores full token URLs
// exclusively for proven damaged sources (7li and 魔戒) with unambiguous identity proof.
func RestoreSourceTokensFromArchive(ctx context.Context, db *sql.DB, archivePath string) (*domain.SourceTokenRepairResult, error) {
	fi, err := os.Stat(archivePath)
	if err != nil {
		return nil, fmt.Errorf("cold archive database not found at %s: %w", archivePath, err)
	}
	if fi.IsDir() {
		return nil, fmt.Errorf("cold archive path %s is a directory", archivePath)
	}

	adb, err := sqlite.OpenReadOnly(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open cold archive database: %w", err)
	}
	defer adb.Close()

	// Query archive subscriptions with enabled = 1 and non-empty token query
	var archiveProofs []archivedEvidence
	rows, err := adb.QueryContext(ctx, "SELECT name, url, enabled FROM subscriptions WHERE enabled = 1;")
	if err == nil {
		for rows.Next() {
			var name, u string
			var enabled int
			if scanErr := rows.Scan(&name, &u, &enabled); scanErr == nil {
				parsed, pErr := url.Parse(u)
				if pErr == nil && parsed.Query().Get("token") != "" {
					archiveProofs = append(archiveProofs, archivedEvidence{
						Name:          name,
						FullURL:       u,
						BaseURL:       strippedBaseURL(u),
						NonTokenQuery: nonTokenQueryParams(u),
						Enabled:       enabled,
					})
				}
			}
		}
		rows.Close()
	}

	// Also check sources table for additional proof if needed
	sRows, sErr := adb.QueryContext(ctx, "SELECT name, url, enabled FROM sources WHERE enabled = 1;")
	if sErr == nil {
		for sRows.Next() {
			var name, u string
			var enabled int
			if sRows.Scan(&name, &u, &enabled) == nil {
				parsed, pErr := url.Parse(u)
				if pErr == nil && parsed.Query().Get("token") != "" {
					exists := false
					for _, existing := range archiveProofs {
						if existing.BaseURL == strippedBaseURL(u) && existing.Name == name {
							exists = true
							break
						}
					}
					if !exists {
						archiveProofs = append(archiveProofs, archivedEvidence{
							Name:          name,
							FullURL:       u,
							BaseURL:       strippedBaseURL(u),
							NonTokenQuery: nonTokenQueryParams(u),
							Enabled:       enabled,
						})
					}
				}
			}
		}
		sRows.Close()
	}

	// Query target DB subscriptions
	type targetSub struct {
		id            string
		name          string
		currentURL    string
		baseURL       string
		nonTokenQuery string
		enabled       int
		revision      string
	}
	var targetSubs []targetSub

	tRows, err := db.QueryContext(ctx, "SELECT id, name, source_url_secret_ref, enabled, revision FROM subscriptions;")
	if err != nil {
		return nil, fmt.Errorf("failed to query subscriptions in target database: %w", err)
	}
	defer tRows.Close()

	for tRows.Next() {
		var id, name, surl, rev string
		var enabled int
		if tRows.Scan(&id, &name, &surl, &enabled, &rev) == nil {
			targetSubs = append(targetSubs, targetSub{
				id:            id,
				name:          name,
				currentURL:    surl,
				baseURL:       strippedBaseURL(surl),
				nonTokenQuery: nonTokenQueryParams(surl),
				enabled:       enabled,
				revision:      rev,
			})
		}
	}
	tRows.Close()

	nowStr := domain.NowUTC().Format(time.RFC3339)
	res := &domain.SourceTokenRepairResult{
		RestoredSources: make([]string, 0, 2),
		RestoredStatus:  make(map[string]bool),
		RedactedURLs:    make(map[string]string),
		ExecutedAt:      domain.NowUTC(),
	}

	tx, err := db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to begin update transaction: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	for _, tgt := range targetSubs {
		// Only consider enabled sources that are proven broken (missing query token)
		if tgt.enabled != 1 || !isProvenBrokenURL(tgt.currentURL) {
			continue
		}

		// Look for unambiguous archive evidence match
		var matchingProofs []archivedEvidence
		for _, proof := range archiveProofs {
			if proof.BaseURL == tgt.baseURL && proof.NonTokenQuery == tgt.nonTokenQuery {
				// Also verify name lineage matches
				if proof.Name == tgt.name {
					matchingProofs = append(matchingProofs, proof)
				}
			}
		}

		if len(matchingProofs) == 0 {
			continue // No archive evidence for this source
		}
		if len(matchingProofs) > 1 {
			return nil, fmt.Errorf("ambiguous archive lineage proof for source %q (multiple matching credentials)", tgt.name)
		}

		proof := matchingProofs[0]

		// Idempotency: if current URL already equals restored URL, skip without incrementing revision or audit
		if tgt.currentURL == proof.FullURL {
			res.SkippedCount++
			continue
		}

		newRev := domain.MustNewUUIDv7()
		_, updateErr := tx.ExecContext(ctx, `
			UPDATE subscriptions
			SET source_url_secret_ref = ?,
			    revision = ?,
			    updated_at = ?
			WHERE id = ?;
		`, proof.FullURL, newRev, nowStr, tgt.id)
		if updateErr != nil {
			return nil, fmt.Errorf("failed to update subscription %s: %w", tgt.name, updateErr)
		}

		auditID := domain.MustNewUUIDv7()
		safeSummary := fmt.Sprintf("Restored full authenticated subscription URL for %s from archive evidence", tgt.name)
		_, auditErr := tx.ExecContext(ctx, `
			INSERT INTO audit_events (id, actor_kind, action, redacted_summary, result, created_at)
			VALUES (?, 'system', 'subscription.source_url.token_restored', ?, 'success', ?);
		`, auditID, safeSummary, nowStr)
		if auditErr != nil {
			return nil, fmt.Errorf("failed to record audit event for %s: %w", tgt.name, auditErr)
		}

		res.RestoredSources = append(res.RestoredSources, tgt.name)
		res.RestoredStatus[tgt.name] = true
		res.RedactedURLs[tgt.name] = RedactTokenURL(proof.FullURL)
		res.RestoredCount++
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit token restoration: %w", err)
	}
	tx = nil

	return res, nil
}
