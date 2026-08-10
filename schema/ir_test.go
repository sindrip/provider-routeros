package schema

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

func load(t *testing.T) *IR {
	t.Helper()
	ir, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(ir.Menus) == 0 {
		t.Fatal("the pinned IR has no menus")
	}
	return ir
}

// TestCensus pins the shape of the device. These counts are a property of
// RouterOS 7.23.2, so a failure here means either the router moved or a
// derivation changed — both worth a human look rather than a silent update.
func TestCensus(t *testing.T) {
	ir := load(t)
	got := ir.Census()
	want := map[Class]int{
		ClassOrdered:   44,
		ClassList:      280,
		ClassSingleton: 80,
	}
	for class, n := range want {
		if got[class] != n {
			t.Errorf("%s: %d menus, want %d", class, got[class], n)
		}
	}
	if len(ir.Menus) != 404 {
		t.Errorf("%d menus, want 404", len(ir.Menus))
	}
}

// TestCardinalityAndMutabilityAreIndependent is the property the first version
// of Class could not express. A device table such as connection tracking holds
// thousands of rows that no caller may write, and an emitter has to be able to
// see both halves: one record versus many is not the same question as writable
// versus not.
func TestCardinalityAndMutabilityAreIndependent(t *testing.T) {
	ir := load(t)
	var rowsReadOnly, rowsWritable, singletonReadOnly int
	for _, m := range ir.Menus {
		switch {
		case m.Rows() && !m.Writable:
			rowsReadOnly++
		case m.Rows():
			rowsWritable++
		case !m.Writable:
			singletonReadOnly++
		}
	}
	if rowsReadOnly == 0 {
		t.Fatal("no read-only menu holds rows, so the two axes are collapsed again")
	}
	t.Logf("%d menus hold rows and are read-only, %d hold rows and are writable, %d are read-only singletons",
		rowsReadOnly, rowsWritable, singletonReadOnly)

	// The archetype: the device maintains it, nobody adds to it.
	conn, ok := ir.Menu("/ip/firewall/connection")
	if !ok {
		t.Fatal("/ip/firewall/connection missing")
	}
	if !conn.Rows() || conn.Writable {
		t.Errorf("/ip/firewall/connection: rows=%v writable=%v, want rows and not writable", conn.Rows(), conn.Writable)
	}
}

// TestNamespacesAreNotMenus guards the distinction between a menu and a
// grouping. /ip has sub-menus but no print of its own, so there is nothing
// there to read or generate.
func TestNamespacesAreNotMenus(t *testing.T) {
	ir := load(t)
	for _, path := range []string{"/ip", "/ipv6", "/system", "/tool"} {
		if m, ok := ir.Menu(path); ok {
			t.Errorf("%s is a namespace but appears as a %s menu", path, m.Class)
		}
	}
	// The two top-level nodes that genuinely are menus must survive.
	for _, path := range []string{"/interface", "/user"} {
		if _, ok := ir.Menu(path); !ok {
			t.Errorf("%s is a real menu and must be present", path)
		}
	}
}

// TestRowlessIsNotInferredFromAdd is a regression guard on the derivation that
// was wrong first time round. These menus all hold rows without exposing add,
// so anything keyed on add classifies them as settings singletons and every
// emitter downstream then generates one record where there are many.
func TestRowlessIsNotInferredFromAdd(t *testing.T) {
	ir := load(t)
	for _, path := range []string{
		"/interface",
		"/interface/ethernet",
		"/routing/route",
		"/ip/firewall/connection",
	} {
		m, ok := ir.Menu(path)
		if !ok {
			t.Errorf("%s missing", path)
			continue
		}
		if !m.Rows() {
			t.Errorf("%s: class %s — it holds rows despite having no add command", path, m.Class)
		}
		if slices.Contains(m.Commands, "add") {
			t.Errorf("%s now has an add command, so it no longer tests what it was written for", path)
		}
		if m.ClassEvidence != Probed {
			t.Errorf("%s: class evidence %s, want probed", path, m.ClassEvidence)
		}
	}
}

