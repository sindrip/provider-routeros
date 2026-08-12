// Package lab boots the disposable RouterOS router and tears it down.
package lab

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Lab is a running disposable router.
type Lab struct {
	URL      string
	User     string
	Password string
	dir      string
}

// Boot runs `docker compose up --wait` on the compose file beside this
// package and returns the router's REST endpoint.
func Boot(ctx context.Context) (*Lab, error) {
	dir := composeDir()
	if out, err := compose(ctx, dir, "up", "--wait"); err != nil {
		return nil, fmt.Errorf("compose up: %w\n%s", err, out)
	}
	return &Lab{URL: "http://127.0.0.1:18080", User: "admin", Password: "", dir: dir}, nil
}

// Down discards the router and its state.
func (l *Lab) Down(ctx context.Context) error {
	if out, err := compose(ctx, l.dir, "down"); err != nil {
		return fmt.Errorf("compose down: %w\n%s", err, out)
	}
	return nil
}

func compose(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", append([]string{"compose", "--project-directory", dir}, args...)...)
	return cmd.CombinedOutput()
}

// composeDir works from any working directory: the source path pins the
// compose file's location.
func composeDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(file)
}
