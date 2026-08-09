package config

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

const (
	certCA     = "cert-ca"
	certIssued = "cert-issued"
	attrCA     = "ca"
	attrRevoke = "revoked"
)

// certHarness serves /certificate with the two console verbs upstream's
// delete drives it through: issued-revoke, which only marks the row, and
// remove, which takes it away.
func certHarness(t *testing.T) (*fakeRouter, *schema.Resource, routeros.Client) {
	t.Helper()
	router := &fakeRouter{
		path:   "/rest/certificate",
		nextID: 2,
		items: map[string]map[string]string{
			"*1": {attrID: "*1", attrName: certCA},
			"*2": {attrID: "*2", attrName: certIssued, attrCA: certCA},
		},
		actions: map[string]func(*fakeRouter, map[string]string){
			"issued-revoke": func(f *fakeRouter, body map[string]string) {
				if it := f.byNumbers(body["numbers"]); it != nil {
					it[attrRevoke] = routerTrue
				}
			},
			"remove": func(f *fakeRouter, body map[string]string) {
				if it := f.byNumbers(body["numbers"]); it != nil {
					delete(f.items, it[attrID])
				}
			},
		},
	}
	srv := httptest.NewServer(router.handler())
	t.Cleanup(srv.Close)
	res := providerForRuntime().ResourcesMap["routeros_system_certificate"]
	return router, res, testClient(t, srv.URL)
}

// The fix: upstream revokes an issued certificate and stops, leaving the row
// holding its enforced-unique name. Delete has to take the row away too.
func TestCertificateRemovalRemovesTheRevokedRow(t *testing.T) {
	router, res, client := certHarness(t)
	d := natData(t, res, map[string]string{attrName: certIssued, "common_name": certIssued, attrCA: certCA})
	d.SetId(certIssued)

	if dg := res.DeleteContext(context.Background(), d, client); dg.HasError() {
		t.Fatalf("delete: %v", dg)
	}
	if _, ok := router.items["*2"]; ok {
		t.Errorf("issued certificate survived its delete: %v", router.items["*2"])
	}
	if !called(router, "POST") {
		t.Errorf("upstream revoke never ran, so the fix is not being exercised: %v", router.calls)
	}
	if _, ok := router.items["*1"]; !ok {
		t.Error("removal took the CA with it")
	}
}

// A root CA takes upstream's remove branch, which already takes the row away;
// the follow-up must find nothing and do nothing.
func TestCertificateRemovalIsNoOpForRootCA(t *testing.T) {
	router, res, client := certHarness(t)
	delete(router.items, "*2")
	d := natData(t, res, map[string]string{attrName: certCA, "common_name": certCA})
	d.SetId(certCA)

	if dg := res.DeleteContext(context.Background(), d, client); dg.HasError() {
		t.Fatalf("delete: %v", dg)
	}
	if len(router.items) != 0 {
		t.Errorf("root CA survived its delete: %v", router.items)
	}
	if called(router, "DELETE") {
		t.Errorf("follow-up removal ran even though upstream had already removed the row: %v", router.calls)
	}
}

func TestCertificateRemovalResourceGates(t *testing.T) {
	for _, name := range certificateRemovalResources {
		upstream := routeros.Provider().ResourcesMap[name]
		if upstream == nil {
			t.Fatalf("%s is not an upstream resource", name)
		}
		if upstream.Schema[routeros.KeyName] == nil {
			t.Errorf("%s has no name field, but the follow-up removal resolves the row by name", name)
		}
	}
}
