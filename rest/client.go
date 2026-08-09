package rest

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Decoding uses encoding/json/v2 at its defaults, which reject both duplicate
// object names and invalid UTF-8. Both rejections are deliberate.
//
// A reply naming a field twice is ambiguous, and this package's stance is that
// ambiguity surfaces rather than being silently resolved — v1 kept the last
// one without comment.
//
// Invalid UTF-8 is the closer call. Neither encoder can hand back the original
// bytes: v1 substitutes U+FFFD, and so does v2 under jsontext.AllowInvalidUTF8.
// Substitution is the worse failure here, because a comment is durable
// identity — a silently mangled one simply stops matching, and "not found" is
// how a caller mints a duplicate row. An error naming the malformed reply is
// actionable; a corrupted identity is not.

// defaultTimeout bounds a request when the caller supplies no http.Client.
//
// It sits above the router's own 60s cap on POST commands deliberately, so a
// command that runs long fails with the router's explanation rather than with
// a bare client-side timeout. http.DefaultClient is not used because it has no
// timeout at all, and an unbounded hang is a poor default for a library.
const defaultTimeout = 75 * time.Second

// Client talks to one router. It holds no package state, so many may exist in
// one process.
type Client struct {
	base string
	http *http.Client
	user string
	pass string
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient supplies the transport. Callers that already own one — a
// collector configured through confighttp, say — should pass it rather than
// let this package build a second policy for TLS, proxying and pooling.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.http = h
		}
	}
}

// WithBasicAuth sets the credentials sent on every request.
//
// The user needs both the "api" and "rest-api" policies. read,rest-api alone
// answers 500 "std failure: not allowed (9)" on every endpoint, because REST
// is a JSON wrapper over the binary API.
func WithBasicAuth(user, pass string) Option {
	return func(c *Client) { c.user, c.pass = user, pass }
}

// New returns a Client for a router's base URL, e.g. "https://10.0.10.1". A
// trailing "/rest" is accepted and not doubled.
func New(endpoint string, opts ...Option) (*Client, error) {
	base := strings.TrimSuffix(strings.TrimRight(endpoint, "/"), "/rest")
	if base == "" {
		return nil, errors.New("rest: empty endpoint")
	}
	c := &Client{base: base + "/rest", http: &http.Client{Timeout: defaultTimeout}}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// List returns the rows of a menu. Without options it is a plain GET; with
// them it becomes POST <path>/print carrying a JSON body, so filter values are
// never URL-encoded.
//
// A singleton menu answers with a bare object rather than an array; it is
// returned as a single-element slice. Use Get to read one directly.
func (c *Client) List(ctx context.Context, path string, opts ...QueryOpt) ([]Record, error) {
	q := newQuery(opts)
	if q.empty() {
		return c.do(ctx, http.MethodGet, menuPath(path), nil)
	}
	return c.do(ctx, http.MethodPost, menuPath(path)+"/print", q.body())
}

// Get reads a settings singleton.
func (c *Client) Get(ctx context.Context, path string) (Record, error) {
	recs, err := c.do(ctx, http.MethodGet, menuPath(path), nil)
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return nil, fmt.Errorf("GET %s: %w", menuPath(path), ErrNotFound)
	}
	return recs[0], nil
}

// Count returns how many rows match, without transferring them. The router
// answers {"ret":"0"} — note that ret is a string.
func (c *Client) Count(ctx context.Context, path string, opts ...QueryOpt) (int, error) {
	q := newQuery(opts)
	q.count = true
	q.props = nil // count-only and .proplist do not combine

	op := "POST " + menuPath(path) + "/print"
	body, err := c.roundTrip(ctx, http.MethodPost, menuPath(path)+"/print", q.body())
	if err != nil {
		return 0, err
	}
	var ret struct {
		Ret string `json:"ret"`
	}
	if err := json.Unmarshal(body, &ret); err != nil {
		return 0, fmt.Errorf("%s: decoding count: %w", op, err)
	}
	n, err := strconv.Atoi(ret.Ret)
	if err != nil {
		return 0, fmt.Errorf("%s: count %q is not a number: %w", op, ret.Ret, err)
	}
	return n, nil
}

