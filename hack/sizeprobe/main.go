// sizeprobe asks a real Kubernetes API server how large a menu resource can be.
//
// ADR 0004 makes the menu the unit of reconciliation and names one open risk:
// /ip/firewall/filter on a real router is hundreds of rows in one spec,
// against etcd's request limit and the API server's patch semantics. This
// program measures that instead of arguing about it. It installs the CR shape
// the ADR implies — rows as an atomic list of string maps in spec, mirrored in
// status.atProvider, with a per-row status list — into an envtest control
// plane (a real kube-apiserver on a real etcd, both with default limits), then
// applies menus of increasing size the way a controller would, with
// server-side apply under one field manager.
//
// Three questions, answered per row shape:
//
//   - What does a stored menu cost? Total object bytes as the API server
//     returns them, split into spec, status and managedFields — the last
//     because an atomic list should cost one managedFields entry however many
//     rows it holds, and that is a claim to verify, not assume.
//
//   - What does a one-row change cost? Server-side apply sends the whole
//     intent every time, so the request for "fix one comment" is the size of
//     the menu, not the size of the fix.
//
//   - Where is the ceiling? Rows are added until a write is refused, and the
//     refusal is recorded verbatim — whether it names etcd's request limit or
//     the API server's own body cap decides what a deployment can tune.
//
// Rows are synthesized from the pinned IR rather than invented: a typical set
// cycles archetypes of real firewall policy (~5 fields a rule), and a dense
// set fills every writable field the menu has, which no human writes and is
// the honest worst case. The observed mirror in status is the desired row
// plus .id and the menu's read-only fields; a live router would add defaulted
// fields on top, so the mirror here is a floor, not an exaggeration — stated
// in the output so a reader can discount it.
//
// The API server is disposable and local; nothing here touches a router or a
// cluster anyone cares about.
//
//	cd hack/sizeprobe && go run -buildvcs=false . \
//	  -assets "$(go run sigs.k8s.io/controller-runtime/tools/setup-envtest@v0.24.1 use -p path 1.36.2)" \
//	  > ../../config/menu-object-size.json
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	ros "github.com/sindrip/provider-routeros/schema"
)

var (
	assets = flag.String("assets", "", "directory holding the envtest kube-apiserver and etcd binaries")
	menu   = flag.String("menu", "/ip/firewall/filter", "menu to synthesize rows for; must be in the IR")
)

const fieldManager = "provider-routeros"

var gvr = k8sschema.GroupVersionResource{Group: "probe.routeros", Version: "v1alpha1", Resource: "menus"}

// Measurement is one menu size, applied and read back.
type Measurement struct {
	Shape string `json:"shape"`
	Rows  int    `json:"rows"`
	// ApplyRequestBytes is the JSON body of the server-side apply that
	// creates the menu — what every reconcile sends, changed or not.
	ApplyRequestBytes int `json:"apply_request_bytes"`
	// StoredBytes is the whole object as the API server returns it after
	// spec and status are both written, managedFields included. CRs are
	// stored in etcd as JSON, so this is what counts against its limit.
	StoredBytes        int `json:"stored_bytes"`
	SpecBytes          int `json:"spec_bytes"`
	StatusBytes        int `json:"status_bytes"`
	ManagedFieldsBytes int `json:"managed_fields_bytes"`
	// OneRowChangeRequestBytes is the apply body for changing a single
	// row's comment. Atomic list, full intent: expect it to be the size of
	// the menu.
	OneRowChangeRequestBytes int `json:"one_row_change_request_bytes"`
}

// Ceiling is where writes stopped working for a shape.
type Ceiling struct {
	Shape string `json:"shape"`
	// MaxRowsOK is the largest row count where spec and status both stored.
	MaxRowsOK int `json:"max_rows_ok"`
	// FirstFailureRows is the smallest count where a write was refused.
	FirstFailureRows int `json:"first_failure_rows"`
	// FailedAt is which write was refused: spec-apply or status-apply. The
	// status write carries spec plus mirror, so it is expected to bind first.
	FailedAt string `json:"failed_at"`
	// Refusal is the API server's message, verbatim, so the limit that bound
	// is named by the system that enforced it.
	Refusal string `json:"refusal"`
}

