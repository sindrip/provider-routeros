package scalar

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

// corpus is the pinned answer the router gave hack/typedump for every console
// property. Driving the parsers from it is the point: it means Duration is
// checked against every interval bound RouterOS states, not against the
// handful of fields a caller happened to read.
const corpus = "../../config/arg-types.json"

type argType struct {
	Path   string `json:"path"`
	Arg    string `json:"arg"`
	Syntax []struct {
		SymbolType string `json:"symbol_type"`
		Text       string `json:"text"`
	} `json:"syntax"`
}

// boundRe pulls the endpoints out of "1s..7101w3d6h28m15s    (time interval)".
// The bound must be read from the syntax row that names the type, not from the
// arg's collected ranges: an argument like interface/6to4 keepalive is
// Interval[,Retries] and carries an integer range alongside the interval one.
var boundRe = regexp.MustCompile(`(\S+)\.\.(\S*)`)

// hexBound matches a bound written bare in hexadecimal, as the hex-valued
// fields state theirs: "0..FFFF" rather than "0..0xFFFF".
var hexBound = regexp.MustCompile(`^[0-9]*[A-F][0-9A-F]*$`)

func loadCorpus(t *testing.T) []argType {
	t.Helper()
	raw, err := os.ReadFile(corpus)
	if err != nil {
		// The module is importable on its own; the corpus only exists in
		// this repo.
		t.Skipf("corpus unavailable: %v", err)
	}
	var dump struct {
		Version string    `json:"routeros_version"`
		Args    []argType `json:"args"`
	}
	if err := json.Unmarshal(raw, &dump); err != nil {
		t.Fatalf("parsing %s: %v", corpus, err)
	}
	if len(dump.Args) == 0 {
		t.Fatalf("%s has no args", corpus)
	}
	t.Logf("corpus: %d args, RouterOS %s", len(dump.Args), dump.Version)
	return dump.Args
}

// TestDurationParsesEveryStatedBound asserts that Duration handles every time
// interval the router itself described. A parser that cannot read a bound
// RouterOS stated is wrong, and this catches it without a device.
func TestDurationParsesEveryStatedBound(t *testing.T) {
	var checked int
	for _, a := range loadCorpus(t) {
		for _, row := range a.Syntax {
			if !strings.Contains(row.Text, "(time interval)") {
				continue
			}
			m := boundRe.FindStringSubmatch(row.Text)
			if m == nil {
				continue
			}
			for _, bound := range m[1:] {
				if bound == "" {
					continue // an open-ended upper bound
				}
				if _, err := Duration(bound); err != nil {
					t.Errorf("%s %s: bound %q from %q: %v", a.Path, a.Arg, bound, row.Text, err)
				}
				checked++
			}
		}
	}
	if checked == 0 {
		t.Fatal("no time-interval bounds found; the corpus or its shape changed")
	}
	t.Logf("parsed %d interval bounds", checked)
}

// TestIntegerBoundsParse does the same for the integer ranges.
//
// Int and Uint are checked as a pair, because RouterOS uses the full width of
// both: bounds run from -9223372036854775808 on signed fields to
// 18446744073709551615 on the traffic generator's counters, and no single Go
// type spans that. Every bound must be readable by one of them.
func TestIntegerBoundsParse(t *testing.T) {
	var checked, needsUint, needsHex, needsID int
	for _, a := range loadCorpus(t) {
		for _, row := range a.Syntax {
			if !strings.Contains(row.Text, "(integer number)") {
				continue
			}
			m := boundRe.FindStringSubmatch(row.Text)
			if m == nil {
				continue
			}
			for _, bound := range m[1:] {
				if bound == "" {
					continue
				}
				checked++
				// Some fields typed "integer number" actually hold a
				// reference to another row — a route's nexthop-id, a script
				// job's parent — and state their range as *0..*FFFFFFFF.
				// Those are ids, and never read as numbers.
				if strings.HasPrefix(bound, "*") {
					if _, err := ID(bound); err != nil {
						t.Errorf("%s %s: id bound %q from %q: %v", a.Path, a.Arg, bound, row.Text, err)
					}
					needsID++
					continue
				}
				// Hex-valued fields state their bound bare ("0..FFFF" on a
				// bridge's priority) even though the value reads back
				// prefixed ("0x8000"). Normalise before parsing.
				lit := bound
				if hexBound.MatchString(bound) {
					lit = "0x" + bound
					needsHex++
				}
				if _, err := Int(lit); err == nil {
					continue
				}
				if _, err := Uint(lit); err != nil {
					t.Errorf("%s %s: bound %q from %q reads as neither signed nor unsigned: %v",
						a.Path, a.Arg, bound, row.Text, err)
					continue
				}
				needsUint++
			}
		}
	}
	if checked == 0 {
		t.Fatal("no integer bounds found; the corpus or its shape changed")
	}
	if needsUint == 0 {
		t.Error("no bound required Uint; expected the traffic generator's uint64 counters")
	}
	t.Logf("parsed %d integer bounds; %d needed unsigned, %d bare hex, %d row ids", checked, needsUint, needsHex, needsID)
}

// TestNoConsoleFurnitureInCorpus guards the typedump fix. print's own
// arguments are not properties, and recording them invented five fields on
// every rowless menu. They are named here rather than derived because the
// point is to fail loudly if they return.
func TestNoConsoleFurnitureInCorpus(t *testing.T) {
	furniture := map[string]bool{
		"as-value": true, "comments": true, "without-paging": true,
		"show-sensitive": true, "oid": true, "value-name": true,
		"as-string": true, "as-string-value": true,
	}
	for _, a := range loadCorpus(t) {
		if furniture[a.Arg] {
			t.Errorf("%s: %q is a console command argument, not a property", a.Path, a.Arg)
		}
	}
}
