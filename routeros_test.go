package routeros

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetShapes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, pass, ok := r.BasicAuth(); !ok || user != "admin" || pass != "" {
			t.Errorf("basic auth = %q %q", user, pass)
		}

		switch r.URL.Path {
		case "/rest/ip/dns":
			w.Write([]byte(`{"servers": "1.1.1.1"}`))
		case "/rest/ip/firewall/filter":
			w.Write([]byte(`[{".id": "*1"}, {".id": "*2"}]`))
		default:
			t.Errorf("path = %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "admin", "")
	ctx := context.Background()

	rec, err := c.Get[map[string]string](ctx, "/ip/dns")
	if err != nil || rec["servers"] != "1.1.1.1" {
		t.Errorf("singleton = %v, %v", rec, err)
	}

	rows, err := c.Get[[]map[string]string](ctx, "/ip/firewall/filter")
	if err != nil || len(rows) != 2 || rows[1][".id"] != "*2" {
		t.Errorf("rows = %v, %v", rows, err)
	}

	raw, err := c.Get[jsontext.Value](ctx, "/ip/firewall/filter")
	if err != nil || raw.Kind() != '[' {
		t.Errorf("raw kind = %v, %v", raw.Kind(), err)
	}
}

func TestError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": 400, "message": "unknown path"}`))
	}))
	defer srv.Close()

	_, err := New(srv.URL, "admin", "").Get[map[string]string](context.Background(), "/nope")

	var re *Error
	if !errors.As(err, &re) || re.Status != 400 || re.Message != "unknown path" {
		t.Errorf("err = %v", err)
	}
}

// TestMutations pins the request shapes the live router answered on
// 2026-08-13: PUT creates with 201, PATCH updates, DELETE answers 204.
func TestMutations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "PUT /rest/ip/firewall/address-list":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{".id": "*1", "address": "10.9.9.9", "list": "probe"}`))
		case "PATCH /rest/ip/firewall/address-list/*1":
			w.Write([]byte(`{".id": "*1", "comment": "probed"}`))
		case "PATCH /rest/ip/dns":
			w.Write([]byte(`{"servers": "1.1.1.1"}`))
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

	created, err := c.Put[map[string]string](ctx, "/ip/firewall/address-list", map[string]string{"address": "10.9.9.9"})
	if err != nil || created[".id"] != "*1" {
		t.Errorf("Put = %v, %v", created, err)
	}

	row, err := c.Patch[map[string]string](ctx, "/ip/firewall/address-list/*1", map[string]string{"comment": "probed"})
	if err != nil || row["comment"] != "probed" {
		t.Errorf("Patch row = %v, %v", row, err)
	}

	single, err := c.Patch[map[string]string](ctx, "/ip/dns", map[string]string{"servers": "1.1.1.1"})
	if err != nil || single["servers"] != "1.1.1.1" {
		t.Errorf("Patch singleton = %v, %v", single, err)
	}

	if err := c.Delete(ctx, "/ip/firewall/address-list/*1"); err != nil {
		t.Errorf("Delete = %v", err)
	}
}

func TestNonStringValue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"n": 1}`))
	}))
	defer srv.Close()

	if _, err := New(srv.URL, "admin", "").Get[map[string]string](context.Background(), "/x"); err == nil {
		t.Error("want decode error for non-string value")
	}
}