// Report is the probe's verdict.
type Report struct {
	GeneratedBy       string              `json:"generated_by"`
	KubernetesVersion string              `json:"kubernetes_version"`
	Menu              string              `json:"menu"`
	RouterOSVersion   string              `json:"routeros_version"`
	MirrorNote        string              `json:"mirror_note"`
	Shapes            map[string]RowShape `json:"shapes"`
	Measurements      []Measurement       `json:"measurements"`
	Ceilings          []Ceiling           `json:"ceilings"`
}

// RowShape describes what one synthesized row of a shape weighs, averaged
// over a full archetype cycle so no single archetype speaks for the mix.
type RowShape struct {
	Description     string `json:"description"`
	FieldsPerRowAvg int    `json:"fields_per_row_avg"`
	RowJSONBytesAvg int    `json:"row_json_bytes_avg"`
}

func main() {
	flag.Parse()
	log.SetFlags(0)
	log.SetOutput(os.Stderr)
	if *assets == "" {
		log.Fatal("sizeprobe: -assets is required; see the doc comment for how to fetch them")
	}

	ir, err := ros.Load()
	if err != nil {
		log.Fatalf("sizeprobe: %v", err)
	}
	m, ok := ir.Menu(*menu)
	if !ok {
		log.Fatalf("sizeprobe: %s is not in the IR", *menu)
	}

	env := &envtest.Environment{
		BinaryAssetsDirectory: *assets,
		CRDs:                  []*apiextensionsv1.CustomResourceDefinition{menuCRD()},
	}
	cfg, err := env.Start()
	if err != nil {
		log.Fatalf("sizeprobe: starting the control plane: %v", err)
	}
	defer func() {
		if err := env.Stop(); err != nil {
			log.Printf("sizeprobe: stopping the control plane: %v", err)
		}
	}()

	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("sizeprobe: %v", err)
	}
	disco, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		log.Fatalf("sizeprobe: %v", err)
	}
	version, err := disco.ServerVersion()
	if err != nil {
		log.Fatalf("sizeprobe: %v", err)
	}
	log.Printf("control plane up: kubernetes %s", version.GitVersion)

	report := Report{
		GeneratedBy:       "hack/sizeprobe",
		KubernetesVersion: version.GitVersion,
		Menu:              *menu,
		RouterOSVersion:   ir.RouterOSVersion,
		MirrorNote: "status.atProvider mirrors the desired row plus .id and the menu's " +
			"read-only fields; a live router also returns defaulted fields, so real " +
			"stored sizes sit above these, not below",
		Shapes: map[string]RowShape{},
	}

	ctx := context.Background()
	shapes := []struct {
		name        string
		description string
		rows        func(n int) []map[string]string
	}{
		{"typical", "archetypes of real firewall policy, ~5 fields a rule", func(n int) []map[string]string { return typicalRows(n) }},
		{"dense", "every writable field the menu has set on every row", func(n int) []map[string]string { return denseRows(m, n) }},
	}

	for _, shape := range shapes {
		sample := shape.rows(7) // one full archetype cycle
		fields, bytes := 0, 0
		for _, row := range sample {
			fields += len(row)
			bytes += jsonLen(row)
		}
		report.Shapes[shape.name] = RowShape{
			Description:     shape.description,
			FieldsPerRowAvg: fields / len(sample),
			RowJSONBytesAvg: bytes / len(sample),
		}

		lastOK := 0
		var firstFail int
		var failedAt, refusal string
		for _, n := range []int{100, 250, 500, 1000, 2000, 4000, 8000} {
			meas, failStage, failMsg := measure(ctx, dyn, m, shape.name, shape.rows(n), n)
			if failStage != "" {
				firstFail, failedAt, refusal = n, failStage, failMsg
				log.Printf("%s/%d: refused at %s: %s", shape.name, n, failStage, failMsg)
				break
			}
			report.Measurements = append(report.Measurements, meas)
			lastOK = n
			log.Printf("%s/%d: stored %d bytes (spec %d, status %d, managedFields %d)",
				shape.name, n, meas.StoredBytes, meas.SpecBytes, meas.StatusBytes, meas.ManagedFieldsBytes)
		}

		// Binary-search the exact ceiling between the last success and the
		// first refusal. If nothing was refused at 8000 rows the ceiling is
		// beyond any menu that exists and searching further proves nothing.
		if firstFail > 0 {
			lo, hi := lastOK, firstFail
			for hi-lo > 1 {
				mid := (lo + hi) / 2
				_, failStage, failMsg := attempt(ctx, dyn, m, shape.name, shape.rows(mid), mid)
				if failStage == "" {
					lo = mid
				} else {
					hi, failedAt, refusal = mid, failStage, failMsg
				}
			}
			report.Ceilings = append(report.Ceilings, Ceiling{
				Shape:            shape.name,
				MaxRowsOK:        lo,
				FirstFailureRows: hi,
				FailedAt:         failedAt,
				Refusal:          refusal,
			})
			log.Printf("%s: ceiling between %d and %d rows (%s)", shape.name, lo, hi, failedAt)
		} else {
			report.Ceilings = append(report.Ceilings, Ceiling{
				Shape:     shape.name,
				MaxRowsOK: lastOK,
			})
			log.Printf("%s: no refusal by %d rows", shape.name, lastOK)
		}
	}

	out, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		log.Fatalf("sizeprobe: %v", err)
	}
	fmt.Println(string(out))
}

