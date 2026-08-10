// Package routeros holds the hand-written half of the generated view: the
// conversions the emitted code calls.
//
// These duplicate rest/scalar deliberately rather than importing it. rest is
// pinned to a Go release candidate for encoding/json/v2, and nothing about a
// typed accessor should drag a prerelease toolchain into a caller's build.
// When rest settles onto a released Go, this file should collapse into it.
package routeros

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// parseBool covers the three encodings that coexist in one record: "true" or
// "false"; present but empty, which is a set flag; and absent, which is unset.
//
// The console states booleans as no/yes, which is how the generator told a
// bool apart from an enum — but a REST read returns true/false, so both
// spellings arrive here.
func parseBool(r map[string]string, key string) bool {
	v, ok := r[key]
	if !ok {
		return false
	}
	if v == "" {
		return true
	}
	return strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

func formatBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

var durTerm = regexp.MustCompile(`^(\d+)(ms|w|d|h|m|s)`)

var durUnit = map[string]time.Duration{
	"ms": time.Millisecond,
	"s":  time.Second,
	"m":  time.Minute,
	"h":  time.Hour,
	"d":  24 * time.Hour,
	"w":  7 * 24 * time.Hour,
}

// parseDuration reads a RouterOS interval such as "3d12h12m44s" or
// "52m34s530ms". An unreadable value is zero; the typed layer has nowhere to
// report an error, which is a cost of the shape, not an oversight.
//
// ms leads the alternation because a regexp that does not put it first reads
// "530ms" as 530 minutes.
func parseDuration(s string) time.Duration {
	var total time.Duration
	for s != "" {
		m := durTerm.FindStringSubmatch(s)
		if m == nil {
			return 0
		}
		n, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			return 0
		}
		total += time.Duration(n) * durUnit[m[2]]
		s = s[len(m[0]):]
	}
	return total
}

func formatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	var b strings.Builder
	for _, u := range []struct {
		suffix string
		size   time.Duration
	}{{"w", 7 * 24 * time.Hour}, {"d", 24 * time.Hour}, {"h", time.Hour}, {"m", time.Minute}, {"s", time.Second}} {
		// n is a count, not a duration, so it is typed as one: multiplying
		// two durations together is a unit error even when it computes.
		if n := int64(d / u.size); n > 0 {
			b.WriteString(strconv.FormatInt(n, 10))
			b.WriteString(u.suffix)
			d -= time.Duration(n) * u.size
		}
	}
	if ms := int64(d / time.Millisecond); ms > 0 {
		b.WriteString(strconv.FormatInt(ms, 10))
		b.WriteString("ms")
	}
	return b.String()
}

// parseInt reads a signed integer, accepting the 0x form some fields use — a
// bridge reports its priority as "0x8000". Base 0 is avoided because it would
// also read a zero-padded decimal as octal.
func parseInt(s string) int64 {
	if s == "" {
		return 0
	}
	digits, base := s, 10
	neg := strings.HasPrefix(digits, "-")
	digits = strings.TrimPrefix(digits, "-")
	if len(digits) > 2 && digits[0] == '0' && (digits[1] == 'x' || digits[1] == 'X') {
		digits, base = digits[2:], 16
	}
	if neg {
		digits = "-" + digits
	}
	n, err := strconv.ParseInt(digits, base, 64)
	if err != nil {
		return 0
	}
	return n
}

func formatInt(n int64) string { return strconv.FormatInt(n, 10) }
