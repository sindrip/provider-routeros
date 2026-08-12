package routeros

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, pass, ok := r.BasicAuth(); !ok || user != "admin" || pass != "" {
			t.Errorf("basic auth = %q %q", user, pass)
		}

		if r.URL.Path != "/rest/system/resource" {
			t.Errorf("path = %q", r.URL.Path)
		}

		w.Write([]byte(`{"version": "7.23.3 (stable)", "uptime": "1m"}`))
	}))
	defer srv.Close()

	rec, err := New(srv.URL, "admin", "").Get(context.Background(), "/system/resource")
	if err != nil {
		t.Fatal(err)
	}

	if rec["version"] != "7.23.3 (stable)" {
		t.Errorf("version = %q", rec["version"])
	}
}

func TestGetError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": 400, "message": "unknown path"}`))
	}))
	defer srv.Close()

	_, err := New(srv.URL, "admin", "").Get(context.Background(), "/nope")
	if err == nil || !strings.Contains(err.Error(), "unknown path") {
		t.Errorf("err = %v", err)
	}
}

func TestGetNonString(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"n": 1}`))
	}))
	defer srv.Close()

	if _, err := New(srv.URL, "admin", "").Get(context.Background(), "/x"); err == nil {
		t.Error("want decode error for non-string value")
	}
}