// TestIdentityIsNeverInvented is the gate that matters most for a generator: a
// key may only come from a verdict a router actually returned.
func TestIdentityIsNeverInvented(t *testing.T) {
	ir := load(t)
	var unique, unprobed int
	for _, m := range ir.Menus {
		id := m.Identity
		switch id.Verdict {
		case Unique:
			unique++
			if id.Key == "" {
				t.Errorf("%s: UNIQUE but no key", m.Path)
			}
			// uniqprobe tests the name field and nothing else, so a
			// verdict is evidence about name alone.
			if id.Key != "name" {
				t.Errorf("%s: key %q, but the verdict only speaks for name", m.Path, id.Key)
			}
		case Duplicate, Untested, Unprobed:
			if id.Verdict == Unprobed {
				unprobed++
			}
			if id.Key != "" {
				t.Errorf("%s: verdict %s must not yield the key %q", m.Path, id.Verdict, id.Key)
			}
		default:
			t.Errorf("%s: unknown verdict %q", m.Path, id.Verdict)
		}
		if id.Key != "" && !slices.Contains(id.Candidates, id.Key) {
			t.Errorf("%s: key %q is not among candidates %v", m.Path, id.Key, id.Candidates)
		}
	}
	if unique != 66 {
		t.Errorf("%d menus with a proven unique name, want 66", unique)
	}
	// The honest headline: most row-bearing menus have never been probed,
	// and that has to stay visible rather than defaulting to "unique".
	t.Logf("%d of %d menus never probed for uniqueness", unprobed, len(ir.Menus))
}

// TestBooleansComeFromTheVocabulary checks the derivation that lets a
// generator emit a bool instead of a string.
func TestBooleansComeFromTheVocabulary(t *testing.T) {
	ir := load(t)
	var bools, enums, observedBools int
	for _, m := range ir.Menus {
		for _, f := range m.Fields {
			switch {
			case f.Bool && f.Evidence == Probed:
				bools++
				// A declared boolean is a closed vocabulary of exactly no/yes.
				if f.Kind != KindEnum || len(f.Values) != 2 {
					t.Errorf("%s %s: marked bool but kind=%s values=%v", m.Path, f.Name, f.Kind, f.Values)
				}
			case f.Bool:
				// An observed boolean has no vocabulary: the router never
				// stated one, it just returned true or false. Fabricating
				// no/yes here would invent evidence.
				observedBools++
				if f.Evidence != Observed {
					t.Errorf("%s %s: bool with neither a vocabulary nor an observation", m.Path, f.Name)
				}
				if f.Sample != "true" && f.Sample != "false" {
					t.Errorf("%s %s: observed bool from sample %q", m.Path, f.Name, f.Sample)
				}
			case f.Kind == KindEnum:
				enums++
			}
		}
	}
	if bools == 0 {
		t.Fatal("no booleans derived; the vocabulary rule stopped working")
	}
	t.Logf("%d booleans from a vocabulary, %d from an observed value, %d other enums", bools, observedBools, enums)

	// The pair that shows why the vocabulary is the right signal: one
	// answers no/yes and is a bool, the other adds a third value and is not.
	nd, ok := ir.Menu("/ipv6/nd")
	if !ok {
		t.Skip("/ipv6/nd absent")
	}
	for _, tc := range []struct {
		field string
		want  bool
	}{
		{"advertise-mac-address", true},
		{"advertise-dns", false},
	} {
		i := slices.IndexFunc(nd.Fields, func(f Field) bool { return f.Name == tc.field })
		if i < 0 {
			t.Errorf("/ipv6/nd has no %s", tc.field)
			continue
		}
		if got := nd.Fields[i].Bool; got != tc.want {
			t.Errorf("/ipv6/nd %s: bool=%v, want %v (values %v)", tc.field, got, tc.want, nd.Fields[i].Values)
		}
	}
}

// TestUnknownsStaySayable checks that the gaps are representable rather than
// papered over. An emitter has to be able to see them to refuse.
func TestUnknownsStaySayable(t *testing.T) {
	ir := load(t)
	var untyped, unknownKind int
	for _, m := range ir.Menus {
		if !m.Typed {
			untyped++
		}
		for _, f := range m.Fields {
			if f.Kind == KindUnknown {
				unknownKind++
			}
		}
	}
	if untyped == 0 && unknownKind == 0 {
		t.Fatal("no gaps recorded at all, which would mean they were filled in rather than kept")
	}
	t.Logf("%d menus with no types, %d fields the router would not type", untyped, unknownKind)
}