// measure applies a menu of n rows, mirrors it into status, and reads back
// what the API server stored. A refusal is returned rather than fatal so the
// caller can record where the ceiling is.
func measure(ctx context.Context, dyn dynamic.Interface, m ros.Menu, shape string, rows []map[string]string, n int) (Measurement, string, string) {
	name := objectName(shape, n)
	defer deleteQuietly(ctx, dyn, name)

	meas := Measurement{Shape: shape, Rows: n}

	obj := menuObject(name, rows)
	body, err := json.Marshal(obj.Object)
	if err != nil {
		return meas, "marshal", err.Error()
	}
	meas.ApplyRequestBytes = len(body)

	if _, err := dyn.Resource(gvr).Apply(ctx, name, obj, metav1.ApplyOptions{FieldManager: fieldManager, Force: true}); err != nil {
		return meas, "spec-apply", err.Error()
	}

	status := statusObject(name, m, rows)
	if _, err := dyn.Resource(gvr).ApplyStatus(ctx, name, status, metav1.ApplyOptions{FieldManager: fieldManager, Force: true}); err != nil {
		return meas, "status-apply", err.Error()
	}

	stored, err := dyn.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return meas, "get", err.Error()
	}
	meas.StoredBytes = jsonLen(stored.Object)
	meas.SpecBytes = jsonLen(stored.Object["spec"])
	meas.StatusBytes = jsonLen(stored.Object["status"])
	if md, ok := stored.Object["metadata"].(map[string]interface{}); ok {
		meas.ManagedFieldsBytes = jsonLen(md["managedFields"])
	}

	// One row's comment changes; the whole intent is what goes on the wire.
	rows[n/2]["comment"] = fmt.Sprintf("edited by sizeprobe to measure a one-row change at %d rows", n)
	changed := menuObject(name, rows)
	body, err = json.Marshal(changed.Object)
	if err != nil {
		return meas, "marshal", err.Error()
	}
	meas.OneRowChangeRequestBytes = len(body)
	if _, err := dyn.Resource(gvr).Apply(ctx, name, changed, metav1.ApplyOptions{FieldManager: fieldManager, Force: true}); err != nil {
		return meas, "one-row-apply", err.Error()
	}

	return meas, "", ""
}

// attempt is measure without the readback, for the ceiling search.
func attempt(ctx context.Context, dyn dynamic.Interface, m ros.Menu, shape string, rows []map[string]string, n int) (Measurement, string, string) {
	name := objectName(shape, n)
	defer deleteQuietly(ctx, dyn, name)

	if _, err := dyn.Resource(gvr).Apply(ctx, name, menuObject(name, rows), metav1.ApplyOptions{FieldManager: fieldManager, Force: true}); err != nil {
		return Measurement{}, "spec-apply", err.Error()
	}
	if _, err := dyn.Resource(gvr).ApplyStatus(ctx, name, statusObject(name, m, rows), metav1.ApplyOptions{FieldManager: fieldManager, Force: true}); err != nil {
		return Measurement{}, "status-apply", err.Error()
	}
	return Measurement{}, "", ""
}

// menuObject is the CR a controller would apply: the menu path, the required
// unlisted policy, and the rows in order.
func menuObject(name string, rows []map[string]string) *unstructured.Unstructured {
	list := make([]interface{}, len(rows))
	for i, r := range rows {
		row := make(map[string]interface{}, len(r))
		for k, v := range r {
			row[k] = v
		}
		list[i] = row
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": gvr.Group + "/" + gvr.Version,
		"kind":       "Menu",
		"metadata":   map[string]interface{}{"name": name},
		"spec": map[string]interface{}{
			"path":     "/ip/firewall/filter",
			"ordered":  true,
			"unlisted": "tolerate",
			"rows":     list,
		},
	}}
}

