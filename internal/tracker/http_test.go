package tracker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func testHandler(t *testing.T) (*handler, *Store) {
	t.Helper()
	s := testStore(t)
	h, err := Handler(s, "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	return h.(*handler), s
}

func request(h http.Handler, method, path, body, contentType string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestEndpointContract(t *testing.T) {
	h, s := testHandler(t)
	w := request(h, "GET", "/api/prod-broke", "", "")
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, body := range []string{
		`{"deployment":"STG","date":"2024-06-02T06:30:00.123456+05:30","key":"test-secret","unknown":"ignored"}`,
		`{"deployment":"PROD","date":"2024-06-01","key":"test-secret"}`,
		`{"deployment":"PROD","date":"2023-01-01","key":"test-secret"}`,
	} {
		w = request(h, "POST", "/api/prod-broke", body, "application/json; charset=utf-8")
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var response struct {
			Success bool  `json:"success"`
			Data    Event `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || !response.Success || !validUUID(response.Data.ID) {
			t.Fatal(response, err)
		}
		if strings.Contains(w.Body.String(), "secret") || strings.Contains(w.Body.String(), "key") {
			t.Fatal("credential leaked")
		}
	}
	w = request(h, "GET", "/api/prod-broke?pretty=true", "", "")
	var events []Event
	if err := json.Unmarshal(w.Body.Bytes(), &events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Deployment != "PROD" || events[0].Date != "2024-06-01T00:00:00.000Z" || events[1].Date != "2024-06-02T01:00:00.123Z" {
		t.Fatal(events)
	}
	if countRows(t, s) != 3 {
		t.Fatal("latest query destroyed history")
	}
	if !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Access-Control-Allow-Origin") != "https://zachmanson.com" {
		t.Fatal(w.Header())
	}
	var objects []map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &objects)
	for _, object := range objects {
		if len(object) != 3 {
			t.Fatal("event fields changed", object)
		}
	}
}

func TestBadRequestsDoNotWrite(t *testing.T) {
	h, s := testHandler(t)
	for _, tc := range []struct {
		body   string
		status int
	}{
		{"", 400}, {"{", 400}, {"null", 400}, {"[]", 400}, {"{}", 400},
		{`{"deployment":"PROD","date":"2024-06-01","key":"wrong"}`, 401},
		{`{"deployment":"PROD","date":"2024-06-01","key":""}`, 401},
		{`{"deployment":"PROD","date":"2024-06-01"}`, 400},
		{`{"deployment":"PROD","date":"2024-06-01","key":null}`, 400},
		{`{"deployment":"PROD","date":null,"key":"test-secret"}`, 400},
		{`{"deployment":"PROD","date":1717200000000,"key":"test-secret"}`, 400},
		{`{"deployment":"prod","date":"2024-06-01","key":"test-secret"}`, 400},
		{`{"Deployment":"PROD","date":"2024-06-01","key":"test-secret"}`, 400},
		{`{"deployment":"PROD","date":"2024-02-30","key":"test-secret"}`, 400},
		{`{"deployment":"PROD","date":"invalid","key":"wrong"}`, 400},
		{`{"deployment":"PROD","date":"2024-06-01","key":"test-secret"} {}`, 400},
		{`{"deployment":"PROD","date":"2024-06-01","key":"test-secret","extra":"` + strings.Repeat("x", MaxRequestBytes) + `"}`, 400},
	} {
		w := request(h, "POST", "/api/prod-broke", tc.body, "application/json")
		if w.Code != tc.status {
			t.Errorf("%d-byte body: got %d want %d (%s)", len(tc.body), w.Code, tc.status, w.Body.String())
		}
		if !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
			t.Fatal(w.Header())
		}
	}
	for _, contentType := range []string{"", "text/plain", "application/json; invalid"} {
		if w := request(h, "POST", "/api/prod-broke", `{}`, contentType); w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	if countRows(t, s) != 0 {
		t.Fatal("bad request wrote event")
	}
	if _, err := Handler(s, ""); err == nil {
		t.Fatal("unset key accepted")
	}
}

func TestLegacyCORSAndHead(t *testing.T) {
	h, s := testHandler(t)
	for _, path := range []string{"/api/prod-broke", "/api/prod-broke/messages"} {
		for _, origin := range []string{"https://zachmanson.com", "https://untrusted.example"} {
			r := httptest.NewRequest(http.MethodOptions, path, nil)
			r.Header.Set("Origin", origin)
			r.Header.Set("Access-Control-Request-Method", "POST")
			r.Header.Set("Access-Control-Request-Headers", "Content-Type")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != http.StatusNoContent || w.Body.Len() != 0 {
				t.Fatalf("preflight failed: %d", w.Code)
			}
			if w.Header().Get("Access-Control-Allow-Origin") != "https://zachmanson.com" || w.Header().Get("Access-Control-Allow-Credentials") != "true" {
				t.Fatal("legacy origin policy changed", w.Header())
			}
			if w.Header().Get("Access-Control-Allow-Origin") == origin && origin != "https://zachmanson.com" {
				t.Fatal("untrusted origin was reflected")
			}
			if !strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), "Content-Type") {
				t.Fatal("JSON preflight headers missing")
			}
		}
	}
	server := httptest.NewServer(h)
	defer server.Close()
	for _, path := range []string{"/", "/api/prod-broke", "/api/prod-broke/messages"} {
		response, err := http.Head(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK || len(body) != 0 {
			t.Fatalf("HEAD contract changed for %s: %d, body bytes %d", path, response.StatusCode, len(body))
		}
	}
	if countRows(t, s) != 0 {
		t.Fatal("preflight/HEAD wrote history")
	}
}

func TestJSONRequestHeaderCompatibility(t *testing.T) {
	for _, contentType := range []string{"", "text/plain", "text/json", "application/json; invalid", "application/json"} {
		t.Run(contentType, func(t *testing.T) {
			h, s := testHandler(t)
			w := request(h, "POST", "/api/prod-broke", `{"deployment":"PROD","date":"2024-06-01","key":"test-secret"}`, contentType)
			if w.Code != http.StatusOK || countRows(t, s) != 1 {
				t.Fatalf("legacy JSON request rejected: status %d", w.Code)
			}
			if !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
				t.Fatal("response must still be JSON")
			}
		})
	}
}

func TestMessagesElapsedDays(t *testing.T) {
	h, s := testHandler(t)
	h.now = func() time.Time { return time.Date(2024, 6, 2, 0, 0, 0, 0, time.UTC) }
	w := request(h, "GET", "/api/prod-broke/messages", "", "")
	var messages map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &messages); err != nil {
		t.Fatal(err)
	}
	if messages["PROD"] != "PROD has never had a broken deployment" || messages["STG"] != "STG has never had a broken deployment" {
		t.Fatal(messages)
	}
	for _, tc := range []struct{ date, want string }{
		{"2024-05-30T19:00:00-05:00", "2 days since last broken PROD deployment"},
		{"2024-06-01T00:00:00.001Z", "0 days since last broken PROD deployment"},
		{"2024-06-02T00:00:00.001Z", "-1 days since last broken PROD deployment"},
		{"2024-06-03T00:00:00.001Z", "-2 days since last broken PROD deployment"},
	} {
		date, _ := ParseDate(tc.date)
		if _, err := s.Create(context.Background(), "PROD", date); err != nil {
			t.Fatal(err)
		}
		w = request(h, "GET", "/api/prod-broke/messages", "", "")
		_ = json.Unmarshal(w.Body.Bytes(), &messages)
		if messages["PROD"] != tc.want || messages["STG"] != "STG has never had a broken deployment" {
			t.Fatal(messages)
		}
	}
}

func TestRoutesMethodsAndHealth(t *testing.T) {
	h, s := testHandler(t)
	for _, path := range []string{"/", "/api/prod-broke", "/api/prod-broke/messages", "/healthz"} {
		for _, method := range []string{"PUT", "DELETE", "PATCH"} {
			w := request(h, method, path, "", "")
			if w.Code != 405 || w.Header().Get("Allow") == "" {
				t.Fatal(method, path, w.Code, w.Header())
			}
		}
	}
	if w := request(h, "POST", "/api/prod-broke/messages", "", ""); w.Code != 405 {
		t.Fatal(w.Code)
	}
	if w := request(h, "GET", "/unknown", "", ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w := request(h, "GET", "/", "", ""); w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") || !strings.Contains(w.Body.String(), "/api/prod-broke/messages") {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request(h, "GET", "/healthz", "", ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	_ = s.Close()
	for _, path := range []string{"/api/prod-broke", "/api/prod-broke/messages", "/healthz"} {
		w := request(h, "GET", path, "", "")
		want := 500
		if path == "/healthz" {
			want = 503
		}
		if w.Code != want {
			t.Fatal(path, w.Code)
		}
	}
}

func TestConcurrentHTTP(t *testing.T) {
	h, s := testHandler(t)
	server := httptest.NewServer(h)
	defer server.Close()
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := fmt.Sprintf(`{"deployment":"PROD","date":"2024-06-%02d","key":"test-secret"}`, i+1)
			res, err := http.Post(server.URL+"/api/prod-broke", "application/json", strings.NewReader(body))
			if err != nil {
				t.Error(err)
				return
			}
			defer res.Body.Close()
			if res.StatusCode != 200 {
				t.Error(res.StatusCode)
			}
		}(i)
	}
	wg.Wait()
	if countRows(t, s) != 12 {
		t.Fatal("lost HTTP writes")
	}
}
