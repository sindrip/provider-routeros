// buildir assembles the pinned IR from the probe artifacts.
//
// It reads only what a live RouterOS produced — the console tree for
// structure, typedump for types, uniqprobe for identity — and joins them into
// schema/ir.json. Nothing here consults the manual or the upstream Terraform
// schema: both are cross-checks, and both have been wrong.
//
// Output is deterministic (menus by path, fields by name) so the pinned file
// diffs cleanly when RouterOS moves.
//
// schema is its own module, so run it from inside:
//
//	cd schema && go run -buildvcs=false ./cmd/buildir
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sindrip/provider-routeros/schema"
)

var (
	configDir = flag.String("config", "../config", "directory holding the pinned probe artifacts")
	out       = flag.String("out", "ir.json", "where to write the IR")
)

// --- the artifacts, as their producers emit them ---

type treeNode struct {
	Name     string      `json:"name"`
	NodeType string      `json:"node_type"`
	Children []*treeNode `json:"children,omitempty"`
}

type consoleTree struct {
	RouterOSVersion string      `json:"routeros_version"`
	Architecture    string      `json:"architecture"`
	GeneratedBy     string      `json:"generated_by"`
	Tree            []*treeNode `json:"tree"`
}

type argType struct {
	Path    string   `json:"path"`
	Arg     string   `json:"arg"`
	Access  string   `json:"access"`
	Kind    string   `json:"kind"`
	Values  []string `json:"values,omitempty"`
	Types   []string `json:"types,omitempty"`
	Ranges  []string `json:"ranges,omitempty"`
	Command string   `json:"command"`
}

type argTypes struct {
	RouterOSVersion string    `json:"routeros_version"`
	Architecture    string    `json:"architecture"`
	GeneratedBy     string    `json:"generated_by"`
	Args            []argType `json:"args"`
}

type observed struct {
	Path   string `json:"path"`
	Arg    string `json:"arg"`
	Type   string `json:"type"`
	Sample string `json:"sample"`
}

type observedTypes struct {
	RouterOSVersion string     `json:"routeros_version"`
	Architecture    string     `json:"architecture"`
	GeneratedBy     string     `json:"generated_by"`
	Verdicts        []observed `json:"verdicts"`
}

type verdict struct {
	Resource string `json:"resource"`
	Path     string `json:"path"`
	Verdict  string `json:"verdict"`
}

type uniqueness struct {
	RouterOSVersion string    `json:"routeros_version"`
	GeneratedBy     string    `json:"generator"`
	Verdicts        []verdict `json:"verdicts"`
}

func main() {
	flag.Parse()
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	var tree consoleTree
	if err := readJSON(filepath.Join(*configDir, "console-tree.json"), &tree); err != nil {
		return err
	}
	var types argTypes
	if err := readJSON(filepath.Join(*configDir, "arg-types.json"), &types); err != nil {
		return err
	}
	var uniq uniqueness
	if err := readJSON(filepath.Join(*configDir, "name-uniqueness.json"), &uniq); err != nil {
		return err
	}
	// Optional: the observed types close a gap the console cannot, but the
	// IR is still assemblable without them.
	var obs observedTypes
	if err := readJSON(filepath.Join(*configDir, "observed-types.json"), &obs); err != nil {
		fmt.Fprintf(os.Stderr, "no observed types (%v); rowless read-only fields stay untyped\n", err)
	}

	ir := &schema.IR{
		RouterOSVersion: tree.RouterOSVersion,
		GeneratedBy:     "schema/cmd/buildir",
		Sources: []schema.Source{
			{Artifact: "console-tree.json", Producer: tree.GeneratedBy, Version: tree.RouterOSVersion, Platform: tree.Architecture},
			{Artifact: "arg-types.json", Producer: types.GeneratedBy, Version: types.RouterOSVersion, Platform: types.Architecture},
			{Artifact: "name-uniqueness.json", Producer: uniq.GeneratedBy, Version: uniq.RouterOSVersion},
			{Artifact: "observed-types.json", Producer: obs.GeneratedBy, Version: obs.RouterOSVersion, Platform: obs.Architecture},
		},
		Menus: assemble(&tree, &types, &uniq, &obs),
	}

	raw, err := json.MarshalIndent(ir, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding the IR: %w", err)
	}
	if err := os.WriteFile(*out, append(raw, '\n'), 0o644); err != nil { //nolint:gosec // a pinned artifact, not a secret
		return fmt.Errorf("writing %s: %w", *out, err)
	}

	c := ir.Census()
	fmt.Fprintf(os.Stderr, "%d menus: %d ordered, %d list, %d singleton; %d writable\n",
		len(ir.Menus), c[schema.ClassOrdered], c[schema.ClassList], c[schema.ClassSingleton], writable(ir))
	return nil
}

