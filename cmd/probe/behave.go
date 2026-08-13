package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/sindrip/routeros"
	"github.com/sindrip/routeros/lab"
)

type behaviour struct {
	Path      string            `json:"path"`
	PutEmpty  string            `json:"put_empty"`
	Error     string            `json:"error,omitempty"`
	Created   map[string]string `json:"created,omitempty"`
	Generated []string          `json:"generated,omitempty"`
	Read      map[string]string `json:"read,omitempty"`
	ReadErr   string            `json:"read_error,omitempty"`
	Deleted   string            `json:"deleted,omitempty"`
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

			c := routeros.New(r.URL, r.User, r.Password)

			for j := i; j < len(menus); j += len(l.Routers) {
				ss := []sample{behaveOne(ctx, c, menus[j].Path), behaveOne(ctx, c, menus[j].Path)}
				shards[i] = append(shards[i], merge(menus[j].Path, ss))
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
