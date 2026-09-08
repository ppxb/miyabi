package ent

import (
	"context"
	"fmt"
)

// WithTx follows ent's transaction helper: commit the callback's writes together
// and roll them back on either an error or a panic.
func WithTx(ctx context.Context, client *Client, fn func(*Tx) error) error {
	tx, err := client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = tx.Rollback()
			panic(recovered)
		}
	}()
	if err := fn(tx); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return fmt.Errorf("%w: rollback transaction: %v", err, rollbackErr)
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}