// Create adds a row and returns it as stored.
//
// The verb is PUT. POST is the console-command verb: sent to a menu path it
// creates nothing and reports success anyway.
func (c *Client) Create(ctx context.Context, path string, r Record) (Record, error) {
	recs, err := c.do(ctx, http.MethodPut, menuPath(path), r)
	if err != nil {
		return nil, err
	}
	return first(recs), nil
}

// Update patches one row, addressed by .id.
func (c *Client) Update(ctx context.Context, path, id string, r Record) (Record, error) {
	recs, err := c.do(ctx, http.MethodPatch, menuPath(path)+"/"+id, r)
	if err != nil {
		return nil, err
	}
	return first(recs), nil
}

// Delete removes one row, addressed by .id.
func (c *Client) Delete(ctx context.Context, path, id string) error {
	_, err := c.do(ctx, http.MethodDelete, menuPath(path)+"/"+id, nil)
	return err
}

// Set writes a settings singleton. Singletons reject PATCH, so this is
// POST <path>/set.
func (c *Client) Set(ctx context.Context, path string, r Record) error {
	_, err := c.do(ctx, http.MethodPost, menuPath(path)+"/set", r)
	return err
}

// Command invokes a console command, e.g. Command(ctx, "/interface", "monitor",
// Record{"numbers": "ether1", "once": ""}).
//
// REST refuses continuous commands, so anything that streams — monitor above —
// needs "once". The router caps a POST at 60s, which the http.Client's own
// timeout should allow for.
func (c *Client) Command(ctx context.Context, path, cmd string, args Record) ([]Record, error) {
	if args == nil {
		args = Record{}
	}
	return c.do(ctx, http.MethodPost, menuPath(path)+"/"+cmd, args)
}

// do performs one exchange and decodes the reply into records.
func (c *Client) do(ctx context.Context, method, path string, body any) ([]Record, error) {
	raw, err := c.roundTrip(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	return decode(method+" "+path, raw)
}

// roundTrip performs one exchange and returns the raw body, having classified
// any failure.
func (c *Client) roundTrip(ctx context.Context, method, path string, body any) ([]byte, error) {
	op := method + " " + path

	var payload io.Reader
	if body != nil {
		enc, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("%s: encoding request: %w", op, err)
		}
		payload = bytes.NewReader(enc)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.base+path, payload)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	req.SetBasicAuth(c.user, c.pass)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, transportError(op, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, transportError(op, err)
	}
	// A 204 carries nothing; DELETE answers that way.
	if len(bytes.TrimSpace(raw)) == 0 {
		if resp.StatusCode >= http.StatusBadRequest {
			return nil, &Error{Op: op, Status: resp.StatusCode, Message: http.StatusText(resp.StatusCode)}
		}
		return nil, nil
	}
	if err := asError(op, resp.StatusCode, raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// decode turns a reply into records. A list menu answers with an array and a
// singleton with a bare object, so both are accepted.
func decode(op string, raw []byte) ([]Record, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if trimmed[0] == '[' {
		var many []Record
		if err := json.Unmarshal(trimmed, &many); err != nil {
			return nil, fmt.Errorf("%s: decoding rows: %w", op, err)
		}
		return many, nil
	}
	var one Record
	if err := json.Unmarshal(trimmed, &one); err != nil {
		return nil, fmt.Errorf("%s: decoding record: %w", op, err)
	}
	return []Record{one}, nil
}

func first(recs []Record) Record {
	if len(recs) == 0 {
		return nil
	}
	return recs[0]
}

// menuPath normalises a menu to a leading slash and no trailing one, so
// callers may pass "ip/address" or "/ip/address".
func menuPath(p string) string {
	p = strings.TrimRight(p, "/")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}
