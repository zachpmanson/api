package tracker

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
)

const MaxImportBytes = 16 << 20

// Import accepts a JSON array of complete Event objects. It validates the whole
// file before writing, then inserts or verifies each ID in a single transaction.
// Existing rows are never updated. Re-importing the same file is a no-op.
func (s *Store) Import(ctx context.Context, r io.Reader) (int, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxImportBytes+1))
	if err != nil || len(data) > MaxImportBytes {
		return 0, errors.New("cannot read import or import exceeds 16 MiB")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var events []Event
	if err := dec.Decode(&events); err != nil || events == nil {
		return 0, errors.New("import must be an array of event objects")
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return 0, errors.New("unexpected trailing import data")
	}
	seen := make(map[string]bool, len(events))
	millis := make([]int64, len(events))
	for i, event := range events {
		date, err := ParseDate(event.Date)
		if !validUUID(event.ID) || !validDeployment(event.Deployment) || err != nil {
			return 0, errors.New("invalid import event")
		}
		if seen[event.ID] {
			return 0, errors.New("duplicate ID in import file")
		}
		seen[event.ID] = true
		millis[i] = date.UnixMilli()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, errors.New("cannot start import transaction")
	}
	defer tx.Rollback()
	inserted := 0
	for i, event := range events {
		result, err := tx.ExecContext(ctx, `INSERT INTO broken_deployments(id,date_ms,deployment)
VALUES(?,?,?) ON CONFLICT(id) DO NOTHING`, event.ID, millis[i], event.Deployment)
		if err != nil {
			return 0, errors.New("cannot insert import event")
		}
		n, err := result.RowsAffected()
		if err != nil {
			return 0, errors.New("cannot verify import insert")
		}
		if n == 1 {
			inserted++
			continue
		}
		var ms int64
		var deployment string
		err = tx.QueryRowContext(ctx, "SELECT date_ms,deployment FROM broken_deployments WHERE id=?", event.ID).Scan(&ms, &deployment)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, errors.New("cannot verify existing event")
		}
		if err != nil || ms != millis[i] || deployment != event.Deployment {
			return 0, errors.New("import conflicts with existing history; no rows imported")
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, errors.New("cannot commit import")
	}
	return inserted, nil
}
