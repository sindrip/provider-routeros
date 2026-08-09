package rest

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"
)

// The live tests run against a disposable CHR, following the repo's existing
// convention:
//
//	hack/chr/run.sh
//	CHR_REST=http://127.0.0.1:18080 go test -run LiveCHR ./rest/...
//	hack/chr/run.sh stop
//
// They cover the behaviours no fake can vouch for — the ones where the value
// of the test is that a real RouterOS agreed.
//
// Note that the tests take their context from t.Context() but the cleanups do
// not: that context is cancelled just before Cleanup-registered functions run,
// so a teardown using it would never reach the router and would leave rows
// behind on the CHR.
func liveClient(t *testing.T) *Client {
	t.Helper()
	endpoint := os.Getenv("CHR_REST")
	if endpoint == "" {
		t.Skip("set CHR_REST to run against a live RouterOS (see hack/chr/run.sh)")
	}
	c, err := New(endpoint, WithBasicAuth("admin", ""), WithHTTPClient(&http.Client{}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// probeList is an address list: inert unless a firewall rule references one,
// so rows can be created and deleted without affecting the router.
const probeList = "/ip/firewall/address-list"

func TestLiveCHRCreateUsesPUTAndPOSTCreatesNothing(t *testing.T) {
	c, ctx := liveClient(t), t.Context()

	before, err := c.Count(ctx, probeList)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}

	rec, err := c.Create(ctx, probeList, Record{"list": "probe", addressField: "192.0.2.1"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if rec.ID() == "" {
		t.Fatal("Create returned no .id")
	}
	t.Cleanup(func() {
		if err := c.Delete(context.Background(), probeList, rec.ID()); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})

	// POST to the same menu path is the console-command verb: it creates
	// nothing, and reports no error while doing so.
	if _, err := c.Command(ctx, probeList, "", Record{"list": "probe", addressField: "192.0.2.99"}); err == nil {
		after, cerr := c.Count(ctx, probeList)
		if cerr != nil {
			t.Fatalf("Count: %v", cerr)
		}
		if after != before+1 {
			t.Errorf("count = %d after PUT and POST, want %d — POST should have created nothing", after, before+1)
		}
	}
}

// TestLiveCHRUpdateRoundTrips exercises PATCH against a real row, and checks
// the .id survives the update — that id is the only handle on the row, and
// RouterOS reassigns it on delete-and-recreate.
func TestLiveCHRUpdateRoundTrips(t *testing.T) {
	c, ctx := liveClient(t), t.Context()

	rec, err := c.Create(ctx, probeList, Record{"list": "probe", addressField: "192.0.2.4"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = c.Delete(context.Background(), probeList, rec.ID()) })

	updated, err := c.Update(ctx, probeList, rec.ID(), Record{"comment": "updated by rest"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.String("comment") != "updated by rest" {
		t.Errorf("comment = %q after update", updated.String("comment"))
	}
	if updated.ID() != rec.ID() {
		t.Errorf("id = %q after update, want %q unchanged", updated.ID(), rec.ID())
	}

	// And the change is visible on a fresh read, not just echoed back.
	got, err := c.List(ctx, probeList, Where("comment", "updated by rest"))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].ID() != rec.ID() {
		t.Errorf("re-read returned %d rows: %v", len(got), got)
	}
}

// TestLiveCHRUpdateOfMissingRowFails checks that patching a row that is gone is
// an error rather than a silent no-op — the reconciler case where an id was
// reassigned out from under the caller.
func TestLiveCHRUpdateOfMissingRowFails(t *testing.T) {
	c := liveClient(t)
	if _, err := c.Update(t.Context(), probeList, "*FFFFFFFE", Record{"comment": "nope"}); err == nil {
		t.Error("want an error patching a nonexistent id")
	}
}

// TestLiveCHRFilterCarriesHostileValues is the reason listings use a POST body.
// The comment holds a space, an ampersand and an equals sign, none of which is
// URL-encoded anywhere on this path.
func TestLiveCHRFilterCarriesHostileValues(t *testing.T) {
	c, ctx := liveClient(t), t.Context()
	const comment = "accept established & more = fun"

	rec, err := c.Create(ctx, probeList, Record{
		"list": "probe", addressField: "192.0.2.2", "comment": comment,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = c.Delete(context.Background(), probeList, rec.ID()) })

	got, err := c.List(ctx, probeList, Where("comment", comment))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].String("comment") != comment {
		t.Fatalf("filtered list returned %d rows: %v", len(got), got)
	}
	if got[0].ID() != rec.ID() {
		t.Errorf("id = %q, want %q", got[0].ID(), rec.ID())
	}
}

// TestLiveCHRPropsStillCarriesID pins the asymmetry that would otherwise cost a
// reconciler its handle on the row: POST .../print omits .id unless it is asked
// for by name, while GET ?.proplist= includes it regardless.
func TestLiveCHRPropsStillCarriesID(t *testing.T) {
	c, ctx := liveClient(t), t.Context()

	rec, err := c.Create(ctx, probeList, Record{"list": "probe", addressField: "192.0.2.3"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = c.Delete(context.Background(), probeList, rec.ID()) })

	got, err := c.List(ctx, probeList, Props(addressField), Where(addressField, "192.0.2.3"))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1", len(got))
	}
	if got[0].ID() == "" {
		t.Error("Props dropped .id; the row cannot be addressed for an update")
	}
	if got[0].String(addressField) != "192.0.2.3" {
		t.Errorf("address = %q", got[0].String(addressField))
	}
}

// TestLiveCHRErrorBodyIsTyped checks that a failure the router describes in a
// body — which decodes exactly like a record, and carries "error" as a number —
// arrives as a typed Error rather than as a decode failure or, worse, as data.
//
// A missing menu is used rather than /system/routerboard: the evidence for
// that one came from an x86 CHR, which answers 400 "no such command or
// directory (routerboard)", but the arm64 image has the menu and answers
// 200 {"routerboard":"false"}. Same RouterOS 7.23.2, different image.
func TestLiveCHRErrorBodyIsTyped(t *testing.T) {
	c, ctx := liveClient(t), t.Context()

	for _, tc := range []struct {
		name string
		call func() error
	}{
		{"missing menu", func() error {
			_, err := c.List(ctx, "/system/no-such-menu")
			return err
		}},
		{"unknown parameter", func() error {
			_, err := c.Create(ctx, probeList, Record{"bogus-field": "x"})
			return err
		}},
	} {
		err := tc.call()
		if err == nil {
			t.Errorf("%s: want an error", tc.name)
			continue
		}
		re, ok := errors.AsType[*Error](err)
		if !ok {
			t.Errorf("%s: want *rest.Error, got %T: %v", tc.name, err, err)
			continue
		}
		if re.Code != http.StatusBadRequest || re.Detail == "" {
			t.Errorf("%s: error not decoded from the body: %+v", tc.name, re)
		}
	}
}

func TestLiveCHRSingletonReadAndCount(t *testing.T) {
	c, ctx := liveClient(t), t.Context()

	// A singleton answers with a bare object.
	res, err := c.Get(ctx, "/system/resource")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if res.String("version") == "" {
		t.Error("no version in /system/resource")
	}
	if res.Duration("uptime") <= 0 {
		t.Errorf("uptime = %q did not parse", res.String("uptime"))
	}

	// count-only answers {"ret":"<string>"}.
	n, err := c.Count(ctx, "/interface")
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	rows, err := c.List(ctx, "/interface")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if n != len(rows) {
		t.Errorf("Count = %d but List returned %d rows", n, len(rows))
	}
}

// TestLiveCHRHexValuedField covers a bridge's priority, which reads back as
// "0x8000" although its stated bound is a bare "0..FFFF".
func TestLiveCHRHexValuedField(t *testing.T) {
	c, ctx := liveClient(t), t.Context()

	br, err := c.Create(ctx, "/interface/bridge", Record{"name": "rest-probe-br"})
	if err != nil {
		t.Fatalf("Create bridge: %v", err)
	}
	t.Cleanup(func() { _ = c.Delete(context.Background(), "/interface/bridge", br.ID()) })

	if got := br.String("priority"); got != "0x8000" {
		t.Errorf("priority = %q, want the 0x-prefixed default", got)
	}
	if got := br.Uint("priority"); got != 0x8000 {
		t.Errorf("Uint(priority) = %d, want 32768 — a base-10 reader gets 0", got)
	}
}
