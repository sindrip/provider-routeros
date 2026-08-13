package main

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/sindrip/routeros"
)

type menuFields struct {
	Path string              `json:"path"`
	Args map[string][]string `json:"args"`
}

// fields pins each menu's argument vocabulary: for every command the
// menu bears, the args the console declares for it.
func fields(ctx context.Context, c *routeros.Client, paths []pathInfo) ([]menuFields, error) {
	var out []menuFields

	for _, p := range paths {
		if p.Class != "rows" && p.Class != "singleton" {
			continue
		}

		mf := menuFields{Path: p.Path, Args: map[string][]string{}}
		segs := strings.Split(strings.TrimPrefix(p.Path, "/"), "/")

		for _, command := range p.Commands {
			kids, err := children(ctx, c, append(segs, command))
			if err != nil {
				return nil, err
			}

			args := []string{}

			for _, k := range kids {
				if k.nodeType == "arg" {
					args = append(args, k.name)
				}
			}

			slices.Sort(args)
			mf.Args[command] = args
		}

		out = append(out, mf)
	}

	slices.SortFunc(out, func(a, b menuFields) int { return cmp.Compare(a.Path, b.Path) })

	return out, nil
}
