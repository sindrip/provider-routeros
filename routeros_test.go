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

// TestMutations pins the request shapes the live router answered on
// 2026-08-13: PUT creates, PATCH by id updates, DELETE by id answers 204.
func TestMutations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "PUT /rest/ip/firewall/address-list":
			w.Write([]byte(`{".id": "*1", "address": "10.9.9.9", "list": "probe"}`))
		case "PATCH /rest/ip/firewall/address-list/*1":
			w.Write([]byte(`{".id": "*1", "address": "10.9.9.9", "comment": "probed"}`))
		case "DELETE /rest/ip/firewall/address-list/*1":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "admin", "")
	ctx := context.Background()

	created, err := c.Add(ctx, "/ip/firewall/address-list", map[string]string{"address": "10.9.9.9", "list": "probe"})
	if err != nil || created[".id"] != "*1" {
		t.Errorf("Add = %v, %v", created, err)
	}

	updated, err := c.Set(ctx, "/ip/firewall/address-list", "*1", map[string]string{"comment": "probed"})
	if err != nil || updated["comment"] != "probed" {
		t.Errorf("Set = %v, %v", updated, err)
	}

	if err := c.Remove(ctx, "/ip/firewall/address-list", "*1"); err != nil {
		t.Errorf("Remove = %v", err)
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
