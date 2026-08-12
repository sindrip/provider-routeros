// Package routeros is a REST client for MikroTik RouterOS. Paths are
// console paths ("/system/resource"); values are strings, as the device
// returns them.
package routeros

import (
	"context"
	"encoding/json"
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/rest"+path, nil)
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(c.user, c.password)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, restError(path, resp.StatusCode, body)
	}

	var rec map[string]string
	if err := json.Unmarshal(body, &rec); err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}

	return rec, nil
}

func restError(path string, code int, body []byte) error {
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

	return fmt.Errorf("GET %s: %d %s", path, code, msg)
}
