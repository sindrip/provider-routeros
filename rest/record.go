package rest

import (
	"time"

	"github.com/sindrip/provider-routeros/rest/scalar"
)

// IDField is the router's row identifier. Values look like "*1A" and are
// reassigned when a row is deleted and recreated, so they address a row for
// the duration of one exchange and are not an identity to store.
const IDField = ".id"

// Record is one row or one settings singleton. RouterOS returns every value as
// a string; the accessors below parse them, and they exist on Record rather
// than in package scalar because a boolean's meaning depends on whether the
// key was present at all.
type Record map[string]string

// ID returns the row's .id, or "" for a singleton.
func (r Record) ID() string { return r[IDField] }

// Has reports whether the router sent the key.
func (r Record) Has(key string) bool { _, ok := r[key]; return ok }

// String returns the raw value, or "" when absent.
func (r Record) String(key string) string { return r[key] }

// Bool covers the three encodings that coexist in a single record: "true" or
// "false"; present but empty, which is a set flag; and absent, which is unset.
// A BGP session carries all three at once — established is "true", ebgp is "",
// ibgp is missing — so reading "" as false gets eBGP exactly backwards.
func (r Record) Bool(key string) bool {
	v, ok := r[key]
	if !ok {
		return false
	}
	return scalar.Bool(v)
}

// Duration parses a RouterOS interval such as "3d12h12m44s" or "52m34s530ms".
// An absent or unparseable value is 0; use scalar.Duration to see the error.
func (r Record) Duration(key string) time.Duration {
	d, err := scalar.Duration(r[key])
	if err != nil {
		return 0
	}
	return d
}

// Int parses a signed integer value; an absent or unparseable value is 0.
func (r Record) Int(key string) int64 {
	n, err := scalar.Int(r[key])
	if err != nil {
		return 0
	}
	return n
}

// Uint parses an unsigned integer value, which is what RouterOS's counters
// are — the traffic generator states their range as 0..18446744073709551615,
// so a signed reader loses the top half. An absent or unparseable value is 0.
func (r Record) Uint(key string) uint64 {
	n, err := scalar.Uint(r[key])
	if err != nil {
		return 0
	}
	return n
}

// Float parses a decimal value such as a sensor's "49.2"; an absent or
// unparseable value is 0.
func (r Record) Float(key string) float64 {
	f, err := scalar.Float(r[key])
	if err != nil {
		return 0
	}
	return f
}
