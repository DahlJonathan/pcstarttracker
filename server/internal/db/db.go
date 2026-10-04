package db

import (
	"database/sql"
	_ "embed"
	"fmt"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

// Open opens (and if needed creates) the SQLite database at path, enables WAL
// mode and applies the schema. The returned *sql.DB is safe for concurrent use.
func Open(path string) (*sql.DB, error) {
	// _pragma params ensure WAL + busy timeout even before schema runs.
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)", path)
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// SQLite writes are serialized; a single writer connection avoids lock churn.
	database.SetMaxOpenConns(1)

	if err := database.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if _, err := database.Exec(schema); err != nil {
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return database, nil
}