// TestObservedTypesFillTheConsoleGap covers the third kind of evidence. The
// console cannot describe a read-only property of a menu that holds no rows,
// so those fields are typed from a value the router returned instead — weaker,
// and labelled as such rather than passed off as a declaration.
func TestObservedTypesFillTheConsoleGap(t *testing.T) {
	ir := load(t)
	var observed, probed int
	for _, m := range ir.Menus {
		for _, f := range m.Fields {
			switch f.Evidence {
			case Observed:
				observed++
				if f.Sample == "" {
					t.Errorf("%s %s: observed but carries no sample to disagree with", m.Path, f.Name)
				}
				if m.Rows() {
					t.Errorf("%s %s: observed typing is for rowless menus; this one has rows", m.Path, f.Name)
				}
				if f.Access != ReadOnly {
					t.Errorf("%s %s: observed typing is for read-only fields, got %s", m.Path, f.Name, f.Access)
				}
			case Probed:
				probed++
				if f.Kind == KindUnknown {
					t.Errorf("%s %s: probed but of unknown kind", m.Path, f.Name)
				}
			case Inferred:
				t.Errorf("%s %s: a field type is never inferred, only probed or observed", m.Path, f.Name)
			}
		}
	}
	if observed == 0 {
		t.Fatal("no observed types; the gap is open again")
	}
	t.Logf("%d fields typed by observation, %d by the console", observed, probed)

	// The menu the gap was found on, and the one telemetry cares about most.
	res, ok := ir.Menu("/system/resource")
	if !ok {
		t.Fatal("/system/resource missing")
	}
	want := map[string]string{
		"uptime":       "time interval",
		"cpu-load":     "integer number",
		"version":      "string value",
		"build-time":   "date time",
		"total-memory": "integer number",
	}
	for _, f := range res.Fields {
		if w, ok := want[f.Name]; ok {
			if f.Type != w {
				t.Errorf("/system/resource %s: type %q, want %q (sample %q)", f.Name, f.Type, w, f.Sample)
			}
			if f.Evidence != Observed {
				t.Errorf("/system/resource %s: evidence %q, want observed", f.Name, f.Evidence)
			}
			delete(want, f.Name)
		}
	}
	for name := range want {
		t.Errorf("/system/resource has no %s field", name)
	}
}

// TestPinnedIRIsDeterministic keeps the artifact diffable: menus sorted by
// path, fields by name, so a RouterOS upgrade shows as a real change.
func TestPinnedIRIsDeterministic(t *testing.T) {
	ir := load(t)
	if !slices.IsSortedFunc(ir.Menus, func(a, b Menu) int { return cmpString(a.Path, b.Path) }) {
		t.Error("menus are not sorted by path")
	}
	for _, m := range ir.Menus {
		if !slices.IsSortedFunc(m.Fields, func(a, b Field) int { return cmpString(a.Name, b.Name) }) {
			t.Errorf("%s: fields are not sorted by name", m.Path)
		}
	}
	// And it round-trips, so Load is reading what buildir wrote.
	raw, err := json.Marshal(ir)
	if err != nil {
		t.Fatalf("re-encoding: %v", err)
	}
	var again IR
	if err := json.Unmarshal(raw, &again); err != nil {
		t.Fatalf("re-parsing: %v", err)
	}
	if len(again.Menus) != len(ir.Menus) {
		t.Errorf("round-trip lost menus: %d -> %d", len(ir.Menus), len(again.Menus))
	}
}

// TestSourcesAreRecorded keeps the IR traceable to the probes it came from.
func TestSourcesAreRecorded(t *testing.T) {
	ir := load(t)
	if ir.RouterOSVersion == "" {
		t.Error("no RouterOS version stamped")
	}
	want := []string{"console-tree.json", "arg-types.json", "name-uniqueness.json"}
	for _, artifact := range want {
		i := slices.IndexFunc(ir.Sources, func(s Source) bool { return s.Artifact == artifact })
		if i < 0 {
			t.Errorf("%s is not recorded as a source", artifact)
			continue
		}
		if ir.Sources[i].Producer == "" {
			t.Errorf("%s has no producer recorded", artifact)
		}
	}
}

// TestNoInformationLostFromArgTypes checks the join against the artifact
// itself: every menu typedump reached must carry its fields.
func TestNoInformationLostFromArgTypes(t *testing.T) {
	raw, err := os.ReadFile("../config/arg-types.json")
	if err != nil {
		t.Skipf("artifact unavailable: %v", err)
	}
	var dump struct {
		Args []struct {
			Path string `json:"path"`
			Arg  string `json:"arg"`
		} `json:"args"`
	}
	if err := json.Unmarshal(raw, &dump); err != nil {
		t.Fatalf("parsing artifact: %v", err)
	}

	ir := load(t)
	have := map[string]map[string]bool{}
	for _, m := range ir.Menus {
		have[m.Path] = map[string]bool{}
		for _, f := range m.Fields {
			have[m.Path][f.Name] = true
		}
	}
	var missing int
	for _, a := range dump.Args {
		if a.Path == "" {
			continue // the root's own command arguments
		}
		if !have["/"+a.Path][a.Arg] {
			if missing < 5 {
				t.Errorf("/%s %s is in arg-types.json but not in the IR", a.Path, a.Arg)
			}
			missing++
		}
	}
	if missing > 0 {
		t.Errorf("%d fields lost in the join", missing)
	}
}

func cmpString(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
