package rest

import (
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// rfc3986Query is the alphabet a correctly escaped query value may use: the
// unreserved set, plus percent-encoded triples. Nothing else.
var rfc3986Query = regexp.MustCompile(`^(?:[A-Za-z0-9\-._~]|%[0-9A-F]{2})*$`)

func TestEscapeQuery(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", ""},
		{"simple", "simple"},
		// url.QueryEscape alone gives "accept+established", which the router
		// matches literally and so finds nothing.
		{"accept established", "accept%20established"},
		{"a&b", "a%26b"},
		{"a=b", "a%3Db"},
		// A literal plus is already %2B by the time the substitution runs, so
		// it survives.
		{"a+b", "a%2Bb"},
	} {
		if got := escapeQuery(tc.in); got != tc.want {
			t.Errorf("escapeQuery(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// FuzzEscapeQuery pins the property rather than a list of characters: whatever
// the input, the output is RFC 3986, carries no '+', and round-trips.
func FuzzEscapeQuery(f *testing.F) {
	f.Add("accept established")
	f.Add("a&b=c#d?e/f%g")
	f.Add("höme \t\n 🛰")
	f.Add("*1A")
	f.Add("")
	f.Fuzz(func(t *testing.T, s string) {
		got := escapeQuery(s)
		if !rfc3986Query.MatchString(got) {
			t.Fatalf("escapeQuery(%q) = %q is not RFC 3986", s, got)
		}
		if strings.ContainsRune(got, '+') {
			t.Fatalf("escapeQuery(%q) = %q still carries '+'", s, got)
		}
		back, err := url.QueryUnescape(got)
		if err != nil || back != s {
			t.Fatalf("round-trip %q -> %q -> %q (%v)", s, got, back, err)
		}
	})
}

func TestPropsAlwaysRequestsID(t *testing.T) {
	// POST .../print omits .id unless it is named, so a caller that asked for
	// one field would get a row it cannot then address.
	q := newQuery([]QueryOpt{Props("name")})
	got, _ := q.body()[".proplist"].([]string)
	if len(got) != 2 || got[0] != IDField || got[1] != "name" {
		t.Fatalf(".proplist = %v, want [%s name]", got, IDField)
	}
	// Asking for it explicitly must not duplicate it.
	q = newQuery([]QueryOpt{Props(IDField, "name")})
	got, _ = q.body()[".proplist"].([]string)
	if len(got) != 2 {
		t.Fatalf(".proplist = %v, want no duplicate %s", got, IDField)
	}
}

func TestMenuPath(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"ip/address", "/ip/address"},
		{"/ip/address", "/ip/address"},
		{"/ip/address/", "/ip/address"},
	} {
		if got := menuPath(tc.in); got != tc.want {
			t.Errorf("menuPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
