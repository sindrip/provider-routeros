// Package lab boots the disposable RouterOS routers and tears them down.
package lab

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
)

// routers is the number of pinned ordinals in compose.yaml.
const routers = 2

// Router is one running disposable router.
type Router struct {
	URL      string
	User     string
	Password string
}

// Lab is the running set of disposable routers.
type Lab struct {
	Routers []Router
	dir     string
}

// Boot runs `docker compose up --wait` on the compose file beside this
// package and returns the routers' REST endpoints. Ordinal n listens on
// 127.0.0.1:801n.
func Boot(ctx context.Context) (*Lab, error) {
	dir := composeDir()
	if out, err := compose(ctx, dir, "up", "--wait", "--remove-orphans"); err != nil {
		return nil, fmt.Errorf("compose up: %w\n%s", err, out)
	}

	l := &Lab{dir: dir}

	for n := 1; n <= routers; n++ {
		l.Routers = append(l.Routers, Router{
			URL:  fmt.Sprintf("http://127.0.0.1:801%d", n),
			User: "admin",
		})
	}

	return l, nil
}

// Down discards the routers and their state.
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
