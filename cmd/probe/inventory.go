package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/sindrip/routeros"
)

// pathInfo is one path's verdict: what commands it bears, what GET
// returned, and the menu class that follows from those two.
type pathInfo struct {
	Path     string   `json:"path"`
	NodeType string   `json:"node_type"`
	Commands []string `json:"commands,omitempty"`
	Get      string   `json:"get"`             // "rows", "record", or the HTTP status
	Class    string   `json:"class"`           // rows | singleton | none | unknown
	Error    string   `json:"error,omitempty"` // inspect failure; class stays unknown
}

const maxDepth = 16

// inventory walks the console tree and gives every dir and path node a
// verdict. Unknowns stay unknown: a failed listing or a contradiction is
// recorded, never guessed away.
func inventory(ctx context.Context, c *routeros.Client) ([]pathInfo, error) {
	var out []pathInfo

	if err := walk(ctx, c, nil, "path", &out, 0); err != nil {
		return nil, err
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })

	return out, nil
}

func walk(ctx context.Context, c *routeros.Client, segs []string, nodeType string, out *[]pathInfo, depth int) error {
	if depth >= maxDepth {
		return fmt.Errorf("/%s: max depth exceeded", strings.Join(segs, "/"))
	}

	kids, err := children(ctx, c, segs)

	// The root must list; below it a failure is that path's verdict.
	if len(segs) == 0 {
		if err != nil {
			return err
		}
	} else {
		info := pathInfo{
			Path:     "/" + strings.Join(segs, "/"),
			NodeType: nodeType,
		}

		if err != nil {
			info.Error = err.Error()
			info.Class = "unknown"
			*out = append(*out, info)

			return nil //nolint:nilerr // recorded as the path's verdict
		}

		for _, k := range kids {
			if k.nodeType == "cmd" {
				info.Commands = append(info.Commands, k.name)
			}
		}

		sort.Strings(info.Commands)

		info.Get, info.Class = classify(ctx, c, info.Path, info.Commands)
		*out = append(*out, info)
	}

	for _, k := range kids {
		if k.nodeType != "dir" && k.nodeType != "path" {
			continue
		}

		if err := walk(ctx, c, append(segs, k.name), k.nodeType, out, depth+1); err != nil {
			return err
		}
	}

	return nil
}

type child struct {
	name     string
	nodeType string
}

// children lists the direct children of the path (nil = root). Each
// listing leads with a "self" row describing the node itself; only
// "child" rows are children.
func children(ctx context.Context, c *routeros.Client, segs []string) ([]child, error) {
	args := map[string]string{"request": "child"}

	if len(segs) > 0 {
		args["path"] = strings.Join(segs, ",")
	}

	rows, err := c.Exec(ctx, "/console/inspect", args)
	if err != nil {
		return nil, err
	}

	var out []child

	for _, row := range rows {
		if row["type"] != "child" || row["name"] == "" {
			continue
		}

		out = append(out, child{name: row["name"], nodeType: row["node-type"]})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].name != out[j].name {
			return out[i].name < out[j].name
		}

		return out[i].nodeType < out[j].nodeType
	})

	return out, nil
}

// classify observes what GET returns for the path and derives the class.
// A print-bearing path that GET cannot read is a contradiction, not a
// container — unknown, never guessed.
func classify(ctx context.Context, c *routeros.Client, path string, commands []string) (get, class string) {
	status, body, err := c.Raw(ctx, "GET", path, nil)
	if err != nil {
		return "error: " + err.Error(), "unknown"
	}

	hasPrint := false

	for _, cmd := range commands {
		if cmd == "print" {
			hasPrint = true
		}
	}

	if status != 200 {
		get = fmt.Sprintf("%d", status)

		if hasPrint {
			return get, "unknown"
		}

		return get, "none"
	}

	switch firstByte(body) {
	case '[':
		return "rows", "rows"
	case '{':
		return "record", "singleton"
	default:
		return "unrecognized body", "unknown"
	}
}

func firstByte(b []byte) byte {
	for _, c := range b {
		switch c {
		case ' ', '\t', '\n', '\r':
			continue
		}

		return c
	}

	return 0
}
