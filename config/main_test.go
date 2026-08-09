package config

import (
	"os"
	"testing"

	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

// pinnedRouterOSVersion is the CHR build every probe-verified claim in this
// package was settled against; testClient reports it to the provider so a live
// run and a unit run agree on which drift compensation applies.
const pinnedRouterOSVersion = "7.23.2"

// The two schemas an override can have to reach. Overrides that change a
// generated CRD's shape — a field's type, whether it is settable, whether it
// is a secret — have to be applied to both, and each is checked against both.
const (
	schemaRuntime    = "runtime"
	schemaGeneration = "generation"
)

// fieldPrivateKey recurs across the WireGuard overrides: it is Sensitive on
// two resources and additionally Computed on one, which is the combination
// withoutMaskedDiff exists for.
const fieldPrivateKey = "private_key"

// TestMain pins the upstream provider's RouterOS version for the whole
// package.
//
// routeros.RouterOSVersion is a package-level global that is normally set as a
// side effect of building a client. A unit test that never builds one — one
// that just serializes a schema, say — leaves it empty, and upstream's drift
// compensation does not tolerate that: GetDriftMap parses the version on every
// call and answers a parse failure with log.Fatal. That kills the test binary
// outright, so the running test reports neither pass nor fail, every test
// after it is silently skipped, and the package fails with nothing but
// "RouterOS version parts parsing error" to show for it. It took an A/B
// against a stashed change to establish that the failure had nothing to do
// with the change under test.
//
// Setting it once here costs nothing and makes the whole package immune:
// serialization goes through the same drift table the pinned CHR would use,
// which is what these tests mean to exercise anyway.
func TestMain(m *testing.M) {
	routeros.RouterOSVersion = pinnedRouterOSVersion
	os.Exit(m.Run())
}
