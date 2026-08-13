// Package routeros is a REST client for MikroTik RouterOS. Paths are the
// device's REST paths ("/system/resource", "/ip/firewall/filter/*2");
// values are strings, as the device returns them.
package routeros

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type response interface {
	map[string]string | []map[string]string | jsontext.Value
}

type Error struct {
	Method  string
	Path    string
	Status  int
	Message string
	Detail  string
}

func (e *Error) Error() string {
	msg := e.Message
	if e.Detail != "" {
		msg += ": " + e.Detail
	}

	return fmt.Sprintf("%s %s: %d %s", e.Method, e.Path, e.Status, msg)
}

type Client struct {
	base     string
	user     string
	password string
	httpc    *http.Client
}

type Option func(*Client)

func WithHTTPClient(httpc *http.Client) Option {
	return func(c *Client) { c.httpc = httpc }
}

func New(base, user, password string, opts ...Option) *Client {
	c := &Client{
		base:     strings.TrimRight(base, "/"),
		user:     user,
		password: password,
		httpc:    &http.Client{Timeout: 30 * time.Second},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

func (c *Client) Get[T response](ctx context.Context, path string) (T, error) {
	return c.do[T](ctx, http.MethodGet, path, nil)
}

func (c *Client) Put[T response](ctx context.Context, path string, body map[string]string) (T, error) {
	return c.do[T](ctx, http.MethodPut, path, body)
}

func (c *Client) Patch[T response](ctx context.Context, path string, body map[string]string) (T, error) {
	return c.do[T](ctx, http.MethodPatch, path, body)
}

func (c *Client) Post[T response](ctx context.Context, path string, args map[string]string) (T, error) {
	return c.do[T](ctx, http.MethodPost, path, args)
}

func (c *Client) Delete(ctx context.Context, path string) error {
	_, err := c.do[jsontext.Value](ctx, http.MethodDelete, path, nil)
	return err
}

func (c *Client) do[T response](ctx context.Context, method, path string, body map[string]string) (T, error) {
	var zero T

	status, b, err := c.request(ctx, method, path, body)
	if err != nil {
		return zero, err
	}

	if status < 200 || status >= 300 {
		return zero, newError(method, path, status, b)
	}

	if len(b) == 0 {
		return zero, nil
	}

	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return zero, fmt.Errorf("%s %s: %w", method, path, err)
	}

	return v, nil
}

func (c *Client) request(ctx context.Context, method, path string, body map[string]string) (int, []byte, error) {
	var rdr io.Reader

	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}

		rdr = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.base+"/rest"+path, rdr)
	if err != nil {
		return 0, nil, err
	}

	req.SetBasicAuth(c.user, c.password)

	if body != nil {
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

func newError(method, path string, status int, body []byte) *Error {
	var payload struct {
		Message string `json:"message"`
		Detail  string `json:"detail"`
	}

	_ = json.Unmarshal(body, &payload)

	if payload.Message == "" {
		payload.Message = strings.TrimSpace(string(body))
	}

	return &Error{Method: method, Path: path, Status: status, Message: payload.Message, Detail: payload.Detail}
}
