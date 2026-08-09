// typedump probes a live RouterOS instance for the datatype of every console
// property and emits the result as JSON. Where hack/inspectdump records the
// shape of the command tree — which menus exist and what arguments they take
// — this records what each of those properties will actually accept.
//
// The router is its own type oracle. /console/inspect takes an `input`
// holding a partial command line, and answers two ways:
//
//   - request=completion lists the candidate values at the cursor. Rows with
//     show=true are the property's vocabulary; a row completing to "<value>"
//     means a freeform literal is accepted there too. So "advertise-dns="
//     answers no/self/yes and "advertise-mac-address=" answers no/yes, which
//     is the difference between an enum and a bool stated by the router
//     rather than inferred from a schema or a manual page.
//
//   - request=syntax expands the grammar behind that value: named types,
//     alternation, and the bounds and units of scalars ("1..65535 (integer
//     number)", "0s..18h12m15s (time interval)").
//
// Closed enums are fully described by their vocabulary, so syntax is only
// requested for properties that accept a freeform value — that is the half
// where it says something completion did not.
//
// Two passes, because a menu's properties are not all reachable the same way:
//
//   - writable properties are the arguments of `add` and `set`, probed as
//     "/menu/set prop=". `set` is preferred over `add` as the steady-state
//     form, falling back to `add` for creation-only arguments.
//
//   - read-only properties never appear under add or set, so a writable-only
//     sweep cannot see them at all — loop-protect-status is the case that
//     exposed this. They are enumerated from "/menu/print where " (which
//     completes to every property the menu has, read-only included) and
//     typed as "/menu/print where prop=". A mistyped read-only field is not
//     harmless: it is observed into atProvider, so a bool over the router's
//     sha256 reads back false.
//
// Output is sorted by menu then property so a dump pinned in-repo diffs
// cleanly across RouterOS versions, and the version is stamped in the header.
//
// typedump is its own module, so go must run from inside it rather than from
// the repo root (which the root go.mod claims). Against the hack/chr CHR,
// reading the pinned tree:
//
//	cd hack/typedump
//	go run -buildvcs=false . -tree ../../config/console-tree.json > ../../config/arg-types.json
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	base    = flag.String("base", "http://127.0.0.1:18080/rest", "RouterOS REST endpoint")
	user    = flag.String("user", "admin", "RouterOS user")
	pass    = flag.String("pass", "", "RouterOS password")
	treeIn  = flag.String("tree", "../../config/console-tree.json", "console tree dump to read menus from")
	workers = flag.Int("workers", 8, "concurrent probes")
)

// Connections are reused across probes; the CHR answers a probe in single
// digit milliseconds and a fresh handshake each time dominates that.
var client = &http.Client{
	Timeout:   30 * time.Second,
	Transport: &http.Transport{MaxIdleConnsPerHost: 64},
}

func rest(method, path string, body any) (int, []byte, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, *base+path, rdr)
	if err != nil {
		return 0, nil, err
	}
	req.SetBasicAuth(*user, *pass)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data, nil
}

// inspect issues one /console/inspect probe. The CHR drops the occasional
// request under TCG (hack/chr/run.sh says as much about boot), so transport
// errors and 5xx are retried; a 4xx is the router's answer and is returned.
func inspect(request, input string) ([]map[string]any, error) {
	body := map[string]any{"request": request, "input": input}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 250 * time.Millisecond)
		}
		code, data, err := rest("POST", "/console/inspect", body)
		if err != nil {
			lastErr = err
			continue
		}
		if code >= 500 {
			lastErr = fmt.Errorf("%d %s", code, trim(data))
			continue
		}
		if code >= 300 {
			return nil, fmt.Errorf("%d %s", code, trim(data))
		}
		var rows []map[string]any
		if err := json.Unmarshal(data, &rows); err != nil {
			return nil, fmt.Errorf("unmarshal: %v: %s", err, trim(data))
		}
		return rows, nil
	}
	return nil, lastErr
}

// shown returns the completion candidates the console would display. Rows
// with show=false are console furniture — whitespace, syntax-meta, the "*"
// id prefix — and "<value>" marks that a freeform literal is accepted, which
// is reported separately.
func shown(rows []map[string]any) (values []string, freeform bool) {
	for _, r := range rows {
		text, _ := r["completion"].(string)
		if text == "<value>" {
			freeform = true
			continue
		}
		if s, _ := r["show"].(string); s == "true" && text != "" {
			values = append(values, text)
		}
	}
	sort.Strings(values)
	return values, freeform
}

// node mirrors hack/inspectdump's output.
type node struct {
	Name     string  `json:"name"`
	NodeType string  `json:"node_type"`
	Children []*node `json:"children,omitempty"`
}

type tree struct {
	RouterOSVersion string  `json:"routeros_version"`
	Tree            []*node `json:"tree"`
}

