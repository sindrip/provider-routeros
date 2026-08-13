package main

import (
	"cmp"
	"context"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/sindrip/routeros"
)

// Console bookkeeping that enumeration lists alongside real properties.
var notProperties = map[string]bool{
	".dead": true, ".id": true, ".nextid": true, "about": true,
	"numbers": true, "value-name": true, "=": true,
}

type argType struct {
	Path    string         `json:"path"`
	Command string         `json:"command,omitempty"`
	Arg     string         `json:"arg"`
	Access  string         `json:"access"`
	Kind    string         `json:"kind"`
	Values  []string       `json:"values,omitempty"`
	Syntax  jsontext.Value `json:"syntax,omitempty"`
	Error   string         `json:"error,omitempty"`

	input string
}

// types asks the router for each property's value space: completion for
// vocabulary (enum / open-enum / scalar), syntax for the grammar of
// freeform values. Read-only properties of get-enumerated menus have no
// console cursor position, so they are unknown by construction.
func types(ctx context.Context, c *routeros.Client, paths []pathInfo, menus []menuFields) ([]*argType, error) {
	fieldsOf := map[string]menuFields{}
	for _, m := range menus {
		fieldsOf[m.Path] = m
	}

	var probes []*argType

	for _, p := range paths {
		if p.Class != "rows" && p.Class != "singleton" {
			continue
		}

		m := fieldsOf[p.Path]
		writable := map[string]string{}

		for _, command := range []string{"add", "set"} {
			for _, arg := range m.Args[command] {
				if !notProperties[arg] {
					writable[arg] = command
				}
			}
		}

		for arg, command := range writable {
			probes = append(probes, &argType{
				Path: p.Path, Command: command, Arg: arg, Access: "writable",
				input: p.Path + "/" + command + " " + arg + "=",
			})
		}

		enumerated, err := enumerate(ctx, c, p, m)
		if err != nil {
			probes = append(probes, &argType{Path: p.Path, Access: "read-only", Kind: "unknown", Error: err.Error()})
			continue
		}

		for _, arg := range enumerated {
			if notProperties[arg] || writable[arg] != "" {
				continue
			}

			at := &argType{Path: p.Path, Arg: arg, Access: "read-only"}

			if p.Class == "rows" {
				at.input = p.Path + "/print where " + arg + "="
			}

			probes = append(probes, at)
		}
	}

	probeAll(ctx, c, probes)
	slices.SortFunc(probes, func(a, b *argType) int {
		return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Arg, b.Arg))
	})

	return probes, nil
}

// enumerate lists every property a menu exposes: `print where` completes
// to them on rows menus; `get` does on singletons, mixed with get's own
// arguments, which are subtracted via the fields observation.
func enumerate(ctx context.Context, c *routeros.Client, p pathInfo, m menuFields) ([]string, error) {
	if p.Class == "rows" {
		values, _, err := completion(ctx, c, p.Path+"/print where ")
		return values, err
	}

	values, _, err := completion(ctx, c, p.Path+"/get ")
	if err != nil {
		return nil, err
	}

	own := slices.Concat(m.Args["get"], m.Args["print"])

	return slices.DeleteFunc(values, func(v string) bool { return slices.Contains(own, v) }), nil
}

func probeAll(ctx context.Context, c *routeros.Client, probes []*argType) {
	var wg sync.WaitGroup

	sem := make(chan struct{}, 8)

	for _, at := range probes {
		if at.input == "" {
			if at.Kind == "" {
				at.Kind = "unknown"
			}

			continue
		}

		wg.Add(1)

		sem <- struct{}{}

		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			probeOne(ctx, c, at)
		}()
	}

	wg.Wait()
}

func probeOne(ctx context.Context, c *routeros.Client, at *argType) {
	rows, err := inspect(ctx, c, "completion", at.input)
	if err != nil {
		at.Kind = "unknown"
		at.Error = err.Error()

		return
	}

	values, freeform := shown(rows)
	at.Values = values

	switch {
	case len(values) > 0 && !freeform:
		at.Kind = "enum"
	case len(values) > 0:
		at.Kind = "open-enum"
	case freeform:
		at.Kind = "scalar"
	default:
		at.Kind = "unknown"

		return
	}

	// Syntax only for pure scalars, always via input=: request=syntax on
	// reference args via path= crashes the console (forum #149360).
	if at.Kind != "scalar" {
		return
	}

	syntax, err := c.Post[jsontext.Value](ctx, "/console/inspect", map[string]string{"request": "syntax", "input": at.input})
	if err != nil {
		at.Error = err.Error()
		return
	}

	at.Syntax = syntax
}

func completion(ctx context.Context, c *routeros.Client, input string) ([]string, bool, error) {
	rows, err := inspect(ctx, c, "completion", input)
	if err != nil {
		return nil, false, err
	}

	values, freeform := shown(rows)

	return values, freeform, nil
}

// The CHR drops the occasional request under load; transport errors and
// 5xx are retried, a 4xx is the router's answer.
func inspect(ctx context.Context, c *routeros.Client, request, input string) ([]map[string]string, error) {
	var lastErr error

	for attempt := range 3 {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 250 * time.Millisecond)
		}

		rows, err := c.Post[[]map[string]string](ctx, "/console/inspect", map[string]string{"request": request, "input": input})
		if err == nil {
			return rows, nil
		}

		lastErr = err

		re, ok := errors.AsType[*routeros.Error](err)
		if ok && re.Status < 500 {
			return nil, err
		}
	}

	return nil, fmt.Errorf("after retries: %w", lastErr)
}

// Rows with show=false are console furniture; "<value>" marks that a
// freeform literal is accepted.
func shown(rows []map[string]string) (values []string, freeform bool) {
	for _, r := range rows {
		if r["completion"] == "<value>" {
			freeform = true
			continue
		}

		if r["show"] == "true" && r["completion"] != "" {
			values = append(values, r["completion"])
		}
	}

	slices.Sort(values)

	return values, freeform
}