// statusObject is what the controller writes back after a reconcile: the
// observed rows and a per-row condition, which ADR 0004 requires because a
// single bad row must be reportable from a single object.
func statusObject(name string, m ros.Menu, desired []map[string]string) *unstructured.Unstructured {
	observed := make([]interface{}, len(desired))
	perRow := make([]interface{}, len(desired))
	for i, r := range desired {
		row := make(map[string]interface{}, len(r)+8)
		for k, v := range r {
			row[k] = v
		}
		id := fmt.Sprintf("*%X", i+1)
		row[".id"] = id
		for k, v := range readOnlyFields(m) {
			row[k] = v
		}
		observed[i] = row
		perRow[i] = map[string]interface{}{
			"index": int64(i),
			"id":    id,
			"ready": true,
		}
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": gvr.Group + "/" + gvr.Version,
		"kind":       "Menu",
		"metadata":   map[string]interface{}{"name": name},
		"status": map[string]interface{}{
			"atProvider": map[string]interface{}{"rows": observed},
			"rowStatus":  perRow,
		},
	}}
}

// menuCRD is the shape ADR 0004 implies, in the generic-kind variant: rows as
// an atomic list of string maps. Atomic is not a choice here — position is
// identity, so there is no merge key a granular list could use.
func menuCRD() *apiextensionsv1.CustomResourceDefinition {
	stringMap := apiextensionsv1.JSONSchemaProps{
		Type: "object",
		AdditionalProperties: &apiextensionsv1.JSONSchemaPropsOrBool{
			Schema: &apiextensionsv1.JSONSchemaProps{Type: "string"},
		},
	}
	atomic := "atomic"
	rows := apiextensionsv1.JSONSchemaProps{
		Type:      "array",
		XListType: &atomic,
		Items:     &apiextensionsv1.JSONSchemaPropsOrArray{Schema: &stringMap},
	}
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: gvr.Resource + "." + gvr.Group},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: gvr.Group,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   gvr.Resource,
				Singular: "menu",
				Kind:     "Menu",
				ListKind: "MenuList",
			},
			Scope: apiextensionsv1.ClusterScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
				Name:    gvr.Version,
				Served:  true,
				Storage: true,
				Subresources: &apiextensionsv1.CustomResourceSubresources{
					Status: &apiextensionsv1.CustomResourceSubresourceStatus{},
				},
				Schema: &apiextensionsv1.CustomResourceValidation{
					OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]apiextensionsv1.JSONSchemaProps{
							"spec": {
								Type: "object",
								Properties: map[string]apiextensionsv1.JSONSchemaProps{
									"path":     {Type: "string"},
									"ordered":  {Type: "boolean"},
									"unlisted": {Type: "string", Enum: []apiextensionsv1.JSON{{Raw: []byte(`"tolerate"`)}, {Raw: []byte(`"prune"`)}}},
									"rows":     rows,
								},
							},
							"status": {
								Type: "object",
								Properties: map[string]apiextensionsv1.JSONSchemaProps{
									"atProvider": {
										Type: "object",
										Properties: map[string]apiextensionsv1.JSONSchemaProps{
											"rows": rows,
										},
									},
									"rowStatus": {
										Type:      "array",
										XListType: &atomic,
										Items: &apiextensionsv1.JSONSchemaPropsOrArray{
											Schema: &apiextensionsv1.JSONSchemaProps{
												Type: "object",
												Properties: map[string]apiextensionsv1.JSONSchemaProps{
													"index":   {Type: "integer"},
													"id":      {Type: "string"},
													"ready":   {Type: "boolean"},
													"message": {Type: "string"},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}},
		},
	}
}

func objectName(shape string, n int) string {
	return fmt.Sprintf("ip-firewall-filter-%s-%d", shape, n)
}

func deleteQuietly(ctx context.Context, dyn dynamic.Interface, name string) {
	if err := dyn.Resource(gvr).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		log.Printf("sizeprobe: deleting %s: %v", name, err)
	}
}

func jsonLen(v interface{}) int {
	if v == nil {
		return 0
	}
	b, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	return len(b)
}
