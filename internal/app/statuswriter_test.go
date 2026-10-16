package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStatusWriterUnwrap(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: rec, status: http.StatusOK}

	var rw http.ResponseWriter = sw
	u, ok := rw.(interface{ Unwrap() http.ResponseWriter })
	if !ok {
		t.Fatal("statusWriter does not implement Unwrap() http.ResponseWriter")
	}
	if u.Unwrap() != rec {
		t.Fatalf("Unwrap() = %v, want %v", u.Unwrap(), rec)
	}
}
