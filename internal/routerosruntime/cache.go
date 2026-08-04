package routerosruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	tf "github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

// ClientCache reuses configured upstream clients across reconciliations. Cache
// keys are hashes and are safe to include in diagnostics; raw credentials are
// never retained as map keys or logged.
type ClientCache struct {
	mu              sync.Mutex
	clients         map[string]*Meta
	providerFactory func() *schema.Provider
	closed          bool
}

// NewClientCache creates a cache and closes its clients when ctx ends.
func NewClientCache(ctx context.Context) *ClientCache {
	return newClientCache(ctx, routeros.Provider)
}

func newClientCache(ctx context.Context, providerFactory func() *schema.Provider) *ClientCache {
	c := &ClientCache{clients: map[string]*Meta{}, providerFactory: providerFactory}
	go func() {
		<-ctx.Done()
		_ = c.Close()
	}()
	return c
}

// Configure validates the supplied configuration with the official provider,
// creates its client once, and returns metadata suitable for guarded callbacks.
func (c *ClientCache) Configure(ctx context.Context, configuration map[string]any) (*Meta, error) {
	p := c.providerFactory()
	normalized, err := normalizeConfiguration(configuration, p.Schema)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("cannot encode RouterOS provider configuration: %w", err)
	}
	sum := sha256.Sum256(b)
	key := hex.EncodeToString(sum[:])

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, fmt.Errorf("RouterOS client cache is closed")
	}
	if cached := c.clients[key]; cached != nil {
		return cached, nil
	}

	operationMu.Lock()
	defer operationMu.Unlock()
	rc := tf.NewResourceConfigRaw(normalized)
	if diags := p.Validate(rc); diags.HasError() {
		return nil, diagnosticsError("invalid RouterOS provider configuration", diags)
	}
	if diags := p.Configure(context.WithoutCancel(ctx), rc); diags.HasError() {
		return nil, diagnosticsError("cannot configure RouterOS provider", diags)
	}
	client, ok := p.Meta().(routeros.Client)
	if !ok || client == nil {
		return nil, fmt.Errorf("official RouterOS provider returned unexpected client %T", p.Meta())
	}
	m := &Meta{Client: client, Version: routeros.RouterOSVersion}
	c.clients[key] = m
	return m, nil
}

// Close closes API connections and idle REST connections held by the cache.
func (c *ClientCache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true

	operationMu.Lock()
	defer operationMu.Unlock()
	var errs []string
	for _, m := range c.clients {
		switch client := m.Client.(type) {
		case *routeros.ApiClient:
			if err := client.Close(); err != nil {
				errs = append(errs, err.Error())
			}
		case *routeros.RestClient:
			client.CloseIdleConnections()
		}
	}
	c.clients = nil
	if len(errs) > 0 {
		return fmt.Errorf("cannot close RouterOS clients: %s", strings.Join(errs, "; "))
	}
	return nil
}

func normalizeConfiguration(configuration map[string]any, providerSchema map[string]*schema.Schema) (map[string]any, error) {
	result := make(map[string]any, len(configuration))
	for key, value := range configuration {
		s, ok := providerSchema[key]
		if !ok {
			result[key] = value
			continue
		}
		if value == nil {
			result[key] = nil
			continue
		}
		var err error
		switch s.Type { //nolint:exhaustive
		case schema.TypeString:
			result[key], err = toString(value)
		case schema.TypeBool:
			result[key], err = toBool(value)
		case schema.TypeInt:
			result[key], err = toInt(value)
		default:
			result[key] = value
		}
		if err != nil {
			return nil, fmt.Errorf("invalid RouterOS provider configuration field %q: %w", key, err)
		}
	}
	return result, nil
}

func toString(value any) (string, error) {
	if s, ok := value.(string); ok {
		return s, nil
	}
	return "", fmt.Errorf("expected string, got %T", value)
}

func toBool(value any) (bool, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		return strconv.ParseBool(v)
	default:
		return false, fmt.Errorf("expected boolean, got %T", value)
	}
}

func toInt(value any) (int, error) {
	switch v := value.(type) {
	case int:
		return v, nil
	case float64:
		if v != float64(int(v)) {
			return 0, fmt.Errorf("expected integer, got %v", v)
		}
		return int(v), nil
	case json.Number:
		n, err := strconv.Atoi(v.String())
		return n, err
	case string:
		return strconv.Atoi(v)
	default:
		return 0, fmt.Errorf("expected integer, got %T", value)
	}
}

func diagnosticsError(prefix string, diags diag.Diagnostics) error {
	parts := make([]string, 0, len(diags))
	for _, d := range diags {
		if d.Severity != diag.Error {
			continue
		}
		message := d.Summary
		if d.Detail != "" && d.Detail != d.Summary {
			message += ": " + d.Detail
		}
		parts = append(parts, message)
	}
	return fmt.Errorf("%s: %s", prefix, strings.Join(parts, "; "))
}
