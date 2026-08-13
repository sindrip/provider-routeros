// Probe boots the disposable router, pins observations, and tears it down.
package main

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/sindrip/routeros"
	"github.com/sindrip/routeros/lab"
)

// The menu tree differs across architectures and boards; version alone lies.
type stamp struct {
	Probe        string `json:"probe"`
	RouterOS     string `json:"routeros"`
	Architecture string `json:"architecture"`
	Board        string `json:"board"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "probe:", err)
		os.Exit(1)
	}
}

func run() (err error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	l, err := lab.Boot(ctx)
	if err != nil {
		return err
	}

	defer func() {
		if derr := l.Down(context.WithoutCancel(ctx)); err == nil {
			err = derr
		}
	}()

	r := l.Routers[0]
	c := routeros.New(r.URL, r.User, r.Password)

	st, err := identity(ctx, c)
	if err != nil {
		return err
	}

	if err := write("version", stamp{Probe: "version", RouterOS: st.RouterOS, Architecture: st.Architecture, Board: st.Board}); err != nil {
		return err
	}

	paths, err := inventory(ctx, c)
	if err != nil {
		return err
	}

	return write("inventory", struct {
		stamp
		Paths []pathInfo `json:"paths"`
	}{stamp{Probe: "inventory", RouterOS: st.RouterOS, Architecture: st.Architecture, Board: st.Board}, paths})
}

func identity(ctx context.Context, c *routeros.Client) (stamp, error) {
	rec, err := c.Get[map[string]string](ctx, "/system/resource")
	if err != nil {
		return stamp{}, err
	}

	if rec["version"] == "" {
		return stamp{}, fmt.Errorf("/system/resource has no version: %v", rec)
	}

	return stamp{
		RouterOS:     rec["version"],
		Architecture: rec["architecture-name"],
		Board:        rec["board-name"],
	}, nil
}

// No timestamps, deterministic map order: same router, byte-identical file.
func write(name string, v any) error {
	b, err := json.Marshal(v, json.Deterministic(true), jsontext.WithIndent("  "))
	if err != nil {
		return err
	}

	if err := os.MkdirAll("observations", 0o755); err != nil {
		return err
	}

	return os.WriteFile(filepath.Join("observations", name+".json"), append(b, '\n'), 0o644)
}
