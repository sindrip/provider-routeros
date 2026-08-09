// Package schema is the intermediate representation the generators read.
//
// It is one pinned answer to "what does this RouterOS have", assembled from
// artifacts that were each probed off a live device rather than read out of a
// manual — the manual has been wrong in both directions often enough that it
// is a cross-check here, never a source.
//
//	config/console-tree.json    hack/inspectdump   structure: menus, commands, arguments
//	config/arg-types.json       hack/typedump      types, as the router states them
//	config/name-uniqueness.json hack/uniqprobe     whether a name is enforced unique
//
// The IR's job is to turn those into facts an emitter can act on, and — just
// as importantly — to say where it cannot. A generator that silently invents a
// field type or an identity key produces exactly the class of bug this repo
// keeps finding: a resource that reads Synced while being wrong.
//
// # Trusting a fact
//
// Provenance is recorded where a generator has a decision to make, rather than
// stamped on every attribute where it would be uniform noise. Three signals
// carry it:
//
//   - Menu.Typed is false when hack/typedump never reached the menu, so its
//     fields carry structure but no types.
//   - Field.Kind is KindUnknown when the router offered neither a vocabulary
//     nor a grammar for that field. It is not a guess to be filled in.
//   - Identity.Verdict is Unprobed when uniqueness was never tested, which is
//     a different claim from Untested — that one was tried and was
//     inconclusive. Neither means "unique".
//
// An emitter should refuse to generate identity from anything but Unique, and
// should refuse to type a field from KindUnknown.
package schema

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed ir.json
var pinned []byte

// Load returns the pinned IR.
func Load() (*IR, error) {
	var ir IR
	if err := json.Unmarshal(pinned, &ir); err != nil {
		return nil, fmt.Errorf("schema: parsing the pinned IR: %w", err)
	}
	return &ir, nil
}

// IR is the whole device surface, as probed from one RouterOS version.
type IR struct {
	RouterOSVersion string   `json:"routeros_version"`
	GeneratedBy     string   `json:"generated_by"`
	Sources         []Source `json:"sources"`
	Menus           []Menu   `json:"menus"`
}

// Source records one artifact the IR was assembled from.
type Source struct {
	Artifact string `json:"artifact"`
	Producer string `json:"producer"`
	Version  string `json:"routeros_version,omitempty"`
}

// Class is how a menu behaves, derived from the commands it exposes. This is
// mechanical: RouterOS tells the console tree which commands a menu has, and
// the shape follows from that rather than from anyone's judgement.
type Class string

const (
	// ClassOrdered menus expose move, so position carries meaning and
	// first-match-wins semantics apply. Firewall chains are the archetype.
	ClassOrdered Class = "ordered-list"
	// ClassList menus expose add: they hold rows, in no significant order.
	ClassList Class = "list"
	// ClassSingleton menus expose set but not add — one row of settings,
	// which also means print has nothing to filter.
	ClassSingleton Class = "singleton"
	// ClassReadOnly menus expose neither: a table the device maintains.
	ClassReadOnly Class = "read-only"
)

// Menu is one addressable RouterOS menu.
type Menu struct {
	Path     string   `json:"path"`
	Class    Class    `json:"class"`
	Commands []string `json:"commands"`
	Identity Identity `json:"identity"`
	Fields   []Field  `json:"fields,omitempty"`
	// Typed is false when hack/typedump never reached this menu, so the
	// fields below are structure without types.
	Typed bool `json:"typed"`
}

// Rows reports whether the menu holds rows that can be addressed individually.
func (m Menu) Rows() bool { return m.Class == ClassOrdered || m.Class == ClassList }

// Verdict is what a uniqueness probe concluded.
type Verdict string

const (
	// Unique means a second create with the same name was rejected.
	Unique Verdict = "UNIQUE"
	// Duplicate means it was accepted: the field cannot key a row.
	Duplicate Verdict = "DUPLICATE"
	// Untested means the probe ran and could not conclude — usually the
	// first create failed because CHR lacks the hardware.
	Untested Verdict = "UNTESTED"
	// Unprobed means no probe was ever attempted here. It is a distinct
	// claim from Untested, and neither is evidence of uniqueness.
	Unprobed Verdict = "UNPROBED"
)

// Identity is how a row in this menu can be addressed durably.
//
// RouterOS's own .id is not durable: it is reassigned when a row is deleted
// and recreated, which an episodic applier tolerates and a controller
// reconciling forever does not.
type Identity struct {
	// Candidates are fields that could key a row, most preferred first.
	Candidates []string `json:"candidates,omitempty"`
	// Key is the candidate backed by a Unique verdict, or empty.
	Key     string  `json:"key,omitempty"`
	Verdict Verdict `json:"verdict"`
}

// Access is whether a field can be written.
type Access string

const (
	Writable Access = "writable"
	ReadOnly Access = "read-only"
)

// Kind is the shape of a field's value space, as the router described it.
type Kind string

const (
	// KindEnum is a closed set; Values is exhaustive.
	KindEnum Kind = "enum"
	// KindOpenEnum suggests values but accepts any literal.
	KindOpenEnum Kind = "open-enum"
	// KindScalar is freeform; Type and Ranges describe it.
	KindScalar Kind = "scalar"
	// KindUnknown is the router declining to say. Not a gap to fill in.
	KindUnknown Kind = "unknown"
)

// Field is one property of a menu.
type Field struct {
	Name   string   `json:"name"`
	Access Access   `json:"access"`
	Kind   Kind     `json:"kind"`
	Type   string   `json:"type,omitempty"`
	Values []string `json:"values,omitempty"`
	Ranges []string `json:"ranges,omitempty"`
	// Bool is true when the vocabulary is exactly no/yes.
	//
	// Worth stating because the console and the wire disagree: completion
	// offers no/yes, while a REST read returns "true"/"false" — and a third
	// encoding, present-but-empty, means the flag is set. A generator that
	// takes the console vocabulary for the wire format gets all three wrong.
	Bool bool `json:"bool,omitempty"`
}

// Census counts menus by class. The numbers are a property of the device, so
// a change here means RouterOS moved, not that the IR drifted.
func (ir *IR) Census() map[Class]int {
	out := map[Class]int{}
	for _, m := range ir.Menus {
		out[m.Class]++
	}
	return out
}

// Menu returns the menu at path.
func (ir *IR) Menu(path string) (Menu, bool) {
	for _, m := range ir.Menus {
		if m.Path == path {
			return m, true
		}
	}
	return Menu{}, false
}
