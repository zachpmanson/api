package tracker

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func importText(t *testing.T, s *Store, text string) (int, error) {
	t.Helper()
	return s.Import(context.Background(), strings.NewReader(text))
}

func TestImportFixturePreservationAndRepeat(t *testing.T) {
	s := testStore(t)
	data, err := os.ReadFile("testdata/history.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture []Event
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if n, err := importText(t, s, string(data)); err != nil || n != len(fixture) {
		t.Fatal(n, err)
	}
	if n, err := importText(t, s, string(data)); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	for _, event := range fixture {
		var date int64
		var deployment string
		if err := s.db.QueryRow("SELECT date_ms,deployment FROM broken_deployments WHERE id=?", event.ID).Scan(&date, &deployment); err != nil {
			t.Fatal(err)
		}
		want, _ := ParseDate(event.Date)
		if date != want.UnixMilli() || deployment != event.Deployment {
			t.Fatal("import changed data")
		}
	}
	// An equivalent offset date is the same instant, not a conflict.
	if n, err := importText(t, s, `[{"id":"11111111-1111-4111-8111-111111111111","date":"2024-06-01T03:00:00+03:00","deployment":"PROD"}]`); err != nil || n != 0 {
		t.Fatal(n, err)
	}
}

func TestImportConflictRollsBackEarlierNewRows(t *testing.T) {
	for _, conflict := range []string{
		`{"id":"11111111-1111-4111-8111-111111111111","date":"2024-06-02","deployment":"PROD"}`,
		`{"id":"11111111-1111-4111-8111-111111111111","date":"2024-06-01","deployment":"STG"}`,
	} {
		s := testStore(t)
		original := `{"id":"11111111-1111-4111-8111-111111111111","date":"2024-06-01","deployment":"PROD"}`
		if _, err := importText(t, s, "["+original+"]"); err != nil {
			t.Fatal(err)
		}
		newEvent := `{"id":"22222222-2222-4222-8222-222222222222","date":"2024-06-02","deployment":"STG"}`
		if n, err := importText(t, s, "["+newEvent+","+conflict+"]"); err == nil || n != 0 {
			t.Fatal("accepted conflict", n, err)
		}
		if countRows(t, s) != 1 {
			t.Fatal("partial writes on conflict")
		}
		latest, _ := s.Latest(context.Background())
		if latest[0].Deployment != "PROD" || latest[0].Date != "2024-06-01T00:00:00.000Z" {
			t.Fatal("replaced existing row")
		}
	}
}

func TestImportMalformedAllOrNothing(t *testing.T) {
	valid := `{"id":"11111111-1111-4111-8111-111111111111","date":"2024-06-01","deployment":"PROD"}`
	for _, text := range []string{
		"", "null", "{}", "[", "[null]", "[1]", "[{}]", "[] {}", "[" + valid + ",]", "[" + valid + "," + valid + "]",
		"[" + valid + `,{"id":"bad","date":"2024-06-01","deployment":"STG"}]`,
		"[" + valid + `,{"id":"22222222-2222-4222-8222-222222222222","date":"invalid","deployment":"STG"}]`,
		"[" + valid + `,{"id":"22222222-2222-4222-8222-222222222222","date":"2024-06-01","deployment":"DEV"}]`,
		`[{"id":"11111111-1111-4111-8111-111111111111","date":"2024-06-01","deployment":"PROD","key":"not-permitted"}]`,
		strings.Repeat(" ", MaxImportBytes+1),
	} {
		s := testStore(t)
		if n, err := importText(t, s, text); err == nil || n != 0 {
			t.Errorf("accepted malformed import (%d bytes): %d %v", len(text), n, err)
		}
		if countRows(t, s) != 0 {
			t.Fatal("partial writes from malformed file")
		}
	}
	s := testStore(t)
	if n, err := importText(t, s, "[]"); err != nil || n != 0 {
		t.Fatal(n, err)
	}
}
