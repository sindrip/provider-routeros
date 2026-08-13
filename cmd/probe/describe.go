package main

import (
	"cmp"
	"context"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/sindrip/routeros"
)

// Console bookkeeping that enumeration lists alongside real properties.
var notProperties = map[string]bool{
	".dead": true, ".id": true, ".nextid": true, "about": true,
	"numbers": true, "value-name": true, "=": true,
}

type menuDesc struct {
	Path       string              `json:"path"`
	NodeType   string              `json:"node_type"`
	Commands   []string            `json:"commands,omitempty"`
	Args       map[string][]string `json:"args,omitempty"`
	Get        string              `json:"get,omitempty"`
	GetError   string              `json:"get_error,omitempty"`
	Class      string              `json:"class"`
	Properties []*property         `json:"properties,omitempty"`
	Error      string              `json:"error,omitempty"`
}

type property struct {
	Arg       string         `json:"arg"`
	Command   string         `json:"command,omitempty"`
	Access    string         `json:"access"`
	Kind      string         `json:"kind"`
	Values    []string       `json:"values,omitempty"`
	Negatable bool           `json:"negatable,omitempty"`
	Syntax    jsontext.Value `json:"syntax,omitempty"`
	Error     string         `json:"error,omitempty"`

	input string
}

const maxDepth = 16

// describe asks the router to describe itself: one walk pins every
// path's verdict, commands, and args; a second phase asks completion
// for each property's value space (enum / open-enum / scalar) and
// syntax for its grammar.
func describe(ctx context.Context, c *routeros.Client) ([]*menuDesc, error) {
	var out []*menuDesc

	if err := walk(ctx, c, nil, "path", &out, 0); err != nil {
		return nil, err
	}

	for _, m := range out {
		if m.Class == "rows" || m.Class == "singleton" {
			m.Properties = properties(ctx, c, m)
		}
	}

	probeProperties(ctx, c, out)
	slices.SortFunc(out, func(a, b *menuDesc) int { return cmp.Compare(a.Path, b.Path) })

	return out, nil
}

