package rest

import (
	"context"
	"encoding/json/v2"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"
	"time"
)

// fakeRouter answers like RouterOS does, including the parts that are
// surprising: errors shaped like records with a numeric "error", creation on
// PUT rather than POST, and a bare object from a singleton menu.
// addressField is spelled often enough across the tests to be worth naming.
const addressField = "address"

type fakeRouter struct {
	t        *testing.T
	lastBody map[string]any
	lastVerb string
	lastPath string
}

// serve stands the fake up on httptest's in-memory network rather than a real
// socket, and ties its lifetime to the test.
func (f *fakeRouter) serve(t *testing.T) *httptest.Server {
	return httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.lastVerb, f.lastPath = r.Method, r.URL.Path
		f.lastBody = nil
		if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
			_ = json.Unmarshal(raw, &f.lastBody)
		}
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/rest/system/resource":
			// A singleton answers with a bare object, not an array.
			writeJSON(f.t, w, http.StatusOK, map[string]string{"cpu-load": "0", "uptime": "1m20s"})

		case r.URL.Path == "/rest/system/routerboard":
			// The error body decodes exactly like a record, and "error" is a
			// number — which is why map[string]string fails first.
			writeRaw(w, http.StatusBadRequest,
				`{"detail":"no such command or directory (routerboard)","error":400,"message":"Bad Request"}`)

		case r.URL.Path == "/rest/unauthorized":
			writeRaw(w, http.StatusUnauthorized, `{"error":401,"message":"Unauthorized"}`)

		case r.URL.Path == "/rest/ip/firewall/filter/print":
			if _, ok := f.lastBody["count-only"]; ok {
				// ret is a string, not a number.
				writeRaw(w, http.StatusOK, `{"ret":"2"}`)
				return
			}
			writeJSON(f.t, w, http.StatusOK, []map[string]string{
				{".id": "*1", "comment": "accept established & more = fun", "chain": "input"},
			})

		case r.URL.Path == "/rest/ip/address" && r.Method == http.MethodPut:
			writeJSON(f.t, w, http.StatusCreated, map[string]string{".id": "*7", addressField: "10.0.0.1/24"})

		case r.URL.Path == "/rest/ip/address" && r.Method == http.MethodPost:
			// POST to a menu path is the console-command verb: it creates
			// nothing and says so quietly.
			writeJSON(f.t, w, http.StatusOK, []map[string]string{})

		case r.URL.Path == "/rest/certificate/settings/set":
			w.WriteHeader(http.StatusNoContent)

		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)

		default:
			writeJSON(f.t, w, http.StatusOK, []map[string]string{})
		}
	}))
}

func writeJSON(t *testing.T, w http.ResponseWriter, status int, v any) {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("encoding fake response: %v", err)
	}
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}