// syntaxRow is one line of a property's expanded grammar. Depth nests
// definitions under the symbol that referenced them.
type syntaxRow struct {
	Depth      int    `json:"depth"`
	Symbol     string `json:"symbol,omitempty"`
	SymbolType string `json:"symbol_type,omitempty"`
	Text       string `json:"text,omitempty"`
}

// argType is the router's answer for one property.
//
// Kind is the shape of the value space:
//
//	enum       a closed set; Values is exhaustive
//	open-enum  suggested values plus any freeform literal
//	scalar     freeform only; Types and Ranges describe it
//	unknown    the router offered neither (see Error)
type argType struct {
	Path    string      `json:"path"`
	Command string      `json:"command"`
	Arg     string      `json:"arg"`
	Access  string      `json:"access"` // writable | read-only
	Kind    string      `json:"kind"`
	Values  []string    `json:"values,omitempty"`
	Types   []string    `json:"types,omitempty"`
	Ranges  []string    `json:"ranges,omitempty"`
	Syntax  []syntaxRow `json:"syntax,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// input is the partial command line that puts the cursor at this property's
// value. Read-only properties have no add/set form, so they are approached
// through the print filter instead.
func (a *argType) input() string {
	if a.Access == "read-only" {
		return "/" + a.Path + "/print where " + a.Arg + "="
	}
	return "/" + a.Path + "/" + a.Command + " " + a.Arg + "="
}

type dump struct {
	RouterOSVersion string     `json:"routeros_version"`
	GeneratedBy     string     `json:"generated_by"`
	Note            string     `json:"note"`
	Args            []*argType `json:"args"`
}

const note = "Types as the router states them, via /console/inspect request=completion " +
	"(vocabulary) and request=syntax (grammar of freeform values). kind: enum = closed set, " +
	"open-enum = suggestions plus freeform, scalar = freeform only. access: writable = an " +
	"add/set argument, read-only = a property only the print filter exposes."

// Console bookkeeping that "print where" lists alongside real properties.
var notProperties = map[string]bool{
	".dead": true, ".id": true, ".nextid": true, "about": true,
	"numbers": true, // addresses rows for `set`, not a property
}

// writable walks the tree and returns one probe per (menu, argument), keyed
// "path\x00arg". `set` wins over `add` when both declare the argument.
func writable(nodes []*node) map[string]*argType {
	out := map[string]*argType{}
	var walk func(nodes []*node, path []string)
	walk = func(nodes []*node, path []string) {
		for _, n := range nodes {
			p := append(append([]string{}, path...), n.Name)
			switch n.NodeType {
			case "dir", "path":
				walk(n.Children, p)
			case "cmd":
				// Command children are the command's arguments;
				// only creation and mutation carry properties.
				if n.Name != "add" && n.Name != "set" {
					continue
				}
				for _, a := range n.Children {
					if a.NodeType != "arg" || notProperties[a.Name] {
						continue
					}
					key := strings.Join(path, "/") + "\x00" + a.Name
					if prev, ok := out[key]; ok && prev.Command == "set" {
						continue
					}
					out[key] = &argType{
						Path:    strings.Join(path, "/"),
						Command: n.Name,
						Arg:     a.Name,
						Access:  "writable",
					}
				}
			}
		}
	}
	walk(nodes, nil)
	return out
}

// printable returns every menu that has a `print` command, which is where
// read-only properties can be enumerated from.
func printable(nodes []*node) []string {
	var out []string
	var walk func(nodes []*node, path []string)
	walk = func(nodes []*node, path []string) {
		for _, n := range nodes {
			p := append(append([]string{}, path...), n.Name)
			switch n.NodeType {
			case "dir", "path":
				for _, c := range n.Children {
					if c.NodeType == "cmd" && c.Name == "print" {
						out = append(out, strings.Join(p, "/"))
						break
					}
				}
				walk(n.Children, p)
			}
		}
	}
	walk(nodes, nil)
	sort.Strings(out)
	return out
}

var (
	// "1..65535    (integer number)" — the label names the type, the
	// bounds carry the unit.
	labelRe = regexp.MustCompile(`\(([^)]+)\)\s*$`)
	rangeRe = regexp.MustCompile(`(\S+)\.\.(\S+)`)
)

// probe fills in one property's type. Completion is asked first; syntax only
// when a freeform value is accepted, which is where completion goes silent.
func probe(a *argType) {
	input := a.input()

	rows, err := inspect("completion", input)
	if err != nil {
		a.Kind, a.Error = "unknown", err.Error()
		return
	}
	values, freeform := shown(rows)
	a.Values = values

	switch {
	case len(a.Values) > 0 && !freeform:
		a.Kind = "enum"
		return // a closed set needs no grammar
	case len(a.Values) > 0:
		a.Kind = "open-enum"
	case freeform:
		a.Kind = "scalar"
	default:
		// Completion went silent. Some of these are console furniture
		// whose candidates are existing rows (copy-from, place-before)
		// and stay silent even on a populated router; others are
		// valueless flags, or free text the console will not guess at.
		// Syntax still answers for the last group, so ask before
		// giving up.
		a.Kind = "unknown"
	}

	rows, err = inspect("syntax", input)
	if err != nil {
		a.Error = err.Error()
		return
	}
	seenType, seenRange := map[string]bool{}, map[string]bool{}
	for _, r := range rows {
		depth, _ := strconv.Atoi(str(r["nested"]))
		row := syntaxRow{
			Depth:      depth,
			Symbol:     str(r["symbol"]),
			SymbolType: str(r["symbol-type"]),
			Text:       str(r["text"]),
		}
		a.Syntax = append(a.Syntax, row)

		if m := labelRe.FindStringSubmatch(row.Text); m != nil {
			if t := strings.TrimSpace(m[1]); !seenType[t] {
				seenType[t] = true
				a.Types = append(a.Types, t)
			}
		} else if strings.Contains(row.Text, "string value") && !seenType["string"] {
			seenType["string"] = true
			a.Types = append(a.Types, "string")
		}
		if m := rangeRe.FindStringSubmatch(row.Text); m != nil {
			if rg := m[1] + ".." + m[2]; !seenRange[rg] {
				seenRange[rg] = true
				a.Ranges = append(a.Ranges, rg)
			}
		}
	}
	sort.Strings(a.Types)
	sort.Strings(a.Ranges)
	// Syntax named a type where completion offered nothing, so the
	// property does take a freeform value after all.
	if a.Kind == "unknown" && len(a.Types) > 0 {
		a.Kind = "scalar"
	}
}

// each runs fn over items with the configured worker count, reporting
// progress under label.
func each[T any](label string, items []T, fn func(T)) {
	var (
		wg   sync.WaitGroup
		next = make(chan T)
		mu   sync.Mutex
		done int
	)
	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for it := range next {
				fn(it)
				mu.Lock()
				done++
				if done%250 == 0 {
					fmt.Fprintf(os.Stderr, "%6d/%d %s\n", done, len(items), label)
				}
				mu.Unlock()
			}
		}()
	}
	for _, it := range items {
		next <- it
	}
	close(next)
	wg.Wait()
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func trim(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 160 {
		s = s[:160]
	}
	return s
}

func version() string {
	code, data, err := rest("GET", "/system/resource", nil)
	if err != nil || code >= 300 {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(data, &m) != nil {
		return ""
	}
	return str(m["version"])
}

func main() {
	flag.Parse()

	raw, err := os.ReadFile(*treeIn)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var t tree
	if err := json.Unmarshal(raw, &t); err != nil {
		fmt.Fprintf(os.Stderr, "parse %s: %v\n", *treeIn, err)
		os.Exit(1)
	}

	byKey := writable(t.Tree)
	fmt.Fprintf(os.Stderr, "%d writable arguments from the tree\n", len(byKey))

	// Read-only properties are not in the tree at all: ask each menu what
	// it has and keep whatever add/set did not already claim.
	menus := printable(t.Tree)
	var mu sync.Mutex
	each("menus enumerated", menus, func(menu string) {
		rows, err := inspect("completion", "/"+menu+"/print where ")
		if err != nil {
			return
		}
		props, _ := shown(rows)
		mu.Lock()
		defer mu.Unlock()
		for _, p := range props {
			if notProperties[p] {
				continue
			}
			key := menu + "\x00" + p
			if _, ok := byKey[key]; ok {
				continue
			}
			byKey[key] = &argType{
				Path: menu, Command: "print", Arg: p, Access: "read-only",
			}
		}
	})

	args := make([]*argType, 0, len(byKey))
	for _, a := range byKey {
		args = append(args, a)
	}
	sort.Slice(args, func(i, j int) bool {
		if args[i].Path != args[j].Path {
			return args[i].Path < args[j].Path
		}
		return args[i].Arg < args[j].Arg
	})
	ro := 0
	for _, a := range args {
		if a.Access == "read-only" {
			ro++
		}
	}
	fmt.Fprintf(os.Stderr, "%d properties to probe (%d writable, %d read-only across %d menus)\n",
		len(args), len(args)-ro, ro, len(menus))

	each("probed", args, probe)

	kinds := map[string]int{}
	for _, a := range args {
		kinds[a.Kind]++
	}
	fmt.Fprintf(os.Stderr, "done: enum=%d open-enum=%d scalar=%d unknown=%d\n",
		kinds["enum"], kinds["open-enum"], kinds["scalar"], kinds["unknown"])

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(dump{
		RouterOSVersion: version(),
		GeneratedBy:     "hack/typedump",
		Note:            note,
		Args:            args,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
