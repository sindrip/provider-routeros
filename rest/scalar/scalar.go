// Package scalar parses the string forms RouterOS returns for every value.
//
// The catalogue these cover is not guesswork: hack/typedump asks the router
// for the datatype of every console property, and config/arg-types.json pins
// the answer. Time intervals are the load-bearing case — over 400 properties
// across 43 distinct bound forms, more than any type but bare strings.
package scalar

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// durTerm matches one component of a RouterOS interval. "ms" leads the
// alternation because Go's regexp prefers the leftmost alternative: were it
// last, "530ms" would match "m" and be read as 530 minutes.
var durTerm = regexp.MustCompile(`^(\d+)(ms|w|d|h|m|s)`)

var durUnit = map[string]time.Duration{
	"ms": time.Millisecond,
	"s":  time.Second,
	"m":  time.Minute,
	"h":  time.Hour,
	"d":  24 * time.Hour,
	"w":  7 * 24 * time.Hour,
}

// Duration parses a RouterOS time interval: any subset of weeks, days, hours,
// minutes, seconds and milliseconds, largest first, with absent components
// simply omitted — "1m20s", "3d12h12m44s", "52m34s530ms", "71w2h27m52s950ms".
//
// The empty string is 0, because RouterOS returns it for an unset interval.
//
// Go's time.ParseDuration is not usable here: it rejects w and d, and folding
// those to hours before calling it silently mis-parses a bare "530ms" in the
// process. Note also that RouterOS displays a *bound* like "00:00:00.200" in
// colon form, but never returns a value that way — that form belongs to
// print's own interval argument, a console display feature, not to any field.
func Duration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	rest, total := s, time.Duration(0)
	for rest != "" {
		m := durTerm.FindStringSubmatch(rest)
		if m == nil {
			return 0, fmt.Errorf("scalar: %q is not a RouterOS interval (at %q)", s, rest)
		}
		n, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("scalar: %q: %w", s, err)
		}
		total += time.Duration(n) * durUnit[m[2]]
		rest = rest[len(m[0]):]
	}
	return total, nil
}

// Bool interprets a value the router sent. The caller must already know the
// key was present: absence means false, and only Record can see that.
//
// A present but empty value is a set flag — RouterOS writes a BGP session's
// ebgp that way — so it is true, not false.
func Bool(v string) bool {
	if v == "" {
		return true
	}
	return strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

// split separates a RouterOS integer literal into digits and base.
//
// Some fields are hexadecimal and read back with an 0x prefix — a bridge's
// priority is "0x8000" — so a base-10 reader gets 0. strconv's base-0 mode
// would handle that, but it would also read a leading zero as octal, turning a
// zero-padded decimal into a different number without complaint. The prefix is
// therefore matched explicitly.
func split(s string) (digits string, base int) {
	neg := strings.HasPrefix(s, "-")
	body := strings.TrimPrefix(s, "-")
	if len(body) > 2 && body[0] == '0' && (body[1] == 'x' || body[1] == 'X') {
		body, base = body[2:], 16
	} else {
		base = 10
	}
	if neg {
		body = "-" + body
	}
	return body, base
}

// Int parses a signed integer value. The empty string is 0.
//
// Signed is not always enough: the bounds RouterOS states span both extremes,
// from -9223372036854775808 on fields like a radio's RSSI up to
// 18446744073709551615 on the traffic generator's byte and packet counters. No
// one Go type holds both, so counters need Uint.
//
// Note that an integer-shaped field is not always a number at all — a bridge
// reports mtu as "auto". Whether a given field can do that is a property of
// the field, recorded in config/arg-types.json, not something to infer here.
func Int(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	digits, base := split(s)
	n, err := strconv.ParseInt(digits, base, 64)
	if err != nil {
		return 0, fmt.Errorf("scalar: %q is not a signed integer: %w", s, err)
	}
	return n, nil
}

// Uint parses an unsigned integer value, which is what RouterOS's counters
// are. The empty string is 0.
func Uint(s string) (uint64, error) {
	if s == "" {
		return 0, nil
	}
	digits, base := split(s)
	n, err := strconv.ParseUint(digits, base, 64)
	if err != nil {
		return 0, fmt.Errorf("scalar: %q is not an unsigned integer: %w", s, err)
	}
	return n, nil
}

// IsID reports whether s looks like a RouterOS row identifier, "*1A".
func IsID(s string) bool {
	if !strings.HasPrefix(s, "*") || len(s) < 2 {
		return false
	}
	_, err := strconv.ParseUint(s[1:], 16, 32)
	return err == nil
}

// ID parses a RouterOS row identifier such as "*1A".
//
// These are not opaque strings: the router states the range of an id-valued
// field as *0..*FFFFFFFF, so they are 32-bit hexadecimal behind the asterisk.
// Some fields hold a reference to another row this way — a route's nexthop-id,
// a script job's parent — and are typed "integer number" despite never reading
// as one.
//
// The numeric value is useful for comparison, not for storage: RouterOS
// reassigns an id when a row is deleted and recreated, which is why durable
// identity has to come from a name or comment instead.
func ID(s string) (uint32, error) {
	if !strings.HasPrefix(s, "*") {
		return 0, fmt.Errorf("scalar: %q is not a RouterOS id (no leading *)", s)
	}
	n, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return 0, fmt.Errorf("scalar: %q is not a RouterOS id: %w", s, err)
	}
	return uint32(n), nil
}

// Float parses a decimal value, such as a health sensor's "49.2". Values are
// not always integral, which a counter-shaped reader tends to assume.
func Float(s string) (float64, error) {
	if s == "" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("scalar: %q is not a number: %w", s, err)
	}
	return f, nil
}
