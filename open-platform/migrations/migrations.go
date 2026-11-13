package migrations

import (
	"context"
	"database/sql"
	_ "embed"
)

//go:embed 001_device_authorization.sql
var schema string

//go:embed 002_grant_scopes.sql
var grantScopes string

func Apply(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(18843002)"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, schema); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, grantScopes); err != nil {
		return err
	}
	return tx.Commit()
}
