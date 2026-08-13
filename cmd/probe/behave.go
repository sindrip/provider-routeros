package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/sindrip/routeros"
	"github.com/sindrip/routeros/lab"
)

type behaviour struct {
	Path      string            `json:"path"`
	Wedged    bool              `json:"wedged,omitempty"`
	PutEmpty  string            `json:"put_empty"`
	Error     string            `json:"error,omitempty"`
	Created   map[string]string `json:"created,omitempty"`
	Generated []string          `json:"generated,omitempty"`
	Read      map[string]string `json:"read,omitempty"`
	ReadErr   string            `json:"read_error,omitempty"`
	Deleted   string            `json:"deleted,omitempty"`
	Required  []string          `json:"required,omitempty"`
	Minimal   map[string]string `json:"minimal,omitempty"`
	Duplicate *duplicate        `json:"duplicate,omitempty"`
	Ordering  *ordering         `json:"ordering,omitempty"`
}

type duplicate struct {
	Verdict string `json:"verdict"`
	Error   string `json:"error,omitempty"`
}

type ordering struct {
	Insert string `json:"insert,omitempty"`
	Move   string `json:"move,omitempty"`
	Order  string `json:"order,omitempty"`
	Error  string `json:"error,omitempty"`
}

type sample struct {
	putEmpty string
	err      string
	created  map[string]string
	read     map[string]string
	readErr  string
	deleted  string
}

// behave PUTs an empty body into every add-capable rows menu: an error
// pins the menu's required fields as the device states them; a created
// row pins its default expansion, read back and then deleted. Each menu
// is created twice — a field whose value differs between the two draws
// is observed to be device-generated. Cross-router diffing misses this:
// the RNG is time-seeded, and routers booted together roll the same
// dice (probed 2026-08-14, identical bridge MACs on both routers).
func behave(ctx context.Context, l *lab.Lab, all []*menuDesc) []behaviour {
	var menus []*menuDesc

	for _, m := range all {
		if m.Class == "rows" && slices.Contains(m.Commands, "add") {
			menus = append(menus, m)
		}
	}

	shards := make([][]behaviour, len(l.Routers))

	var wg sync.WaitGroup

	for i, r := range l.Routers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			c := client(r)

			done := 0

			for j := i; j < len(menus); j += len(l.Routers) {
				m := menus[j]

				menuCtx, cancel := context.WithTimeout(ctx, time.Minute)
				ss := []sample{behaveOne(menuCtx, c, m.Path), behaveOne(menuCtx, c, m.Path)}
				b := merge(m.Path, ss)
				extend(menuCtx, c, m, &b)
				cancel()

				if !healthy(ctx, c) {
					b.Wedged = true

					fmt.Fprintf(os.Stderr, "behave: r%d wedged by %s, recycling\n", i+1, m.Path)

					if err := l.Recycle(ctx, i); err != nil {
						fmt.Fprintf(os.Stderr, "behave: r%d lost: %v\n", i+1, err)
						shards[i] = append(shards[i], b)

						return
					}
				}

				shards[i] = append(shards[i], b)

				if done++; done%25 == 0 {
					fmt.Fprintf(os.Stderr, "behave: r%d %d menus\n", i+1, done)
				}
			}
		}()
	}

	wg.Wait()

	out := slices.Concat(shards...)
	slices.SortFunc(out, func(a, b behaviour) int { return cmp.Compare(a.Path, b.Path) })

	return out
}

func behaveOne(ctx context.Context, c *routeros.Client, path string) sample {
	s := sample{}

	created, err := c.Put[map[string]string](ctx, path, map[string]string{})
	if err != nil {
		s.putEmpty = "error"
		s.err = errText(err)

		return s
	}

	s.putEmpty = "created"
	s.created = created

	id := created[".id"]
	if id == "" {
		s.readErr = "created without .id"

		return s
	}

	read, err := c.Get[map[string]string](ctx, path+"/"+id)
	if err != nil {
		s.readErr = errText(err)
	} else {
		s.read = read
	}

	if err := c.Delete(ctx, path+"/"+id); err != nil {
		s.deleted = errText(err)
	} else {
		s.deleted = "ok"
	}

	return s
}

// merge folds one menu's samples: values identical in every draw are
// pinned verbatim; fields that differ are device-generated.
func merge(path string, ss []sample) behaviour {
	b := behaviour{Path: path}
	generated := map[string]bool{}

	b.PutEmpty = agree(ss, func(s sample) string { return s.putEmpty })
	b.Error = agree(ss, func(s sample) string { return s.err })
	b.ReadErr = agree(ss, func(s sample) string { return s.readErr })
	b.Deleted = agree(ss, func(s sample) string { return s.deleted })
	b.Created = stable(ss, func(s sample) map[string]string { return s.created }, generated)
	b.Read = stable(ss, func(s sample) map[string]string { return s.read }, generated)
	b.Generated = slices.Sorted(maps.Keys(generated))

	return b
}

func agree(ss []sample, field func(sample) string) string {
	first := field(ss[0])

	for _, s := range ss[1:] {
		if field(s) != first {
			var vals []string
			for _, s := range ss {
				vals = append(vals, field(s))
			}

			return fmt.Sprintf("disagree(%s)", strings.Join(vals, " | "))
		}
	}

	return first
}

