package rest

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestRecordBoolCoversAllThreeEncodings is the one worth having. scalar.Bool
// cannot see absence, so the distinction between "present but empty" and
// "missing" only exists at this level — and getting it wrong reads a BGP
// session's eBGP flag as its opposite.
func TestRecordBoolCoversAllThreeEncodings(t *testing.T) {
	// A real /routing/bgp/session carries all three at once.
	session := Record{
		"established": "true",
		"ebgp":        "", // present and empty: the flag is set
		// ibgp is absent: the flag is not set
		"disabled": "false",
	}
	for _, tc := range []struct {
		key  string
		want bool
	}{
		{"established", true},
		{"ebgp", true},
		{"ibgp", false},
		{"disabled", false},
	} {
		if got := session.Bool(tc.key); got != tc.want {
			t.Errorf("Bool(%q) = %v, want %v", tc.key, got, tc.want)
		}
	}

	if !session.Has("ebgp") {
		t.Error(`Has("ebgp") = false, but the key is present with an empty value`)
	}
	if session.Has("ibgp") {
		t.Error(`Has("ibgp") = true, but the key is absent`)
	}
}

func TestRecordAccessors(t *testing.T) {
	r := Record{
		".id":       "*1A",
		"rx-byte":   "5577",
		"uptime":    "1m20s",
		"value":     "49.2",
		"priority":  "0x8000",
		"rssi":      "-64",
		"malformed": "not-a-number",
	}
	if r.ID() != "*1A" {
		t.Errorf("ID() = %q", r.ID())
	}
	if got := r.Int("rssi"); got != -64 {
		t.Errorf("Int(rssi) = %d, want -64", got)
	}
	if got := r.Uint("rx-byte"); got != 5577 {
		t.Errorf("Uint(rx-byte) = %d", got)
	}
	if got := r.Uint("priority"); got != 0x8000 {
		t.Errorf("Uint(priority) = %d, want 32768", got)
	}
	if got := r.Float("value"); got != 49.2 {
		t.Errorf("Float(value) = %v", got)
	}
	if got := r.Duration("uptime"); got != 80*time.Second {
		t.Errorf("Duration(uptime) = %v", got)
	}
	if got := r.String("rx-byte"); got != "5577" {
		t.Errorf("String(rx-byte) = %q", got)
	}

	// Absent and unparseable both read as zero: the accessors are for callers
	// that want a value, not a diagnosis. scalar exposes the error.
	if r.Int("absent") != 0 || r.Uint("absent") != 0 || r.Float("absent") != 0 || r.Duration("absent") != 0 {
		t.Error("absent keys should read as zero")
	}
	if r.Int("malformed") != 0 || r.Float("malformed") != 0 || r.Duration("malformed") != 0 {
		t.Error("unparseable values should read as zero")
	}
	if r.String("absent") != "" {
		t.Error("String on an absent key should be empty")
	}
}

func TestErrorMessage(t *testing.T) {
	// Detail is preferred, then Message, then the status text — the router
	// sends the useful half in Detail.
	for _, tc := range []struct {
		err  *Error
		want string
	}{
		{&Error{Op: "GET /x", Status: 400, Code: 400, Detail: "no such command", Message: "Bad Request"}, "no such command"},
		{&Error{Op: "GET /x", Status: 400, Code: 400, Message: "Bad Request"}, "Bad Request"},
		{&Error{Op: "GET /x", Status: http.StatusNotFound}, "Not Found"},
	} {
		if got := tc.err.Error(); !contains(got, tc.want) {
			t.Errorf("Error() = %q, want it to mention %q", got, tc.want)
		}
		if !contains(tc.err.Error(), "GET /x") {
			t.Errorf("Error() = %q, want it to name the operation", tc.err.Error())
		}
	}
}

func TestErrorIsNotFound(t *testing.T) {
	// Both the HTTP status and the router's own code map onto ErrNotFound, so
	// a caller can ask one question instead of two.
	for _, e := range []*Error{
		{Op: "GET /x", Status: http.StatusNotFound},
		{Op: "GET /x", Code: http.StatusNotFound},
	} {
		if !isNotFound(e) {
			t.Errorf("errors.Is(%v, ErrNotFound) = false", e)
		}
	}
	if isNotFound(&Error{Op: "GET /x", Status: 400, Code: 400}) {
		t.Error("a 400 must not read as ErrNotFound")
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

func isNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

func TestWhereRaw(t *testing.T) {
	// WhereRaw exists for the operators Where does not model; it must reach
	// the body untouched.
	q := newQuery([]QueryOpt{WhereRaw("bytes>1000")})
	got, _ := q.body()[".query"].([]string)
	if len(got) != 1 || got[0] != "bytes>1000" {
		t.Errorf(".query = %v", got)
	}
}

// TestFloatOKSeparatesZeroFromAbsent is the distinction /system/health needs:
// the router answers an unplugged sensor with "no-input", not with a number,
// and publishing that as 0 °C invents a reading.
func TestFloatOKSeparatesZeroFromAbsent(t *testing.T) {
	r := Record{"cold": "0", "warm": "49.2", "unplugged": "no-input", "blank": ""}
	for _, tc := range []struct {
		key  string
		want float64
		ok   bool
	}{
		{"cold", 0, true},
		{"warm", 49.2, true},
		{"unplugged", 0, false},
		{"blank", 0, false},
		{"missing", 0, false},
	} {
		got, ok := r.FloatOK(tc.key)
		if got != tc.want || ok != tc.ok {
			t.Errorf("FloatOK(%q) = %v, %v; want %v, %v", tc.key, got, ok, tc.want, tc.ok)
		}
	}
	// Float keeps its lenient shape for callers that have no way to report it.
	if r.Float("unplugged") != 0 {
		t.Error("Float should still fall back to zero")
	}
}
