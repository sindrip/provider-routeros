// Probe boots the disposable router, pins observations, and tears it down.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/sindrip/routeros"
	"github.com/sindrip/routeros/lab"
)

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
	// Teardown must survive ^C.
	defer func() {
		if derr := l.Down(context.WithoutCancel(ctx)); err == nil {
			err = derr
		}
	}()

	c := routeros.New(l.URL, l.User, l.Password)

	rec, err := c.Get(ctx, "/system/resource")
	if err != nil {
		return err
	}

	if rec["version"] == "" {
		return fmt.Errorf("/system/resource has no version: %v", rec)
	}

	return write("version", rec["version"])
}

// write pins one observation, stamped with the router's version. No
// timestamps: same router, byte-identical file.
func write(probe, version string) error {
	b, err := json.MarshalIndent(struct {
		Probe    string `json:"probe"`
		RouterOS string `json:"routeros"`
	}{probe, version}, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll("observations", 0o755); err != nil {
		return err
	}

	return os.WriteFile(filepath.Join("observations", probe+".json"), append(b, '\n'), 0o644)
}
