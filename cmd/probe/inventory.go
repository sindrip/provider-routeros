package main

import (
	"cmp"
	"context"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/sindrip/routeros"
)

type pathInfo struct {
	Path     string   `json:"path"`
	NodeType string   `json:"node_type"`
	Commands []string `json:"commands,omitempty"`
	Get      string   `json:"get"`
	GetError string   `json:"get_error,omitempty"`
	Class    string   `json:"class"`
	Error    string   `json:"error,omitempty"`
}

const maxDepth = 16

func inventory(ctx context.Context, c *routeros.Client) ([]pathInfo, error) {
	var out []pathInfo

	if err := walk(ctx, c, nil, "path", &out, 0); err != nil {
		return nil, err
	}

	slices.SortFunc(out, func(a, b pathInfo) int { return cmp.Compare(a.Path, b.Path) })

	return out, nil
}

func walk(ctx context.Context, c *routeros.Client, segs []string, nodeType string, out *[]pathInfo, depth int) error {
	if depth >= maxDepth {
		return fmt.Errorf("/%s: max depth exceeded", strings.Join(segs, "/"))
	}

	kids, err := children(ctx, c, segs)

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

		slices.Sort(info.Commands)

		info.Get, info.GetError, info.Class = classify(ctx, c, info.Path, info.Commands)
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

// Each listing leads with a "self" row describing the node itself.
func children(ctx context.Context, c *routeros.Client, segs []string) ([]child, error) {
	args := map[string]string{"request": "child"}

	if len(segs) > 0 {
		args["path"] = strings.Join(segs, ",")
	}

	rows, err := c.Post[[]map[string]string](ctx, "/console/inspect", args)
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

	slices.SortFunc(out, func(a, b child) int {
		return cmp.Or(cmp.Compare(a.name, b.name), cmp.Compare(a.nodeType, b.nodeType))
	})

	return out, nil
}

// Only 400/404 on a print-less path means "not a menu"; any other
// failure, or a print-bearing path GET cannot read, stays unknown.
func classify(ctx context.Context, c *routeros.Client, path string, commands []string) (get, getErr, class string) {
	body, err := c.Get[jsontext.Value](ctx, path)
	if err != nil {
		re, ok := errors.AsType[*routeros.Error](err)
		if !ok {
			return "", err.Error(), "unknown"
		}

		get = fmt.Sprintf("%d", re.Status)
		getErr = re.Message

		if re.Detail != "" {
			getErr += ": " + re.Detail
		}

		notFound := re.Status == 400 || re.Status == 404

		if notFound && !slices.Contains(commands, "print") {
			return get, getErr, "none"
		}

		return get, getErr, "unknown"
	}

	switch body.Kind() {
	case '[':
		return "rows", "", "rows"
	case '{':
		return "record", "", "singleton"
	default:
		return "unrecognized body", "", "unknown"
	}
}
