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
//   - Field.Kind is KindUnknown when neither the console nor a returned value
//     said anything about the field. It is not a guess to be filled in.
//   - Field.Evidence is Observed where the type came from reading a value
//     rather than from the console describing the field, which is what
//     happens for a read-only property of a menu that holds no rows.
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

// Class is a menu's cardinality, and nothing else.
//
// Cardinality and mutability are independent, and an earlier version of this
// type conflated them: it had a read-only class, which left no way to say that
// /ip/firewall/connection holds thousands of rows the device maintains and no
// caller may write. Whether a menu can be written is Menu.Writable.
type Class string

const (
	// ClassOrdered holds rows whose order carries meaning — the menu
	// exposes move, and first-match-wins applies. Firewall chains.
	ClassOrdered Class = "ordered-list"
	// ClassList holds rows in no significant order.
	ClassList Class = "list"
	// ClassSingleton holds one implicit record. Such a menu has nothing for
	// `print where` to filter, which is how it can be told apart.
	ClassSingleton Class = "singleton"
)

// Evidence is how firmly a derived fact is held.
type Evidence string

const (
	// Probed means a live router demonstrated it.
	Probed Evidence = "probed"
	// Observed means it was read off a value the router returned, rather
	// than declared. Weaker than Probed: it rests on one sample from one
	// device in one state, so a counter reading 0 is indistinguishable from
	// anything else reading 0.
	Observed Evidence = "observed"
	// Inferred means it follows from the console tree's shape alone, which
	// is weaker and has been wrong before.
	Inferred Evidence = "inferred"
)

// Menu is one addressable RouterOS menu.
//
// Namespaces are not menus and do not appear: /ip and /system group other
// menus but expose no print or get of their own, so there is nothing to read.
type Menu struct {
	Path     string   `json:"path"`
	Class    Class    `json:"class"`
	Commands []string `json:"commands"`
	Identity Identity `json:"identity"`
	Fields   []Field  `json:"fields,omitempty"`
	// Typed is false when hack/typedump never reached this menu, so the
	// fields below are structure without types.
	Typed bool `json:"typed"`
	// Writable is whether the menu accepts add or set. It is independent of
	// Class: a menu may hold many rows and accept no writes at all.
	Writable bool `json:"writable"`
	// ClassEvidence distinguishes a class a router demonstrated from one
	// inferred off the command list.
	//
	// The distinction is load-bearing. The obvious inference — no add
	// command means no rows — is false: /interface, /interface/ethernet and
	// the read-only tables all hold rows without one. What settles it is
	// which command could enumerate the menu's properties, since `print
	// where` has nothing to filter on a menu that holds no rows and answers
	// with print's own arguments instead.
	ClassEvidence Evidence `json:"class_evidence"`
}

// Rows reports whether the menu holds rows that can be addressed
// individually, as opposed to one implicit record.
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
	// Evidence is how the type was arrived at: Probed where the console
	// described the field, Observed where only its value was available.
	//
	// A generator may reasonably treat the two differently. An observed type
	// is a reading of one sample, so a field that happened to be 0, or empty,
	// or absent, is typed weakly or not at all.
	Evidence Evidence `json:"evidence,omitempty"`
	// Sample is the value an observed type was read from, so a reader can
	// disagree with the verdict.
	Sample string `json:"sample,omitempty"`
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