func writable(ir *schema.IR) int {
	var n int
	for _, m := range ir.Menus {
		if m.Writable {
			n++
		}
	}
	return n
}

func assemble(tree *consoleTree, types *argTypes, uniq *uniqueness, obs *observedTypes) []schema.Menu {
	fields := fieldsByMenu(types, obs)
	verdicts := verdictsByMenu(uniq)
	rowless := rowlessByMenu(types)

	var menus []schema.Menu
	var walk func(nodes []*treeNode, path []string)
	walk = func(nodes []*treeNode, path []string) {
		for _, n := range nodes {
			p := append(append([]string{}, path...), n.Name)
			if n.NodeType != "dir" && n.NodeType != "path" {
				continue
			}
			menuPath := strings.Join(p, "/")
			var cmds []string
			for _, c := range n.Children {
				if c.NodeType == "cmd" {
					cmds = append(cmds, c.Name)
				}
			}
			slices.Sort(cmds)

			// A node with neither print nor get is a namespace, not a
			// menu: /ip and /system group other menus but hold nothing
			// readable of their own.
			if !slices.Contains(cmds, "print") && !slices.Contains(cmds, "get") {
				walk(n.Children, p)
				continue
			}

			f, typed := fields[menuPath]
			slices.SortFunc(f, func(a, b schema.Field) int { return strings.Compare(a.Name, b.Name) })

			class, ev := classify(cmds, rowless[menuPath])
			menus = append(menus, schema.Menu{
				Path:          "/" + menuPath,
				Class:         class,
				Commands:      cmds,
				Identity:      identify(f, verdicts[menuPath]),
				Fields:        f,
				Typed:         typed,
				Writable:      slices.Contains(cmds, "add") || slices.Contains(cmds, "set"),
				ClassEvidence: ev,
			})
			walk(n.Children, p)
		}
	}
	walk(tree.Tree, nil)
	slices.SortFunc(menus, func(a, b schema.Menu) int { return strings.Compare(a.Path, b.Path) })
	return menus
}

// classify derives a menu's shape, preferring evidence over inference.
//
// move is decisive on its own: only a menu with rows can reorder them.
//
// Otherwise the question is whether the menu holds rows, and the tempting
// inference — no add command, therefore no rows — is simply false:
// /interface, /interface/ethernet, /routing/route and /ip/firewall/connection
// all hold rows without one. hack/typedump settled it per menu by discovering
// which command could enumerate the properties, and records the answer, so
// that is used where it exists. Only a menu typedump never reached falls back
// to the command list, and says so.
func classify(cmds []string, rows rowEvidence) (schema.Class, schema.Evidence) {
	has := func(c string) bool { return slices.Contains(cmds, c) }
	if has("move") {
		return schema.ClassOrdered, schema.Probed
	}
	switch rows {
	case rowsPresent:
		return schema.ClassList, schema.Probed
	case rowsAbsent:
		return schema.ClassSingleton, schema.Probed
	case rowsUnknown:
		// typedump never reached this menu; fall through to inference.
	}
	// No evidence: add proves rows exist, find implies them. Anything else
	// is taken for a singleton, and flagged as a guess.
	if has("add") || has("find") {
		return schema.ClassList, schema.Inferred
	}
	return schema.ClassSingleton, schema.Inferred
}

// rowEvidence is what hack/typedump discovered about a menu's cardinality.
type rowEvidence int

const (
	rowsUnknown rowEvidence = iota
	rowsPresent
	rowsAbsent
)

