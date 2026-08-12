// Package routeros is a REST client for MikroTik RouterOS. Paths are
// console paths ("/system/resource"); values are strings, as the device
// returns them.
package routeros

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	base     string
	user     string
	password string
	httpc    *http.Client
}

func New(base, user, password string) *Client {
	return &Client{
		base:     strings.TrimRight(base, "/"),
		user:     user,
		password: password,
		httpc:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Get fetches a menu's single record.
func (c *Client) Get(ctx context.Context, path string) (map[string]string, error) {
	return c.do[map[string]string](ctx, http.MethodGet, path, nil)
}

// List fetches a menu's rows.
func (c *Client) List(ctx context.Context, path string) ([]map[string]string, error) {
	return c.do[[]map[string]string](ctx, http.MethodGet, path, nil)
}

// Add creates one row and returns it as the device expanded it.
func (c *Client) Add(ctx context.Context, path string, fields map[string]string) (map[string]string, error) {
	return c.do[map[string]string](ctx, http.MethodPut, path, fields)
}

// Set updates the stated fields of one row or singleton record and
// returns the result.
func (c *Client) Set(ctx context.Context, path, id string, fields map[string]string) (map[string]string, error) {
	return c.do[map[string]string](ctx, http.MethodPatch, path+"/"+id, fields)
}

// Remove deletes one row. The device answers 204 with no body.
func (c *Client) Remove(ctx context.Context, path, id string) error {
	status, body, err := c.Raw(ctx, http.MethodDelete, path+"/"+id, nil)
	if err != nil {
		return err
	}

	if status != http.StatusNoContent && status != http.StatusOK {
		return restError(http.MethodDelete, path+"/"+id, status, body)
	}

	return nil
}

// Exec runs a console command with the given arguments and returns the
// records it emits.
func (c *Client) Exec(ctx context.Context, path string, args map[string]string) ([]map[string]string, error) {
	return c.do[[]map[string]string](ctx, http.MethodPost, path, args)
}

// Raw performs one request and returns the raw status and body — the
// escape hatch probes use to observe response shape.
func (c *Client) Raw(ctx context.Context, method, path string, args map[string]string) (int, []byte, error) {
	var body io.Reader

	if args != nil {
		b, err := json.Marshal(args)
		if err != nil {
			return 0, nil, err
		}

		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.base+"/rest"+path, body)
	if err != nil {
		return 0, nil, err
	}

	req.SetBasicAuth(c.user, c.password)

	if args != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpc.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("%s %s: %w", method, path, err)
	}

	return resp.StatusCode, b, nil
}

// do is the one HTTP path: request, auth, status check, decode. T is the
// caller's response shape — a record or a list of them — never a schema.
func (c *Client) do[T any](ctx context.Context, method, path string, args map[string]string) (T, error) {
	var zero T

	status, body, err := c.Raw(ctx, method, path, args)
	if err != nil {
		return zero, err
	}

	if status != http.StatusOK {
		return zero, restError(method, path, status, body)
	}

	var v T
	if err := json.Unmarshal(body, &v); err != nil {
		return zero, fmt.Errorf("%s %s: %w", method, path, err)
	}

	return v, nil
}

func restError(method, path string, code int, body []byte) error {
	var e struct {
		Message string `json:"message"`
		Detail  string `json:"detail"`
	}

	_ = json.Unmarshal(body, &e)

	msg := e.Message
	if e.Detail != "" {
		msg += ": " + e.Detail
	}

	if msg == "" {
		msg = strings.TrimSpace(string(body))
	}

	return fmt.Errorf("%s %s: %d %s", method, path, code, msg)
}
