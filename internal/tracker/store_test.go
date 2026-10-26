package tracker

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "api.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func countRows(t *testing.T, s *Store) int {
	t.Helper()
	var count int
	if err := s.db.QueryRow("SELECT count(*) FROM broken_deployments").Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestSchemaAndConfiguration(t *testing.T) {
	s := testStore(t)
	for pragma, want := range map[string]string{"journal_mode": "wal", "busy_timeout": "5000", "synchronous": "2", "foreign_keys": "1", "user_version": "1"} {
		var got string
		if err := s.db.QueryRow("PRAGMA " + pragma).Scan(&got); err != nil || got != want {
			t.Errorf("%s = %q, %v; want %q", pragma, got, err, want)
		}
	}
	if s.db.Stats().MaxOpenConnections != 1 {
		t.Fatal("unbounded DB pool")
	}
	var index string
	if err := s.db.QueryRow("SELECT name FROM sqlite_master WHERE type='index' AND name='broken_deployments_latest'").Scan(&index); err != nil {
		t.Fatal("latest index missing", err)
	}
	id := "11111111-1111-4111-8111-111111111111"
	for _, values := range []struct {
		id, deployment string
		date           any
	}{
		{"invalid", "PROD", 0}, {id, "DEV", 0}, {id, "PROD", "not-an-integer"}, {id, "PROD", nil}, {id, "PROD", int64(253402300800000)}, {id, "PROD", int64(-62167219200001)},
	} {
		if _, err := s.db.Exec("INSERT INTO broken_deployments VALUES(?,?,?)", values.id, values.date, values.deployment); err == nil {
			t.Errorf("constraint accepted %+v", values)
		}
	}
	if err := s.Healthy(context.Background()); err != nil {
		t.Fatal("empty database should be healthy", err)
	}
}

func TestPersistenceAndStableLatest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.sqlite")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	fixture := `[
{"id":"33333333-3333-4333-8333-333333333333","date":"2024-06-01","deployment":"STG"},
{"id":"22222222-2222-4222-8222-222222222222","date":"2024-06-01","deployment":"PROD"},
{"id":"11111111-1111-4111-8111-111111111111","date":"2024-06-01","deployment":"PROD"},
{"id":"00000000-0000-4000-8000-000000000000","date":"2024-05-31","deployment":"PROD"}]`
	if n, err := s.Import(context.Background(), strings.NewReader(fixture)); n != 4 || err != nil {
		t.Fatal(n, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	latest, err := s.Latest(context.Background())
	if err != nil || len(latest) != 2 {
		t.Fatal(latest, err)
	}
	if latest[0].ID != "11111111-1111-4111-8111-111111111111" || latest[0].Deployment != "PROD" || latest[1].Deployment != "STG" || latest[0].Date != "2024-06-01T00:00:00.000Z" {
		t.Fatal(latest)
	}
	if countRows(t, s) != 4 {
		t.Fatal("history lost on reopen")
	}
}

func TestConcurrentWritesAndReads(t *testing.T) {
	s := testStore(t)
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			e, err := s.Create(context.Background(), "PROD", time.UnixMilli(int64(i)))
			if err != nil || !validUUID(e.ID) {
				t.Error(e, err)
				return
			}
			if _, err := s.Latest(context.Background()); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if countRows(t, s) != 24 {
		t.Fatal("writes lost")
	}
}

func TestBusyWriterWaits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.sqlite")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	other, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec("INSERT INTO broken_deployments VALUES('11111111-1111-4111-8111-111111111111',0,'PROD')"); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		_, err := other.Create(context.Background(), "STG", time.Now())
		done <- err
	}()
	<-started
	select {
	case err := <-done:
		t.Fatalf("writer did not wait for lock: %v", err)
	case <-time.After(75 * time.Millisecond):
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestMigrationRejectsUnknownSchemaWithoutReset(t *testing.T) {
	for _, version := range []int{0, 2} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.sqlite")
			db, err := sql.Open("sqlite3", path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.Exec(fmt.Sprintf("CREATE TABLE broken_deployments (sentinel TEXT); INSERT INTO broken_deployments VALUES('keep'); PRAGMA user_version=%d", version)); err != nil {
				t.Fatal(err)
			}
			if s, err := Open(path); err == nil {
				s.Close()
				t.Fatal("accepted unknown schema")
			}
			var sentinel string
			if err := db.QueryRow("SELECT sentinel FROM broken_deployments").Scan(&sentinel); err != nil || sentinel != "keep" {
				t.Fatal("existing data changed", sentinel, err)
			}
		})
	}
}
