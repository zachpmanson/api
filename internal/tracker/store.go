package tracker

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const dateLayout = "2006-01-02T15:04:05.000Z"

// Event is the public representation used by the previous Prisma API.
type Event struct {
	ID         string `json:"id"`
	Date       string `json:"date"`
	Deployment string `json:"deployment"`
}

type Store struct{ db *sql.DB }

// Open never resets an existing database. Only schema version 0 -> 1 is supported.
func Open(path string) (*Store, error) {
	if path == "" || path == ":memory:" {
		return nil, errors.New("a persistent database path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(abs, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: abs}
	q := u.Query()
	q.Set("_busy_timeout", "5000")
	q.Set("_journal_mode", "WAL")
	q.Set("_synchronous", "FULL")
	q.Set("_foreign_keys", "on")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite3", u.String())
	if err != nil {
		return nil, err
	}
	// Serialize this tiny workload and bound connection/cache memory. The busy
	// timeout also permits a separate backup or import process to hold the lock.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err := tx.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version != 0 && version != 1 {
		return errors.New("unsupported newer database schema")
	}
	if version == 0 {
		// No IF NOT EXISTS: an unversioned, pre-existing table must not be
		// silently adopted as a known schema.
		_, err = tx.Exec(`
CREATE TABLE broken_deployments (
  id TEXT NOT NULL PRIMARY KEY CHECK (
    length(id) = 36 AND substr(id,9,1) = '-' AND substr(id,14,1) = '-'
    AND substr(id,19,1) = '-' AND substr(id,24,1) = '-'
    AND length(replace(id,'-','')) = 32 AND id NOT GLOB '*[^0-9a-fA-F-]*'
  ),
  date_ms INTEGER NOT NULL CHECK (date_ms BETWEEN -62167219200000 AND 253402300799999),
  deployment TEXT NOT NULL CHECK (deployment IN ('PROD', 'STG'))
) STRICT;
CREATE INDEX broken_deployments_latest ON broken_deployments(deployment, date_ms DESC, id ASC);
PRAGMA user_version = 1;`)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Healthy(ctx context.Context) error {
	var one int
	return s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM broken_deployments LIMIT 1)").Scan(&one)
}

var rfc3339 = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T(?:[01]\d|2[0-3]):[0-5]\d:[0-5]\d(?:\.\d+)?(?:Z|[+-](?:[01]\d|2[0-3]):[0-5]\d)$`)
var dateOnly = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// ParseDate deliberately supports unambiguous RFC3339 and ISO date-only input.
// Normalize to Prisma's UTC millisecond representation (also for imports).
func ParseDate(value string) (time.Time, error) {
	var t time.Time
	var err error
	switch {
	case dateOnly.MatchString(value):
		t, err = time.Parse("2006-01-02", value)
	case rfc3339.MatchString(strings.ToUpper(value)):
		t, err = time.Parse(time.RFC3339Nano, strings.ToUpper(value))
	default:
		return time.Time{}, errors.New("invalid date")
	}
	if err != nil || t.UTC().Year() < 0 || t.UTC().Year() > 9999 {
		return time.Time{}, errors.New("invalid date")
	}
	return time.UnixMilli(t.UnixMilli()).UTC(), nil
}

func validUUID(id string) bool {
	if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		return false
	}
	raw := id[:8] + id[9:13] + id[14:18] + id[19:23] + id[24:]
	_, err := hex.DecodeString(raw)
	return err == nil
}

func validDeployment(value string) bool { return value == "PROD" || value == "STG" }

func (s *Store) Create(ctx context.Context, deployment string, date time.Time) (Event, error) {
	if !validDeployment(deployment) {
		return Event{}, errors.New("invalid deployment")
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return Event{}, err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	id := fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
	_, err := s.db.ExecContext(ctx, "INSERT INTO broken_deployments(id,date_ms,deployment) VALUES(?,?,?)", id, date.UnixMilli(), deployment)
	return Event{ID: id, Date: date.UTC().Format(dateLayout), Deployment: deployment}, err
}

func (s *Store) Latest(ctx context.Context) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, date_ms, deployment FROM broken_deployments AS b
WHERE NOT EXISTS (SELECT 1 FROM broken_deployments AS newer
 WHERE newer.deployment = b.deployment AND
 (newer.date_ms > b.date_ms OR (newer.date_ms = b.date_ms AND newer.id < b.id)))
ORDER BY deployment ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]Event, 0, 2)
	for rows.Next() {
		var event Event
		var ms int64
		if err := rows.Scan(&event.ID, &ms, &event.Deployment); err != nil {
			return nil, err
		}
		event.Date = time.UnixMilli(ms).UTC().Format(dateLayout)
		events = append(events, event)
	}
	return events, rows.Err()
}
