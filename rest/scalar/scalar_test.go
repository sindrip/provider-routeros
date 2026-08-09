package scalar

import (
	"testing"
	"time"
)

func TestDuration(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"1s", time.Second},
		{"1m20s", time.Minute + 20*time.Second},
		{"2d20h12m20s", 2*24*time.Hour + 20*time.Hour + 12*time.Minute + 20*time.Second},
		{"3d12h12m44s", 3*24*time.Hour + 12*time.Hour + 12*time.Minute + 44*time.Second},
		// The trap: a regexp that splits on [wdhms] reads this as 530 minutes.
		{"52m34s530ms", 52*time.Minute + 34*time.Second + 530*time.Millisecond},
		{"530ms", 530 * time.Millisecond},
		{"100ms", 100 * time.Millisecond},
		// Bounds taken verbatim from config/arg-types.json.
		{"71w2h27m52s950ms", 71*7*24*time.Hour + 2*time.Hour + 27*time.Minute + 52*time.Second + 950*time.Millisecond},
		{"7101w3d6h28m15s", 7101*7*24*time.Hour + 3*24*time.Hour + 6*time.Hour + 28*time.Minute + 15*time.Second},
		{"18h12m15s", 18*time.Hour + 12*time.Minute + 15*time.Second},
		{"0s", 0},
		{"4w", 4 * 7 * 24 * time.Hour},
	} {
		got, err := Duration(tc.in)
		if err != nil {
			t.Errorf("Duration(%q): unexpected error %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Duration(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestDurationRejects(t *testing.T) {
	// The colon form is a console display bound on print's own interval
	// argument, never a value RouterOS returns. Parsing it would mean the
	// caller had mistaken one for the other.
	for _, in := range []string{"00:00:00.200", "1x", "abc", "12", "1s2", "-1s", "1.5s"} {
		if _, err := Duration(in); err == nil {
			t.Errorf("Duration(%q) = nil error, want a failure", in)
		}
	}
}

func TestBool(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"true", true},
		{"false", false},
		{"yes", true},
		{"no", false},
		// Present but empty is a set flag — a BGP session's ebgp. Reading
		// this as false gets eBGP exactly backwards.
		{"", true},
		{"anything-else", false},
	} {
		if got := Bool(tc.in); got != tc.want {
			t.Errorf("Bool(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestHexValues(t *testing.T) {
	// A bridge reads its priority back as "0x8000", and accepts "0x4000".
	if n, err := Uint("0x8000"); err != nil || n != 32768 {
		t.Errorf("Uint(0x8000) = %v, %v; want 32768", n, err)
	}
	if n, err := Int("0xFF"); err != nil || n != 255 {
		t.Errorf("Int(0xFF) = %v, %v; want 255", n, err)
	}
	// A leading zero must stay decimal. strconv's base-0 mode would read
	// this as octal 8 and never complain.
	if n, err := Int("010"); err != nil || n != 10 {
		t.Errorf("Int(010) = %v, %v; want 10 — a zero-padded decimal is not octal", n, err)
	}
}

func TestID(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want uint32
	}{
		{"*0", 0},
		{"*1", 1},
		{"*1A", 26},
		{"*FFFFFFFF", 4294967295},
	} {
		got, err := ID(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("ID(%q) = %v, %v; want %v", tc.in, got, err, tc.want)
		}
		if !IsID(tc.in) {
			t.Errorf("IsID(%q) = false", tc.in)
		}
	}
	for _, in := range []string{"", "*", "1A", "*GG", "*100000000", "accept established"} {
		if _, err := ID(in); err == nil {
			t.Errorf("ID(%q) = nil error, want a failure", in)
		}
		if IsID(in) {
			t.Errorf("IsID(%q) = true", in)
		}
	}
}

func TestIntAndFloat(t *testing.T) {
	if n, err := Int("5577"); err != nil || n != 5577 {
		t.Errorf("Int(5577) = %v, %v", n, err)
	}
	if n, err := Int(""); err != nil || n != 0 {
		t.Errorf("Int(empty) = %v, %v", n, err)
	}
	if _, err := Int("49.2"); err == nil {
		t.Error("Int(49.2) should fail; a voltage is not an integer")
	}
	// jack-voltage is "49.2": values are not always integral.
	if f, err := Float("49.2"); err != nil || f != 49.2 {
		t.Errorf("Float(49.2) = %v, %v", f, err)
	}
}