// rowlessByMenu reads typedump's per-menu enumeration command as a cardinality
// oracle: read-only properties reached through `print where` mean the menu has
// rows to filter, and through `get` mean it has none.
func rowlessByMenu(types *argTypes) map[string]rowEvidence {
	out := map[string]rowEvidence{}
	for _, a := range types.Args {
		if a.Access != "read-only" {
			continue
		}
		switch a.Command {
		case "print":
			out[a.Path] = rowsPresent
		case "get":
			if out[a.Path] != rowsPresent {
				out[a.Path] = rowsAbsent
			}
		}
	}
	return out
}

// identityCandidates are the fields that could key a row, most preferred
// first. name is the router's own notion of a handle; comment is the fallback
// the bridge already relies on for menus that have no name.
var identityCandidates = []string{"name", "comment"}

func identify(fields []schema.Field, v string) schema.Identity {
	id := schema.Identity{Verdict: schema.Unprobed}
	for _, want := range identityCandidates {
		if slices.ContainsFunc(fields, func(f schema.Field) bool { return f.Name == want }) {
			id.Candidates = append(id.Candidates, want)
		}
	}
	if v != "" {
		id.Verdict = schema.Verdict(v)
	}
	// Only a proven-unique verdict promotes a candidate to the key. An
	// untested or unprobed menu has no key, which is the honest answer and
	// the one an emitter must be able to see.
	if id.Verdict == schema.Unique && len(id.Candidates) > 0 {
		id.Key = id.Candidates[0]
	}
	return id
}

func fieldsByMenu(types *argTypes, obs *observedTypes) map[string][]schema.Field {
	seen := map[string]observed{}
	for _, o := range obs.Verdicts {
		seen[o.Path+"\x00"+o.Arg] = o
	}
	out := map[string][]schema.Field{}
	for _, a := range types.Args {
		if a.Path == "" {
			continue // the root menu's own command arguments
		}
		f := schema.Field{
			Name:   a.Arg,
			Access: schema.Access(a.Access),
			Kind:   schema.Kind(a.Kind),
			Values: a.Values,
			Ranges: a.Ranges,
			Bool:   isBool(a),
		}
		if len(a.Types) > 0 {
			f.Type = strings.Join(a.Types, "|")
		}
		// Any kind at all means the console described the field: a closed
		// vocabulary is as much an answer as a grammar is.
		if f.Kind != schema.KindUnknown {
			f.Evidence = schema.Probed
		}
		// Where the console said nothing, a value the router returned may
		// still say something. It never overrides a probed type.
		if f.Kind == schema.KindUnknown {
			if o, ok := seen[a.Path+"\x00"+a.Arg]; ok {
				f.Kind, f.Type, f.Sample, f.Evidence = schema.KindScalar, o.Type, o.Sample, schema.Observed
				if o.Type == "boolean" {
					f.Bool = true
				}
			}
		}
		out[a.Path] = append(out[a.Path], f)
	}
	return out
}

// isBool reports whether a field's vocabulary is exactly no/yes.
//
// The router states booleans that way, which is how a bool is told apart from
// an enum that merely looks like one — advertise-mac-address answers no/yes
// and is a bool; advertise-dns answers no/self/yes and is not.
func isBool(a argType) bool {
	if a.Kind != "enum" || len(a.Values) != 2 {
		return false
	}
	v := slices.Clone(a.Values)
	slices.Sort(v)
	return v[0] == "no" && v[1] == "yes"
}

func verdictsByMenu(uniq *uniqueness) map[string]string {
	out := map[string]string{}
	for _, v := range uniq.Verdicts {
		path := strings.TrimPrefix(v.Path, "/")
		// A menu probed more than once (several upstream resources can map
		// to one path) keeps the strongest claim: a single DUPLICATE is
		// disqualifying, and UNIQUE outranks UNTESTED.
		switch {
		case out[path] == string(schema.Duplicate) || v.Verdict == string(schema.Duplicate):
			out[path] = string(schema.Duplicate)
		case out[path] == string(schema.Unique) || v.Verdict == string(schema.Unique):
			out[path] = string(schema.Unique)
		default:
			out[path] = v.Verdict
		}
	}
	return out
}

func readJSON(path string, v any) error {
	raw, err := os.ReadFile(path) //nolint:gosec // paths come from flags, this is a build tool
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	return nil
}
