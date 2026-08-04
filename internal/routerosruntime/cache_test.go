package routerosruntime

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

const testRouterOSVersion = "7.20.1"

func TestClientCacheReusesEquivalentConfiguration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var configurations atomic.Int32
	factory := func() *schema.Provider {
		return &schema.Provider{
			Schema: map[string]*schema.Schema{
				"hosturl":          {Type: schema.TypeString, Required: true},
				"username":         {Type: schema.TypeString, Required: true},
				"insecure":         {Type: schema.TypeBool, Optional: true},
				"rest_timeout":     {Type: schema.TypeInt, Optional: true, Default: 59},
				"routeros_version": {Type: schema.TypeString, Optional: true},
			},
			ConfigureContextFunc: func(_ context.Context, d *schema.ResourceData) (any, diag.Diagnostics) {
				configurations.Add(1)
				routeros.RouterOSVersion = d.Get("routeros_version").(string)
				return &routeros.RestClient{Client: &http.Client{}}, nil
			},
		}
	}
	cache := newClientCache(ctx, factory)
	configuration := map[string]any{
		"hosturl":          "router.example.test",
		"username":         "crossplane",
		"insecure":         "true",
		"rest_timeout":     "59",
		"routeros_version": testRouterOSVersion,
	}
	first, err := cache.Configure(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	second, err := cache.Configure(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Error("equivalent configuration did not reuse cached metadata")
	}
	if configurations.Load() != 1 {
		t.Fatalf("provider configured %d times, want 1", configurations.Load())
	}
	if first.Version != testRouterOSVersion {
		t.Fatalf("captured version = %q, want %s", first.Version, testRouterOSVersion)
	}
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Configure(ctx, configuration); err == nil {
		t.Error("Configure succeeded after Close")
	}
}

func TestNormalizeConfiguration(t *testing.T) {
	s := map[string]*schema.Schema{
		"text":    {Type: schema.TypeString},
		"enabled": {Type: schema.TypeBool},
		"timeout": {Type: schema.TypeInt},
	}
	got, err := normalizeConfiguration(map[string]any{
		"text":    "value",
		"enabled": "true",
		"timeout": "42",
	}, s)
	if err != nil {
		t.Fatal(err)
	}
	if got["text"] != "value" || got["enabled"] != true || got["timeout"] != 42 {
		t.Fatalf("unexpected normalized configuration: %#v", got)
	}
}
