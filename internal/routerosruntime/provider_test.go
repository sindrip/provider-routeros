package routerosruntime

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	tf "github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

func TestWrapProviderRestoresClientVersion(t *testing.T) {
	var calls atomic.Int32
	r := &schema.Resource{
		Schema: map[string]*schema.Schema{"id": {Type: schema.TypeString, Computed: true}},
		ReadContext: func(_ context.Context, _ *schema.ResourceData, meta any) diag.Diagnostics {
			if _, ok := meta.(*routeros.RestClient); !ok {
				return diag.Errorf("callback received metadata %T", meta)
			}
			if routeros.RouterOSVersion != "7.20.1" {
				return diag.Errorf("callback observed RouterOS version %q", routeros.RouterOSVersion)
			}
			time.Sleep(time.Millisecond)
			if routeros.RouterOSVersion != "7.20.1" {
				return diag.Errorf("RouterOS version changed during callback")
			}
			calls.Add(1)
			return nil
		},
	}
	p := WrapProvider(&schema.Provider{ResourcesMap: map[string]*schema.Resource{"test": r}})
	meta := &Meta{Client: &routeros.RestClient{Client: &http.Client{}}, Version: "7.20.1"}
	d := schema.TestResourceDataRaw(t, r.Schema, map[string]any{})

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if diags := p.ResourcesMap["test"].ReadContext(context.Background(), d, meta); diags.HasError() {
				t.Errorf("guarded callback failed: %v", diags)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 20 {
		t.Fatalf("callback ran %d times, want 20", calls.Load())
	}
}

func TestRefreshDoesNotRunValidateFunc(t *testing.T) {
	var validations atomic.Int32
	r := &schema.Resource{
		Schema: map[string]*schema.Schema{
			"policy": {
				Type:     schema.TypeSet,
				Optional: true,
				Computed: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
					ValidateFunc: func(any, string) ([]string, []error) {
						validations.Add(1)
						return nil, []error{assertionError("validator must not run during refresh")}
					},
				},
			},
		},
		ReadContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
			d.SetId("*1")
			if err := d.Set("policy", []string{"romon"}); err != nil {
				return diag.FromErr(err)
			}
			return nil
		},
	}
	state := &tf.InstanceState{ID: "*1", Attributes: map[string]string{}}
	refreshed, diags := r.RefreshWithoutUpgrade(context.Background(), state, nil)
	if diags.HasError() {
		t.Fatalf("refresh failed: %v", diags)
	}
	if refreshed == nil || refreshed.ID != "*1" {
		t.Fatalf("unexpected refreshed state: %#v", refreshed)
	}
	if validations.Load() != 0 {
		t.Fatalf("ValidateFunc ran %d times during refresh", validations.Load())
	}
}

type assertionError string

func (e assertionError) Error() string { return string(e) }
