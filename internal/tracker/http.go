package tracker

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

const MaxRequestBytes = 8 << 10

type handler struct {
	store   *Store
	keyHash [32]byte
	now     func() time.Time
}

// Handler refuses an unset/empty shared key instead of accepting empty keys.
func Handler(store *Store, key string) (http.Handler, error) {
	if key == "" {
		return nil, errors.New("PROD_BROKE_KEY must be set and nonempty")
	}
	h := &handler{store: store, keyHash: sha256.Sum256([]byte(key)), now: time.Now}
	return h, nil
}

func jsonResponse(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func malformed(w http.ResponseWriter) {
	jsonResponse(w, http.StatusBadRequest, map[string]string{"status": "Malformed", "error": "invalid JSON, deployment, date or key"})
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Match the single-origin policy from the former Next.js headers config.
	// Never reflect arbitrary Origin values or allow wildcard credentials.
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Access-Control-Allow-Origin", "https://zachmanson.com")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Methods", "GET,OPTIONS,PATCH,DELETE,POST,PUT")
		w.Header().Set("Access-Control-Allow-Headers", "X-CSRF-Token, X-Requested-With, Accept, Accept-Version, Content-Length, Content-MD5, Content-Type, Date, X-Api-Version")
	}
	allowed := "GET, HEAD, OPTIONS"
	switch r.URL.Path {
	case "/api/prod-broke":
		allowed = "GET, HEAD, OPTIONS, POST"
		if r.Method == http.MethodPost {
			h.create(w, r)
			return
		}
	case "/", "/healthz", "/api/prod-broke/messages":
	default:
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodOptions {
		w.Header().Set("Allow", allowed)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", allowed)
		jsonResponse(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if r.URL.Path == "/" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, homepage)
		return
	}
	if r.URL.Path == "/healthz" {
		if h.store.Healthy(r.Context()) != nil {
			jsonResponse(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
		jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	events, err := h.store.Latest(r.Context())
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": "database unavailable"})
		return
	}
	if r.URL.Path == "/api/prod-broke" {
		jsonResponse(w, http.StatusOK, events)
		return
	}
	messages := map[string]string{
		"PROD": "PROD has never had a broken deployment",
		"STG":  "STG has never had a broken deployment",
	}
	now := h.now().UnixMilli()
	for _, event := range events {
		date, _ := ParseDate(event.Date)
		// JS Math.floor, including negative/future times; not calendar days.
		days := int64(math.Floor(float64(now-date.UnixMilli()) / 86400000))
		messages[event.Deployment] = fmt.Sprintf("%d days since last broken %s deployment", days, event.Deployment)
	}
	jsonResponse(w, http.StatusOK, messages)
}

func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	// Preserve Next Request.json() compatibility: parse the bounded JSON body
	// even when legacy callers omit or mislabel the request Content-Type.
	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBytes)
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	// Exact field names, string types, and required fields match the old Zod
	// object; unrecognized fields are ignored, never echoed or logged.
	var fields map[string]json.RawMessage
	if err := dec.Decode(&fields); err != nil || fields == nil {
		malformed(w)
		return
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		malformed(w)
		return
	}
	var deployment, date, key string
	if !stringField(fields, "deployment", &deployment) || !stringField(fields, "date", &date) || !stringField(fields, "key", &key) || !validDeployment(deployment) {
		malformed(w)
		return
	}
	parsed, err := ParseDate(date)
	if err != nil {
		malformed(w)
		return
	}
	provided := sha256.Sum256([]byte(key))
	if subtle.ConstantTimeCompare(provided[:], h.keyHash[:]) != 1 {
		jsonResponse(w, http.StatusUnauthorized, map[string]string{"error": "Password incorrect"})
		return
	}
	event, err := h.store.Create(r.Context(), deployment, parsed)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": "database unavailable"})
		return
	}
	jsonResponse(w, http.StatusOK, struct {
		Success bool  `json:"success"`
		Data    Event `json:"data"`
	}{true, event})
}

func stringField(fields map[string]json.RawMessage, name string, target *string) bool {
	value, ok := fields[name]
	return ok && len(value) > 0 && value[0] == '"' && json.Unmarshal(value, target) == nil
}
