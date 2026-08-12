# Sketches

Pinned ideas, not decisions. Delete when built or refuted.

## Client `do[T]` (lands with List, M2)

Go 1.27 generic methods let the shared HTTP path be a method. `T` is the
caller's response shape, never a schema — the client stays untyped.

```go
// One HTTP path: build request, auth, send, status-check, decode.
func (c *Client) do[T any](ctx context.Context, method, path string, body io.Reader) (T, error)

// The verbs are one-liners over it.
func (c *Client) Get(ctx context.Context, path string) (map[string]string, error) {
	return c.do[map[string]string](ctx, http.MethodGet, path, nil)
}

func (c *Client) List(ctx context.Context, path string) ([]map[string]string, error) {
	return c.do[[]map[string]string](ctx, http.MethodGet, path, nil)
}
```