func writeRaw(w http.ResponseWriter, status int, body string) {
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func newTestClient(t *testing.T) (*Client, *fakeRouter) {
	t.Helper()
	f := &fakeRouter{t: t}
	srv := f.serve(t)
	c, err := New(srv.URL, WithHTTPClient(srv.Client()), WithBasicAuth("admin", ""))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c, f
}

func TestSingletonReturnsBareObject(t *testing.T) {
	c, _ := newTestClient(t)
	rec, err := c.Get(t.Context(), "/system/resource")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rec.String("cpu-load") != "0" {
		t.Errorf("cpu-load = %q", rec.String("cpu-load"))
	}
	if got := rec.Duration("uptime"); got.Seconds() != 80 {
		t.Errorf("uptime = %v, want 1m20s", got)
	}
}

func TestErrorBodyIsDetectedByShape(t *testing.T) {
	c, _ := newTestClient(t)
	_, err := c.Get(t.Context(), "/system/routerboard")
	if err == nil {
		t.Fatal("want an error; a body that decodes is not automatically data")
	}
	re, ok := errors.AsType[*Error](err)
	if !ok {
		t.Fatalf("want *rest.Error, got %T: %v", err, err)
	}
	if re.Code != 400 || re.Detail != "no such command or directory (routerboard)" {
		t.Errorf("Error = %+v", re)
	}
}

func TestUnauthorizedDecodesRatherThanFailing(t *testing.T) {
	// The bug this pins: unmarshalling into map[string]string fails on the
	// numeric "error" before any check can run, so a 401 surfaces as a decode
	// failure instead of an auth failure.
	c, _ := newTestClient(t)
	_, err := c.List(t.Context(), "/unauthorized")
	re, ok := errors.AsType[*Error](err)
	if !ok || re.Code != 401 {
		t.Fatalf("want a 401 *rest.Error, got %T: %v", err, err)
	}
}

func TestCreateUsesPUT(t *testing.T) {
	c, f := newTestClient(t)
	rec, err := c.Create(t.Context(), "/ip/address", Record{addressField: "10.0.0.1/24"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if f.lastVerb != http.MethodPut {
		t.Errorf("verb = %s, want PUT — POST is the console-command verb", f.lastVerb)
	}
	if rec.ID() != "*7" {
		t.Errorf("id = %q", rec.ID())
	}
}

func TestFilteredListUsesPostBodyNotQueryString(t *testing.T) {
	c, f := newTestClient(t)
	const comment = "accept established & more = fun"
	recs, err := c.List(t.Context(), "/ip/firewall/filter", Where("comment", comment))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if f.lastVerb != http.MethodPost || f.lastPath != "/rest/ip/firewall/filter/print" {
		t.Fatalf("filtered list went to %s %s", f.lastVerb, f.lastPath)
	}
	// The value must reach the router untouched — no escaping involved.
	got, _ := f.lastBody[".query"].([]any)
	if len(got) != 1 || got[0].(string) != "comment="+comment {
		t.Errorf(".query = %v, want the value verbatim", got)
	}
	if len(recs) != 1 || recs[0].String("comment") != comment {
		t.Errorf("rows = %v", recs)
	}
}

func TestUnfilteredListUsesGET(t *testing.T) {
	c, f := newTestClient(t)
	if _, err := c.List(t.Context(), "/ip/firewall/filter"); err != nil {
		t.Fatalf("List: %v", err)
	}
	if f.lastVerb != http.MethodGet {
		t.Errorf("verb = %s, want GET when there is nothing to filter", f.lastVerb)
	}
}

func TestCountParsesRetAsString(t *testing.T) {
	c, _ := newTestClient(t)
	n, err := c.Count(t.Context(), "/ip/firewall/filter")
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 2 {
		t.Errorf("Count = %d, want 2", n)
	}
}

func TestSetUsesPostSetForSingletons(t *testing.T) {
	c, f := newTestClient(t)
	if err := c.Set(t.Context(), "/certificate/settings", Record{"crl-use": "true"}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if f.lastVerb != http.MethodPost || f.lastPath != "/rest/certificate/settings/set" {
		t.Errorf("Set went to %s %s; singletons reject PATCH", f.lastVerb, f.lastPath)
	}
}

func TestDeleteTolerates204(t *testing.T) {
	c, _ := newTestClient(t)
	if err := c.Delete(t.Context(), "/ip/address", "*7"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

// TestAddressRejectionIsNotMenuAbsent pins the case with no status code at
// all: RouterOS completes the handshake and closes when the source address is
// not permitted, which must not read as "menu absent" or "router down".
func TestAddressRejectionIsNotMenuAbsent(t *testing.T) {
	// The listener reads the request and then hangs up, which is what
	// RouterOS was observed to do: a clean FIN, reaching the caller as
	// io.EOF, with no status code anywhere.
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			buf := make([]byte, 4096)
			_ = conn.SetReadDeadline(time.Now().Add(time.Second))
			_, _ = conn.Read(buf)
			_ = conn.Close()
		}
	}()

	c, err := New("http://"+ln.Addr().String(), WithBasicAuth("admin", ""))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err = c.List(t.Context(), "/interface"); !errors.Is(err, ErrAddressRejected) {
		t.Fatalf("err = %v, want ErrAddressRejected", err)
	}
}

// TestTransportErrorClassification checks the policy directly rather than
// through a socket.
//
// Whether a hung-up connection surfaces as EOF or as ECONNRESET depends on
// whether the peer had drained the request first, which is not something a
// test can pin down: driving it through a listener produced one and then the
// other on consecutive runs. The classification itself is deterministic, so
// that is what gets asserted.
func TestTransportErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      error
		rejected bool
	}{
		{"graceful close", io.EOF, true},
		{"truncated response", io.ErrUnexpectedEOF, true},
		// A reset is an ordinary network fault — a reboot, a restarted
		// service, a middlebox. Calling it an address rejection would send
		// the reader to check the service's address list while the real
		// fault lay elsewhere.
		{"reset", syscall.ECONNRESET, false},
		{"refused", syscall.ECONNREFUSED, false},
		{"cancelled", context.Canceled, false},
		{"timeout", context.DeadlineExceeded, false},
	} {
		got := errors.Is(transportError("GET /interface", tc.err), ErrAddressRejected)
		if got != tc.rejected {
			t.Errorf("%s: rejected = %v, want %v", tc.name, got, tc.rejected)
		}
	}
}

// TestJSONStrictness pins the two encoding/json/v2 choices, since both are
// departures: one from v2's default, one from v1's behaviour.
func TestJSONStrictness(t *testing.T) {
	t.Run("duplicate names are rejected", func(t *testing.T) {
		// v1 silently keeps the last. A reply that names a field twice is
		// ambiguous, and ambiguity must surface.
		_, err := decode("GET /test", []byte(`{"comment":"first","comment":"second"}`))
		if err == nil {
			t.Fatal("want an error for a duplicated field")
		}
	})

	t.Run("invalid UTF-8 is rejected, never silently substituted", func(t *testing.T) {
		// Neither encoder can return the original bytes: v1 substitutes
		// U+FFFD, and so does v2 under jsontext.AllowInvalidUTF8. Since a
		// comment is durable identity, a mangled one stops matching and the
		// caller mints a duplicate row — so an error is the safer answer.
		recs, err := decode("GET /test", []byte("[{\"comment\":\"caf\xe9\"}]"))
		if err == nil {
			t.Fatalf("want an error, got %v — a substituted identity fails silently", recs)
		}
		if len(recs) != 0 {
			t.Errorf("rows = %v, want none alongside the error", recs)
		}
		// And the caller can tell it apart from a router-reported failure.
		if _, ok := errors.AsType[*Error](err); ok {
			t.Error("a malformed reply is not an *Error the router described")
		}
	})
}

func TestNewRejectsEmptyEndpoint(t *testing.T) {
	if _, err := New(""); err == nil {
		t.Error("New(\"\") should fail")
	}
}

func TestNewToleratesTrailingRest(t *testing.T) {
	for _, in := range []string{"https://10.0.10.1", "https://10.0.10.1/", "https://10.0.10.1/rest"} {
		c, err := New(in)
		if err != nil {
			t.Fatalf("New(%q): %v", in, err)
		}
		if c.base != "https://10.0.10.1/rest" {
			t.Errorf("New(%q).base = %q", in, c.base)
		}
	}
}
