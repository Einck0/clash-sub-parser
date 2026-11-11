package sqlite

import (
	"context"
	"database/sql"
	"strings"
)

// compactionUniqueOrderPrefix returns zero unless an ordinary non-partial
// unique index proves a BINARY canonical prefix unique for this snapshot.
// Nullable UNIQUE keys need an actual NULL check: SQLite permits multiple NULLs.
// Expressions, descending keys and other collations deliberately fall back.
func compactionUniqueOrderPrefix(ctx context.Context, tx *sql.Tx, table string, columns []string) (int, error) {
	rows, err := tx.QueryContext(ctx, `SELECT name FROM pragma_index_list(?) WHERE "unique"=1 AND partial=0`, table)
	if err != nil {
		return 0, err
	}
	var indexes []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return 0, err
		}
		indexes = append(indexes, name)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	best := 0
	for _, index := range indexes {
		rows, err := tx.QueryContext(ctx, `SELECT cid,name,"desc",coll FROM pragma_index_xinfo(?) WHERE "key"=1 ORDER BY seqno`, index)
		if err != nil {
			return 0, err
		}
		valid, prefix := true, 0
		for rows.Next() {
			var cid, desc int
			var name, collation sql.NullString
			if err := rows.Scan(&cid, &name, &desc, &collation); err != nil {
				rows.Close()
				return 0, err
			}
			if cid < 0 || prefix >= len(columns) || !name.Valid || name.String != columns[prefix] || desc != 0 || !collation.Valid || collation.String != "BINARY" {
				valid = false
			}
			prefix++
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return 0, err
		}
		if !valid || prefix == 0 || (best != 0 && best <= prefix) {
			continue
		}
		nulls := make([]string, prefix)
		for i := range nulls {
			nulls[i] = quoteCompactionIdentifier(columns[i]) + " IS NULL"
		}
		var hasRows, hasNull int
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM `+quoteCompactionIdentifier(table)+`),
 EXISTS(SELECT 1 FROM `+quoteCompactionIdentifier(table)+` WHERE `+strings.Join(nulls, " OR ")+`)`).Scan(&hasRows, &hasNull); err != nil {
			return 0, err
		}
		if hasRows != 0 && hasNull == 0 {
			best = prefix
		}
	}
	return best, nil
}