func walk(ctx context.Context, c *routeros.Client, segs []string, nodeType string, out *[]*menuDesc, depth int) error {
	if depth >= maxDepth {
		return fmt.Errorf("/%s: max depth exceeded", strings.Join(segs, "/"))
	}

	kids, err := children(ctx, c, segs)

	if len(segs) == 0 {
		if err != nil {
			return err
		}
	} else {
		m := &menuDesc{
			Path:     "/" + strings.Join(segs, "/"),
			NodeType: nodeType,
		}

		if err != nil {
			m.Error = err.Error()
			m.Class = "unknown"
			*out = append(*out, m)

			return nil //nolint:nilerr // recorded as the path's verdict
		}

		for _, k := range kids {
			if k.nodeType == "cmd" {
				m.Commands = append(m.Commands, k.name)
			}
		}

		slices.Sort(m.Commands)

		m.Get, m.GetError, m.Class = classify(ctx, c, m.Path, m.Commands)

		if m.Class == "rows" || m.Class == "singleton" {
			m.Args = map[string][]string{}

			for _, command := range m.Commands {
				argKids, err := children(ctx, c, append(segs, command))
				if err != nil {
					m.Error = err.Error()
					continue
				}

				args := []string{}

				for _, k := range argKids {
					if k.nodeType == "arg" {
						args = append(args, k.name)
					}
				}

				slices.Sort(args)
				m.Args[command] = args
			}
		}

		*out = append(*out, m)
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

// properties collapses a menu's args into one probe per property:
// `set` wins over `add` for writables; everything else the menu
// enumerates is read-only. Read-only properties of get-enumerated menus
// have no console cursor position, so they stay unknown by construction.
func properties(ctx context.Context, c *routeros.Client, m *menuDesc) []*property {
	writable := map[string]string{}

	for _, command := range []string{"add", "set"} {
		for _, arg := range m.Args[command] {
			if !notProperties[arg] {
				writable[arg] = command
			}
		}
	}

	var out []*property

	for arg, command := range writable {
		out = append(out, &property{
			Arg: arg, Command: command, Access: "writable",
			input: m.Path + "/" + command + " " + arg + "=",
		})
	}

	enumerated, err := enumerate(ctx, c, m)
	if err != nil {
		out = append(out, &property{Access: "read-only", Kind: "unknown", Error: err.Error()})
	} else {
		for _, arg := range enumerated {
			if notProperties[arg] || writable[arg] != "" {
				continue
			}

			p := &property{Arg: arg, Access: "read-only"}

			if m.Class == "rows" {
				p.input = m.Path + "/print where " + arg + "="
			}

			out = append(out, p)
		}
	}

	slices.SortFunc(out, func(a, b *property) int { return cmp.Compare(a.Arg, b.Arg) })

	return out
}

// enumerate lists every property a menu exposes: `print where` completes
// to them on rows menus; `get` does on singletons, mixed with get's own
// arguments, which are subtracted via the menu's args.
func enumerate(ctx context.Context, c *routeros.Client, m *menuDesc) ([]string, error) {
	if m.Class == "rows" {
		values, _, _, err := completion(ctx, c, m.Path+"/print where ")
		return values, err
	}

	values, _, _, err := completion(ctx, c, m.Path+"/get ")
	if err != nil {
		return nil, err
	}

	own := slices.Concat(m.Args["get"], m.Args["print"])

	return slices.DeleteFunc(values, func(v string) bool { return slices.Contains(own, v) }), nil
}

func probeProperties(ctx context.Context, c *routeros.Client, menus []*menuDesc) {
	var wg sync.WaitGroup

	sem := make(chan struct{}, 32)

	for _, m := range menus {
		for _, p := range m.Properties {
			if p.input == "" {
				if p.Kind == "" {
					p.Kind = "unknown"
				}

				continue
			}

			wg.Add(1)

			sem <- struct{}{}

			go func() {
				defer wg.Done()
				defer func() { <-sem }()

				probeOne(ctx, c, p)
			}()
		}
	}

	wg.Wait()
}

func probeOne(ctx context.Context, c *routeros.Client, p *property) {
	values, freeform, negatable, err := completion(ctx, c, p.input)
	if err != nil {
		p.Kind = "unknown"
		p.Error = err.Error()

		return
	}

	p.Values = values
	p.Negatable = negatable

	switch {
	case len(values) > 0 && !freeform:
		p.Kind = "enum"
	case len(values) > 0:
		p.Kind = "open-enum"
	case freeform:
		p.Kind = "scalar"
	default:
		p.Kind = "unknown"
		p.Error = "empty completion"

		return
	}

	// Always via input=; bare scripting keywords via path= deadlocked
	// inspect on <=7.20.8 (restraml SUP-127641), which we never emit.
	syntax, err := c.Post[jsontext.Value](ctx, "/console/inspect", map[string]string{"request": "syntax", "input": p.input})
	if err != nil {
		p.Error = err.Error()
		return
	}

	p.Syntax = syntax
}

func completion(ctx context.Context, c *routeros.Client, input string) (values []string, freeform, negatable bool, err error) {
	rows, err := inspect(ctx, c, "completion", input)
	if err != nil {
		return nil, false, false, err
	}

	for _, r := range rows {
		if r["completion"] == "<value>" {
			freeform = true
			continue
		}

		// style=syntax-meta rows are grammar tokens ("!" = negatable),
		// not vocabulary; show=false rows are console furniture.
		if r["style"] == "syntax-meta" {
			if r["completion"] == "!" {
				negatable = true
			}

			continue
		}

		if r["show"] == "true" && r["completion"] != "" {
			values = append(values, r["completion"])
		}
	}

	slices.Sort(values)

	return values, freeform, negatable, nil
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
