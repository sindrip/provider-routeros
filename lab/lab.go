// Package lab boots the disposable RouterOS routers and tears them down.
package lab

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
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

// Recycle recreates one router and waits for its REST endpoint to
// answer again. Recreate, not restart: the VM disk lives in the
// container filesystem, so a restart boots the dirty config.
func (l *Lab) Recycle(ctx context.Context, ordinal int) error {
	if out, err := compose(ctx, l.dir, "up", "-d", "--force-recreate", fmt.Sprintf("r%d", ordinal+1)); err != nil {
		return fmt.Errorf("compose recreate: %w\n%s", err, out)
	}

	r := l.Routers[ordinal]
	deadline := time.Now().Add(4 * time.Minute)

	for time.Now().Before(deadline) {
		reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		req, _ := http.NewRequestWithContext(reqCtx, http.MethodGet, r.URL+"/rest/system/resource", nil)
		req.SetBasicAuth(r.User, r.Password)

		resp, err := http.DefaultClient.Do(req)

		cancel()

		if err == nil {
			resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}

	return fmt.Errorf("r%d: not healthy after recycle", ordinal+1)
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
