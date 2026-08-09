package config

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

const (
	liveCertPath = "/certificate"
	liveCA       = "ci-live-ca"
	liveIssued   = "ci-live-issued"
)

// signCertificate drives /certificate/sign, a console action with no exported
// helper in the upstream client: its crud method type is unexported, so a live
// test can only reach it over plain HTTP.
func signCertificate(ctx context.Context, t *testing.T, host string, params map[string]string) {
	t.Helper()
	body, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("encode sign request: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, host+"/rest/certificate/sign",
		strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("sign request: %v", err)
	}
	req.SetBasicAuth("admin", "")
	req.Header.Set("Content-Type", "application/json")
	// Signing generates a key pair; under emulation that takes seconds.
	resp, err := (&http.Client{Timeout: 2 * time.Minute}).Do(req)
	if err != nil {
		t.Fatalf("sign %v: %v", params, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		t.Fatalf("sign %v: HTTP %d", params, resp.StatusCode)
	}
}

func removeCertificate(client routeros.Client, name string) {
	items, err := itemsByField(client, liveCertPath, routeros.KeyName, name)
	if err != nil {
		return
	}
	for _, it := range items {
		_ = routeros.DeleteItem(&routeros.ItemId{Type: routeros.Id, Value: it.GetID(routeros.Id)}, liveCertPath, client)
	}
}

// TestCertificateRemovalLiveCHRIssuedCertificate proves the fix against a real
// router. Deleting a CA-issued certificate upstream posts to
// /certificate/issued-revoke, which marks the row revoked and leaves it in
// place; because the name is enforced unique, the stranded row then blocks
// every recreate. Delete must release the row, and the name with it.
func TestCertificateRemovalLiveCHRIssuedCertificate(t *testing.T) {
	host := os.Getenv("CHR_REST")
	if host == "" {
		t.Skip("set CHR_REST to run against a live CHR")
	}

	client := testClient(t, host)
	res := providerForRuntime().ResourcesMap["routeros_system_certificate"]
	ctx := context.Background()
	t.Cleanup(func() {
		removeCertificate(client, liveIssued)
		removeCertificate(client, liveCA)
	})

	ca, err := routeros.CreateItem(ctx, routeros.MikrotikItem{
		attrName: liveCA, "common-name": liveCA, "days-valid": "365", "key-usage": "key-cert-sign,crl-sign",
	}, liveCertPath, client)
	if err != nil {
		t.Fatalf("CA fixture: %v", err)
	}
	signCertificate(ctx, t, host, map[string]string{"number": ca.GetID(routeros.Id)})

	// Create through the resource: under name identity the id is the name.
	issued := natData(t, res, map[string]string{
		attrName: liveIssued, "common_name": liveIssued, "days_valid": "90",
	})
	if dg := res.CreateContext(ctx, issued, client); dg.HasError() {
		t.Fatalf("create issued: %v", dg)
	}
	if issued.Id() != liveIssued {
		t.Fatalf("id = %q, want the name %q -- name identity is not reaching the hand-written create", issued.Id(), liveIssued)
	}
	signCertificate(ctx, t, host, map[string]string{"number": liveIssued, "ca": liveCA})

	// Confirm the premise: the row is CA-issued, which is what sends upstream
	// down the revoke branch.
	rows, err := itemsByField(client, liveCertPath, routeros.KeyName, liveIssued)
	if err != nil || len(rows) != 1 {
		t.Fatalf("re-read issued: %v (%d rows)", err, len(rows))
	}
	if rows[0][attrCA] != liveCA {
		t.Fatalf("issued row ca = %q, want %q -- signing did not take", rows[0][attrCA], liveCA)
	}
	if dg := res.ReadContext(ctx, issued, client); dg.HasError() {
		t.Fatalf("read issued: %v", dg)
	}
	if issued.Get(attrCA).(string) != liveCA {
		t.Fatalf("state ca = %q, want %q -- delete would not take the revoke branch", issued.Get(attrCA), liveCA)
	}

	if dg := res.DeleteContext(ctx, issued, client); dg.HasError() {
		t.Fatalf("delete issued: %v", dg)
	}

	left, err := itemsByField(client, liveCertPath, routeros.KeyName, liveIssued)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(left) != 0 {
		t.Fatalf("issued certificate survived its delete as %v -- revoked, not removed", left[0][attrRevoke])
	}

	// The point of removing it: the enforced-unique name is free again, so a
	// reset can reconverge.
	if _, err := routeros.CreateItem(ctx, routeros.MikrotikItem{
		attrName: liveIssued, "common-name": liveIssued, "days-valid": "90",
	}, liveCertPath, client); err != nil {
		t.Fatalf("recreate under the same name: %v", err)
	}
}
