// Package lab boots the disposable RouterOS routers and tears them down.
package lab

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
)

const routers = 2

type Router struct {
	URL      string
	User     string
	Password string
}

type Lab struct {
	Routers []Router
	dir     string
}

func Boot(ctx context.Context) (*Lab, error) {
	dir := composeDir()
	if out, err := compose(ctx, dir, "up", "--wait", "--remove-orphans"); err != nil {
		// A failed up leaves half-created containers that poison the retry.
		_, _ = compose(context.WithoutCancel(ctx), dir, "down")

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

// The compose file lives beside this package's source.
func composeDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(file)
}