func stable(ss []sample, field func(sample) map[string]string, generated map[string]bool) map[string]string {
	keys := map[string]bool{}

	for _, s := range ss {
		for k := range field(s) {
			keys[k] = true
		}
	}

	if len(keys) == 0 {
		return nil
	}

	out := map[string]string{}

	for k := range keys {
		first, ok := field(ss[0])[k]
		same := ok

		for _, s := range ss[1:] {
			v, ok := field(s)[k]
			if !ok || v != first {
				same = false
			}
		}

		if same {
			out[k] = first
		} else {
			generated[k] = true
		}
	}

	if len(out) == 0 {
		return nil
	}

	return out
}

func errText(err error) string {
	if re, ok := errors.AsType[*routeros.Error](err); ok {
		msg := re.Message
		if re.Detail != "" {
			msg += ": " + re.Detail
		}

		return msg
	}

	return err.Error()
}

var missingRe = regexp.MustCompile(`missing =([^=]+)=`)

// extend resolves a minimal creatable row by iterating on the device's
// missing-field errors, then asks the two remaining behaviour questions:
// is a duplicate of that row rejected (the rejection names the enforced
// field), and where do new rows land and move.
func extend(ctx context.Context, c *routeros.Client, m *menuDesc, b *behaviour) {
	fields := map[string]string{}

	for range 8 {
		created, err := c.Put[map[string]string](ctx, m.Path, fields)
		if err == nil {
			b.Minimal = fields

			defer remove(ctx, c, m.Path, created[".id"])

			b.Duplicate = duplicateOf(ctx, c, m, fields)

			if slices.Contains(m.Commands, "move") {
				b.Ordering = orderingOf(ctx, c, m, fields)
			}

			return
		}

		miss := missingRe.FindStringSubmatch(errText(err))
		if miss == nil {
			return
		}

		b.Required = append(b.Required, miss[1])

		v, ok := synth(m, miss[1], len(fields))
		if !ok {
			return
		}

		fields[miss[1]] = v
	}
}

func duplicateOf(ctx context.Context, c *routeros.Client, m *menuDesc, fields map[string]string) *duplicate {
	row, err := c.Put[map[string]string](ctx, m.Path, fields)
	if err != nil {
		return &duplicate{Verdict: "rejected", Error: errText(err)}
	}

	remove(ctx, c, m.Path, row[".id"])

	return &duplicate{Verdict: "accepted"}
}

func orderingOf(ctx context.Context, c *routeros.Client, m *menuDesc, fields map[string]string) *ordering {
	o := &ordering{}

	var ids []string

	defer func() {
		for _, id := range ids {
			remove(ctx, c, m.Path, id)
		}
	}()

	before, err := c.Get[[]map[string]string](ctx, m.Path)
	if err != nil {
		o.Error = errText(err)
		return o
	}

	for i := range 3 {
		varied := maps.Clone(fields)

		for k := range varied {
			if v, ok := synth(m, k, 100+i); ok {
				varied[k] = v
			}
		}

		row, err := c.Put[map[string]string](ctx, m.Path, varied)
		if err != nil {
			o.Error = errText(err)
			return o
		}

		ids = append(ids, row[".id"])
	}

	order, err := rowOrder(ctx, c, m.Path, len(before), ids)
	if err != nil {
		o.Error = errText(err)
		return o
	}

	switch order {
	case "1,2,3":
		o.Insert = "append"
	case "3,2,1":
		o.Insert = "prepend"
	default:
		o.Insert = "other: " + order
	}

	if _, err := c.Post[[]map[string]string](ctx, m.Path+"/move", map[string]string{"numbers": ids[2], "destination": ids[0]}); err != nil {
		o.Move = errText(err)
		return o
	}

	o.Move = "ok"

	if order, err = rowOrder(ctx, c, m.Path, len(before), ids); err == nil {
		o.Order = order
	} else {
		o.Error = errText(err)
	}

	return o
}

// rowOrder reports our created rows' positions after skipping the menu's
// pre-existing rows, as creation indexes in device order.
func rowOrder(ctx context.Context, c *routeros.Client, path string, skip int, ids []string) (string, error) {
	rows, err := c.Get[[]map[string]string](ctx, path)
	if err != nil {
		return "", err
	}

	var order []string

	for _, r := range rows[min(skip, len(rows)):] {
		if i := slices.Index(ids, r[".id"]); i >= 0 {
			order = append(order, fmt.Sprintf("%d", i+1))
		}
	}

	return strings.Join(order, ","), nil
}

// synth invents a value for a field from its described value space; i
// keeps repeated inventions distinct where the space allows it.
func synth(m *menuDesc, field string, i int) (string, bool) {
	var p *property

	for _, cand := range m.Properties {
		if cand.Arg == field {
			p = cand
			break
		}
	}

	if p == nil {
		return "", false
	}

	if len(p.Values) > 0 {
		return p.Values[min(i, len(p.Values)-1)], true
	}

	text := string(p.Syntax)

	switch {
	case strings.Contains(text, "(IP address)"), strings.Contains(text, "IP prefix"):
		return fmt.Sprintf("10.99.99.%d", i+1), true
	case strings.Contains(text, "IPv6"):
		return fmt.Sprintf("fd99::%d", i+1), true
	case strings.Contains(text, "(MAC address)"):
		return fmt.Sprintf("02:99:00:00:00:%02X", i+1), true
	case strings.Contains(text, "(time interval)"):
		return "1m", true
	case strings.Contains(text, "(integer number)"):
		return "1", true
	default:
		return fmt.Sprintf("probe%d", i), true
	}
}

func remove(ctx context.Context, c *routeros.Client, path, id string) {
	if id != "" {
		_ = c.Delete(ctx, path+"/"+id)
	}
}

func healthy(ctx context.Context, c *routeros.Client) bool {
	hctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := c.Get[map[string]string](hctx, "/system/resource")

	return err == nil
}
