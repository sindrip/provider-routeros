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
	GeneratedBy     string    `json:"generated_by"`
	Args            []argType `json:"args"`
}

type verdict struct {
	Resource string `json:"resource"`
	Path     string `json:"path"`
	Verdict  string `json:"verdict"`
}

type uniqueness struct {
	RouterOSVersion string    `json:"routeros_version"`
	GeneratedBy     string    `json:"generated_by"`
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

	ir := &schema.IR{
		RouterOSVersion: tree.RouterOSVersion,
		GeneratedBy:     "schema/cmd/buildir",
		Sources: []schema.Source{
			{Artifact: "console-tree.json", Producer: tree.GeneratedBy, Version: tree.RouterOSVersion},
			{Artifact: "arg-types.json", Producer: types.GeneratedBy, Version: types.RouterOSVersion},
			{Artifact: "name-uniqueness.json", Producer: uniq.GeneratedBy, Version: uniq.RouterOSVersion},
		},
		Menus: assemble(&tree, &types, &uniq),
	}

	raw, err := json.MarshalIndent(ir, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding the IR: %w", err)
	}
	if err := os.WriteFile(*out, append(raw, '\n'), 0o644); err != nil { //nolint:gosec // a pinned artifact, not a secret
		return fmt.Errorf("writing %s: %w", *out, err)
	}

	c := ir.Census()
	fmt.Fprintf(os.Stderr, "%d menus: %d ordered, %d list, %d singleton, %d read-only\n",
		len(ir.Menus), c[schema.ClassOrdered], c[schema.ClassList], c[schema.ClassSingleton], c[schema.ClassReadOnly])
	return nil
}

func assemble(tree *consoleTree, types *argTypes, uniq *uniqueness) []schema.Menu {
	fields := fieldsByMenu(types)
	verdicts := verdictsByMenu(uniq)

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

			f, typed := fields[menuPath]
			slices.SortFunc(f, func(a, b schema.Field) int { return strings.Compare(a.Name, b.Name) })

			menus = append(menus, schema.Menu{
				Path:     "/" + menuPath,
				Class:    classify(cmds),
				Commands: cmds,
				Identity: identify(f, verdicts[menuPath]),
				Fields:   f,
				Typed:    typed,
			})
			walk(n.Children, p)
		}
	}
	walk(tree.Tree, nil)
	slices.SortFunc(menus, func(a, b schema.Menu) int { return strings.Compare(a.Path, b.Path) })
	return menus
}

// classify derives a menu's shape from the commands it exposes.
//
// move implies order carries meaning; add implies rows; set without add is a
// settings singleton. Nothing else is consulted, because nothing else is
// reliable — an absent add does not imply an absent row, as interface/ethernet
// and the read-only tables demonstrate.
func classify(cmds []string) schema.Class {
	has := func(c string) bool { return slices.Contains(cmds, c) }
	switch {
	case has("move"):
		return schema.ClassOrdered
	case has("add"):
		return schema.ClassList
	case has("set"):
		return schema.ClassSingleton
	default:
		return schema.ClassReadOnly
	}
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

func fieldsByMenu(types *argTypes) map[string][]schema.Field {
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
