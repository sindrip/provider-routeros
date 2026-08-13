package main

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"sync"

	"github.com/sindrip/routeros"
	"github.com/sindrip/routeros/lab"
)

type expansion struct {
	Path     string            `json:"path"`
	PutEmpty string            `json:"put_empty"`
	Error    string            `json:"error,omitempty"`
	Created  map[string]string `json:"created,omitempty"`
	Read     map[string]string `json:"read,omitempty"`
	ReadErr  string            `json:"read_error,omitempty"`
	Deleted  string            `json:"deleted,omitempty"`
}

// behave PUTs an empty body into every add-capable rows menu: an error
// pins the menu's required fields as the device states them; a created
// row pins its full default expansion, read back and then deleted.
func behave(ctx context.Context, l *lab.Lab, all []*menuDesc) []expansion {
	var menus []*menuDesc

	for _, m := range all {
		if m.Class == "rows" && slices.Contains(m.Commands, "add") {
			menus = append(menus, m)
		}
	}

	shards := make([][]expansion, len(l.Routers))

	var wg sync.WaitGroup

	for i, r := range l.Routers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			c := routeros.New(r.URL, r.User, r.Password)

			for j := i; j < len(menus); j += len(l.Routers) {
				shards[i] = append(shards[i], behaveOne(ctx, c, menus[j].Path))
			}
		}()
	}

	wg.Wait()

	out := slices.Concat(shards...)
	slices.SortFunc(out, func(a, b expansion) int { return cmp.Compare(a.Path, b.Path) })

	return out
}

func behaveOne(ctx context.Context, c *routeros.Client, path string) expansion {
	e := expansion{Path: path}

	created, err := c.Put[map[string]string](ctx, path, map[string]string{})
	if err != nil {
		e.PutEmpty = "error"
		e.Error = errText(err)

		return e
	}

	e.PutEmpty = "created"
	e.Created = created

	id := created[".id"]
	if id == "" {
		e.ReadErr = "created without .id"

		return e
	}

	read, err := c.Get[map[string]string](ctx, path+"/"+id)
	if err != nil {
		e.ReadErr = errText(err)
	} else {
		e.Read = read
	}

	if err := c.Delete(ctx, path+"/"+id); err != nil {
		e.Deleted = errText(err)
	} else {
		e.Deleted = "ok"
	}

	return e
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
