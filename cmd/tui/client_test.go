package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchEntriesSuccess(t *testing.T) {
	resp := APIResponse{
		Timestamp:             "2025-01-15T13:53:57Z",
		ObservationWindowSecs: 60,
		Entries: []Entry{
			{Pod: "test-pod", Container: "app", SecretPath: "/token", ReadPerSec: 1.5, Cached: true, LastRead: "2025-01-15T13:53:57Z"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/secret-access" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	got, err := c.FetchEntries()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(got.Entries))
	}
	if got.Entries[0].Pod != "test-pod" {
		t.Errorf("pod=%q, want test-pod", got.Entries[0].Pod)
	}
	if got.Entries[0].ReadPerSec != 1.5 {
		t.Errorf("reads=%.2f, want 1.50", got.Entries[0].ReadPerSec)
	}
}

func TestFetchEntriesEmptyResponse(t *testing.T) {
	resp := APIResponse{
		Timestamp:             "2025-01-15T13:53:57Z",
		ObservationWindowSecs: 60,
		Entries:               []Entry{},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	got, err := c.FetchEntries()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Entries) != 0 {
		t.Errorf("got %d entries, want 0", len(got.Entries))
	}
}

func TestFetchEntriesServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.FetchEntries()
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestFetchEntriesConnectionRefused(t *testing.T) {
	c := NewClient("http://localhost:1") // nothing listening
	_, err := c.FetchEntries()
	if err == nil {
		t.Error("expected error for connection refused")
	}
}

func TestFetchEntriesBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.FetchEntries()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestFetchEntriesMatchesMockAPIFormat(t *testing.T) {
	// Simulate the exact format the mock-api.py returns
	mockResp := `{
		"timestamp": "2025-01-15T13:53:57Z",
		"observation_window_seconds": 60,
		"entries": [
			{
				"pod": "payment-service-7f8b9c6d4-xk2mn",
				"container": "payment-svc",
				"secret_path": "/var/run/secrets/kubernetes.io/serviceaccount/token",
				"reads_per_sec": 4872.3,
				"last_read": "2025-01-15T13:53:57Z",
				"cached": false
			},
			{
				"pod": "payment-service-7f8b9c6d4-xk2mn",
				"container": "payment-svc",
				"secret_path": "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
				"reads_per_sec": 0.12,
				"last_read": "2025-01-15T13:53:57Z",
				"cached": true
			}
		]
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(mockResp))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	got, err := c.FetchEntries()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.ObservationWindowSecs != 60 {
		t.Errorf("observation_window=%d, want 60", got.ObservationWindowSecs)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(got.Entries))
	}

	e := got.Entries[0]
	if e.Pod != "payment-service-7f8b9c6d4-xk2mn" {
		t.Errorf("pod=%q", e.Pod)
	}
	if e.ReadPerSec != 4872.3 {
		t.Errorf("reads=%.1f, want 4872.3", e.ReadPerSec)
	}
	if e.Cached {
		t.Error("first entry should not be cached")
	}
	if e.LastRead != "2025-01-15T13:53:57Z" {
		t.Errorf("last_read=%q", e.LastRead)
	}

	e2 := got.Entries[1]
	if !e2.Cached {
		t.Error("second entry should be cached")
	}
}