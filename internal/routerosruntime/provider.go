package routerosruntime

import (
	"context"
	"fmt"
	"sync"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

// Meta carries the official provider client together with the RouterOS version
// that was active when that client was configured.
type Meta struct {
	Client  routeros.Client
	Version string
}

// The upstream provider uses RouterOSVersion as package-global state. A
// Terraform plugin process configures one client, but no-fork mode embeds many
// configured clients in one process. Serializing callbacks preserves the
// upstream single-client invariant without changing any resource behavior.
var operationMu sync.Mutex

// WrapProvider adds the execution guard required by the upstream package-global
// RouterOS version. The wrapped callbacks delegate directly to the originals.
func WrapProvider(p *schema.Provider) *schema.Provider {
	for _, r := range p.ResourcesMap {
		wrapResource(r)
	}
	return p
}

// Legacy callbacks are deprecated by the SDK but remain part of the upstream
// provider contract, so no-fork mode must guard them along with context APIs.
func wrapResource(r *schema.Resource) {
	if original := r.Create; original != nil {
		r.Create = func(d *schema.ResourceData, meta any) error {
			return withMetaError(meta, func(client routeros.Client) error { return original(d, client) })
		}
	}
	if original := r.Read; original != nil {
		r.Read = func(d *schema.ResourceData, meta any) error {
			return withMetaError(meta, func(client routeros.Client) error { return original(d, client) })
		}
	}
	if original := r.Update; original != nil {
		r.Update = func(d *schema.ResourceData, meta any) error {
			return withMetaError(meta, func(client routeros.Client) error { return original(d, client) })
		}
	}
	if original := r.Delete; original != nil {
		r.Delete = func(d *schema.ResourceData, meta any) error {
			return withMetaError(meta, func(client routeros.Client) error { return original(d, client) })
		}
	}
	if original := r.Exists; original != nil {
		r.Exists = func(d *schema.ResourceData, meta any) (bool, error) {
			var exists bool
			err := withMetaError(meta, func(client routeros.Client) error {
				var err error
				exists, err = original(d, client)
				return err
			})
			return exists, err
		}
	}

	r.CreateContext = wrapContextFunc(r.CreateContext)
	r.ReadContext = wrapContextFunc(r.ReadContext)
	r.UpdateContext = wrapContextFunc(r.UpdateContext)
	r.DeleteContext = wrapContextFunc(r.DeleteContext)
	r.CreateWithoutTimeout = wrapContextFunc(r.CreateWithoutTimeout)
	r.ReadWithoutTimeout = wrapContextFunc(r.ReadWithoutTimeout)
	r.UpdateWithoutTimeout = wrapContextFunc(r.UpdateWithoutTimeout)
	r.DeleteWithoutTimeout = wrapContextFunc(r.DeleteWithoutTimeout)

	if original := r.CustomizeDiff; original != nil {
		r.CustomizeDiff = func(ctx context.Context, d *schema.ResourceDiff, meta any) error {
			return withMetaError(meta, func(client routeros.Client) error { return original(ctx, d, client) })
		}
	}
}

func wrapContextFunc[T ~func(context.Context, *schema.ResourceData, interface{}) diag.Diagnostics](original T) T {
	if original == nil {
		return nil
	}
	return T(func(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
		m, err := acquireMeta(meta)
		if err != nil {
			return diag.FromErr(err)
		}
		defer operationMu.Unlock()
		return original(ctx, d, m.Client)
	})
}

func withMetaError(meta any, call func(routeros.Client) error) error {
	m, err := acquireMeta(meta)
	if err != nil {
		return err
	}
	defer operationMu.Unlock()
	return call(m.Client)
}

func acquireMeta(meta any) (*Meta, error) {
	m, ok := meta.(*Meta)
	if !ok || m == nil || m.Client == nil {
		return nil, fmt.Errorf("unexpected RouterOS provider metadata %T", meta)
	}
	operationMu.Lock()
	routeros.RouterOSVersion = m.Version
	return m, nil
}
