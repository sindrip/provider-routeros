// inspectdump walks a RouterOS instance's console command tree via the
// /console/inspect REST command and emits it as JSON. The output is the
// router's own authoritative description of its menus, commands, and
// properties — the closest thing RouterOS has to a REST API schema, since the
// REST API is generated from this same tree.
//
// The walk starts at the root and recurses into every non-arg child (dir,
// path, and cmd nodes — command children are their arguments). Rows with
// node-type "arg" are the properties of the enclosing menu or command.
//
// Output is deterministic (children sorted by name then node-type) so a dump
// pinned in-repo diffs cleanly across RouterOS versions. The RouterOS version
// is stamped in the output header.
//
// Usage against the uniqprobe CHR:
//
//	go run ./hack/inspectdump > config/console-tree.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
)

var (
	base = flag.String("base", "http://127.0.0.1:18080/rest", "RouterOS REST endpoint")
	user = flag.String("user", "admin", "RouterOS user")
	pass = flag.String("pass", "", "RouterOS password")
)

var client = &http.Client{}

func rest(method, path string, body any) (int, []byte, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		rdr = strings.NewReader(string(b))
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

// node is one entry in the console tree. Children is nil for leaves (args and
// commands without arguments); Error records a failed child listing without
// aborting the walk.
type node struct {
	Name     string  `json:"name"`
	NodeType string  `json:"node_type"`
	Children []*node `json:"children,omitempty"`
	Error    string  `json:"error,omitempty"`
}

type dump struct {
	RouterOSVersion string  `json:"routeros_version"`
	GeneratedBy     string  `json:"generated_by"`
	Tree            []*node `json:"tree"`
}

const maxDepth = 16

var visited int

// children lists the direct children of the menu path (nil path = root).
func children(path []string) ([]*node, error) {
	body := map[string]any{"request": "child"}
	if len(path) > 0 {
		body["path"] = strings.Join(path, ",")
	}
	code, data, err := rest("POST", "/console/inspect", body)
	if err != nil {
		return nil, err
	}
	if code >= 300 {
		return nil, fmt.Errorf("%d %s", code, trim(data))
	}
	var rows []map[string]any
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("unmarshal: %v: %s", err, trim(data))
	}
	var out []*node
	for _, row := range rows {
		// Each listing leads with a "self" row describing the node itself;
		// only "child" rows are actual children.
		if t, _ := row["type"].(string); t != "child" {
			continue
		}
		name, _ := row["name"].(string)
		if name == "" {
			continue
		}
		nt, _ := row["node-type"].(string)
		out = append(out, &node{Name: name, NodeType: nt})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].NodeType < out[j].NodeType
	})
	return out, nil
}

func walk(path []string, n *node, depth int) {
	visited++
	if visited%100 == 0 {
		fmt.Fprintf(os.Stderr, "%6d nodes, at /%s\n", visited, strings.Join(path, "/"))
	}
	if depth >= maxDepth {
		n.Error = "max depth exceeded"
		return
	}
	kids, err := children(path)
	if err != nil {
		n.Error = err.Error()
		return
	}
	n.Children = kids
	for _, kid := range kids {
		if kid.NodeType == "arg" {
			continue
		}
		walk(append(path, kid.Name), kid, depth+1)
	}
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
	v, _ := m["version"].(string)
	return v
}

func trim(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 160 {
		s = s[:160]
	}
	return s
}

func main() {
	flag.Parse()

	root := &node{Name: "/", NodeType: "path"}
	walk(nil, root, 0)
	if root.Error != "" && root.Children == nil {
		fmt.Fprintf(os.Stderr, "root listing failed: %s\n", root.Error)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "%6d nodes total\n", visited)

	out := dump{
		RouterOSVersion: version(),
		GeneratedBy:     "hack/inspectdump",
		Tree:            root.Children,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
