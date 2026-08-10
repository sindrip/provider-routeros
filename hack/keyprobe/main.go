// keyprobe asks a disposable router which fields it enforces unique.
//
// This is the question row-as-resource depends on. Addressing a row by a field
// is only safe where the device refuses to hold two rows with the same value —
// otherwise "find the row whose name is X" can match two rows, and a
// reconciler either edits the wrong one or mints a duplicate. hack/uniqprobe
// answered it for 66 menus and left 324 unprobed, for two reasons this program
// removes:
//
//   - It iterated the upstream Terraform provider's resource map, so a menu
//     upstream does not model could not be reached at all. This iterates the
//     IR, which is the device's own menu list.
//
//   - It tested only `name`, and needed roughly forty hand-written value
//     overrides to build a row at all, because a Terraform schema does not say
//     what a value may contain. The IR does: every field carries its kind, its
//     vocabulary, and the type and bounds the router stated. So synthesis is
//     derived here rather than listed, and any field can be a candidate.
//
// The router is also asked what a row requires, rather than being guessed at:
// a create that omits something mandatory comes back naming it, so the missing
// field is filled and the create retried. That loop is bounded, and a menu
// that will not accept a row after it is reported unprobed with the router's
// own last words rather than silently skipped.
//
// # This program writes to the device
//
// Every row it creates is disabled where the menu has a disabled field, which
// matters more than it sounds: an enabled row in /ip/firewall/filter or
// /ip/service can cut the probe's own connection, and a sweep across every
// menu will eventually try. Rows carry a recognisable comment where comments
// are supported, so anything a crash leaves behind can be found and swept.
// Point it at hack/chr, never at a device you care about.
//
//	cd hack/keyprobe && go run -buildvcs=false . -pass '' > ../../config/key-uniqueness.json
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sindrip/provider-routeros/rest"
	"github.com/sindrip/provider-routeros/schema"
)

var (
	endpoint = flag.String("endpoint", "http://127.0.0.1:18080", "RouterOS REST endpoint")
	user     = flag.String("user", "admin", "RouterOS user")
	pass     = flag.String("pass", "", "RouterOS password")
	only     = flag.String("only", "", "probe just this menu path, for debugging")
	maxFill  = flag.Int("fill", 8, "how many times to let the router name a missing field")
)

// marker tags every row this program creates, so a crash leaves something
// findable rather than anonymous debris.
const marker = "keyprobe-delete-me"

// candidates are the fields worth testing as an identity, most likely first.
//
// name and comment are the two a reconciler would actually reach for: name
// because it is the conventional handle, comment because this provider already
// addresses firewall rules by it (ADR 0002) and has never established whether
// the device enforces it. The rest are natural keys for menus that have no
// name at all, which is most of the 324.
var candidates = []string{
	"name", "comment", "address", "dst-address", "src-address", "interface",
	"code", "vlan-id", "service", "host", "target", "prefix", "mac-address",
}

// severing lists menus where even a disabled row can lock the probe out, so
// they are not written to at all. Everything else is protected by disabling
// the row; these are the cases where that is not enough or not possible.
//
// /user and /user/group can remove the account the probe authenticates as.
// /ip/service carries no disabled field in the sense that matters — the row is
// the service — and rewriting it drops REST. /system/* reboots and resets.
var severing = []string{
	"/user", "/user/group", "/ip/service", "/ip/firewall/service-port",
	"/system/scheduler", "/system/script", "/system/watchdog",
	"/interface/wireless/security-profiles",
}

type verdict struct {
	Path    string `json:"path"`
	Field   string `json:"field,omitempty"`
	Verdict string `json:"verdict"`
	Detail  string `json:"detail,omitempty"`
}

type dump struct {
	RouterOSVersion string    `json:"routeros_version"`
	Architecture    string    `json:"architecture"`
	BoardName       string    `json:"board_name"`
	GeneratedBy     string    `json:"generated_by"`
	Note            string    `json:"note"`
	Verdicts        []verdict `json:"verdicts"`
}

const note = "Whether RouterOS enforces uniqueness on a field, established by creating a row " +
	"and then attempting a second row that differs in everything except that field. UNIQUE = the " +
	"second create was rejected and the message names the field. DUPLICATE = it succeeded, so the " +
	"field does not identify a row and addressing by it can match two. AMBIGUOUS = it was rejected " +
	"for some other reason, which is not evidence either way. UNPROBED = no row could be created " +
	"at all, with the router's last refusal as the detail. Absence of a verdict is not absence of a " +
	"constraint; it means this program could not establish one."

func main() {
	flag.Parse()
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	ir, err := schema.Load()
	if err != nil {
		return err
	}
	c, err := rest.New(*endpoint, rest.WithBasicAuth(*user, *pass))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	version, arch, board := "unknown", "", ""
	if res, err := c.Get(ctx, "/system/resource"); err == nil {
		version = res.String("version")
		arch, board = res.String("architecture-name"), res.String("board-name")
	}

	p := &prober{c: c, ctx: ctx}
	var out []verdict
	var creatable, unprobed int

	for _, m := range ir.Menus {
		if !m.Rows() || !m.Writable || !slices.Contains(m.Commands, "add") {
			continue
		}
		if *only != "" && m.Path != *only {
			continue
		}
		if slices.Contains(severing, m.Path) {
			out = append(out, verdict{Path: m.Path, Verdict: "SKIPPED",
				Detail: "writing here can cut the probe's own access"})
			continue
		}

		// Sweep before as well as after. /interface/6to4 is the first tunnel
		// menu alphabetically, and it failed in one run out of two on an
		// orphan the previous run had left in the shared namespace — cleaning
		// up only afterwards means the first menu of the next run pays for it.
		p.sweep(m.Path)
		if strings.HasPrefix(m.Path, "/interface/") {
			p.sweep("/interface")
		}
		row, detail := p.buildable(m)
		if row == nil {
			p.sweep(m.Path)
			unprobed++
			out = append(out, verdict{Path: m.Path, Verdict: "UNPROBED", Detail: detail})
			fmt.Fprintf(os.Stderr, "%-46s UNPROBED  %s\n", m.Path, truncate(detail, 70))
			continue
		}
		creatable++

		var tested int
		for _, field := range candidates {
			f := fieldOf(m, field)
			if f == nil || f.Access != schema.Writable {
				continue
			}
			if _, sent := row[field]; !sent {
				continue
			}
			v := p.uniqueness(m, row, *f)
			out = append(out, v)
			tested++
			fmt.Fprintf(os.Stderr, "%-46s %-10s %s\n", m.Path+" "+field, v.Verdict, truncate(v.Detail, 60))
		}
		p.sweep(m.Path)
		if strings.HasPrefix(m.Path, "/interface/") {
			// Every tunnel type shares one namespace, so an orphan here blocks
			// creates in twenty other menus rather than just this one.
			p.sweep("/interface")
		}
		if tested == 0 {
			out = append(out, verdict{Path: m.Path, Verdict: "NO-CANDIDATE",
				Detail: "a row was created but it carries none of the candidate fields"})
			fmt.Fprintf(os.Stderr, "%-46s NO-CANDIDATE\n", m.Path)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Field < out[j].Field
	})

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(dump{
		RouterOSVersion: version, Architecture: arch, BoardName: board,
		GeneratedBy: "hack/keyprobe", Note: note, Verdicts: out,
	}); err != nil {
		return err
	}

	counts := map[string]int{}
	for _, v := range out {
		counts[v.Verdict]++
	}
	fmt.Fprintf(os.Stderr, "\n%d menus took a row, %d would not; verdicts: %v\n", creatable, unprobed, counts)
	return nil
}

type prober struct {
	c   *rest.Client
	ctx context.Context
}

// transient reports a failure that says nothing about the request. The CHR
// under TCG drops requests and answers "interrupted" often enough to matter:
// two runs of this program disagreed by 29 menus until this was handled, and
// the cause was one create whose reply was lost after the router had already
// made the row. rest deliberately does not retry — a client should not decide
// that for its caller — so the probe does it here, where it can also tell a
// lost reply from a real refusal.
func transient(err error) bool {
	if err == nil {
		return false
	}
	var e *rest.Error
	if errors.As(err, &e) {
		return strings.Contains(strings.ToLower(cmpDetail(e)), "interrupted")
	}
	return true // transport-level: no reply, so the outcome is unknown
}

// create makes a row, surviving a lost reply.
//
// A create is not idempotent, so a blind retry can mint a second row and
// silently invalidate the uniqueness verdict that follows. Instead, when the
// reply is lost, the menu is read back: if the row is there the router did the
// work and only the answer went missing, so its id is recovered rather than
// the create repeated.
func (p *prober) create(path string, row rest.Record) (string, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 300 * time.Millisecond)
		}
		created, err := p.c.Create(p.ctx, path, row)
		if err == nil {
			return created.ID(), nil
		}
		lastErr = err
		if !transient(err) {
			return "", err
		}
		if id := p.findRow(path, row); id != "" {
			return id, nil
		}
	}
	return "", lastErr
}

// findRow locates a row this program created, by the values it sent rather
// than by an id it may never have received.
func (p *prober) findRow(path string, row rest.Record) string {
	rows, err := p.c.List(p.ctx, path)
	if err != nil {
		return ""
	}
	for _, r := range rows {
		match := true
		for _, k := range []string{"name", "comment"} {
			want, sent := row[k]
			if !sent {
				continue
			}
			if r[k] != want {
				match = false
				break
			}
		}
		if match && (row["name"] != "" || row["comment"] != "") {
			return r.ID()
		}
	}
	return ""
}

// del removes a row, retrying because a delete is idempotent and leaving one
// behind poisons every later menu that shares a namespace — one orphaned
// interface blocks creates in all twenty-odd /interface submenus.
func (p *prober) del(path, id string) {
	if id == "" {
		return
	}
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 300 * time.Millisecond)
		}
		err := p.c.Delete(p.ctx, path, id)
		if err == nil || errors.Is(err, rest.ErrNotFound) {
			return
		}
		var e *rest.Error
		if errors.As(err, &e) && e.Status == 404 {
			return
		}
	}
}

// sweep deletes anything recognisable this program left in a menu, whatever
// the reason. Called after every menu so debris cannot accumulate into the
// next one's failure.
func (p *prober) sweep(path string) {
	rows, err := p.c.List(p.ctx, path)
	if err != nil {
		return
	}
	for _, r := range rows {
		if strings.Contains(r["name"], "keyprobe") || strings.Contains(r["comment"], marker) {
			p.del(path, r.ID())
		}
	}
}

// missingRe pulls the field name out of the router's refusal, in the two forms
// it comes in. RouterOS mostly answers in console syntax — "missing =chain=" —
// which is the form that matters and the one a prose-shaped pattern misses.
var missingRe = []*regexp.Regexp{
	regexp.MustCompile(`=([a-z0-9][a-z0-9._-]*)=`),
	regexp.MustCompile(`(?i)(?:missing|required|mandatory)\s+(?:value\s+for\s+)?(?:required\s+)?(?:argument|parameter|property|field)?\s*["'\x60]?([a-z0-9][a-z0-9._-]*)["'\x60]?`),
}

// missingField returns the field the router asked for, or "".
func missingField(detail string) string {
	for _, re := range missingRe {
		if m := re.FindStringSubmatch(detail); m != nil {
			return m[1]
		}
	}
	return ""
}

// buildable creates one row, letting the router name what it is missing, and
// deletes it again. It returns the body that worked, so the uniqueness tests
// can reuse it rather than rediscover it per field.
func (p *prober) buildable(m schema.Menu) (rest.Record, string) {
	row := p.seed(m, 0)
	var last string
	for attempt := 0; attempt < *maxFill; attempt++ {
		id, err := p.create(m.Path, row)
		if err == nil {
			p.del(m.Path, id)
			return row, ""
		}
		var e *rest.Error
		if !errors.As(err, &e) {
			return nil, err.Error()
		}
		last = cmpDetail(e)

		// The router named something it wants. Supply it and try again.
		field := missingField(strings.ToLower(last))
		if field == "" {
			return nil, last
		}
		if _, already := row[field]; already {
			// It is asking again for something already sent, so the value is
			// wrong rather than absent and this loop cannot fix it.
			return nil, last
		}
		f := fieldOf(m, field)
		if f == nil {
			return nil, last + " (the router wants " + field + ", which the IR does not list)"
		}
		row[field] = p.value(*f, 0)
	}
	return nil, last
}

// uniqueness creates two rows that differ in every candidate-adjacent value
// except field, and reports what the router did about the second.
func (p *prober) uniqueness(m schema.Menu, base rest.Record, f schema.Field) verdict {
	v := verdict{Path: m.Path, Field: f.Name}

	first := clone(base)
	second := p.vary(m, base, f.Name)

	firstID, err := p.create(m.Path, first)
	if err != nil {
		v.Verdict, v.Detail = "UNPROBED", "first create: "+errDetail(err)
		return v
	}
	ids := []string{firstID}
	defer func() {
		for _, id := range ids {
			p.del(m.Path, id)
		}
	}()

	dupID, err := p.create(m.Path, second)
	if err == nil {
		ids = append(ids, dupID)
		v.Verdict = "DUPLICATE"
		return v
	}
	detail := errDetail(err)
	v.Detail = detail
	low := strings.ToLower(detail)

	// A refusal only proves a constraint on this field if it says so. Matching
	// "already" or "exists" alone is not enough, and reading those as UNIQUE
	// was wrong on five menus: /interface/eoip answers "already have such
	// tunnel" because the endpoints collided, and ovpn-server names a
	// "protocol-port-vrf combination" — neither has anything to do with the
	// field being held constant. vary cannot always change every other value
	// (a field with one legal value has to stay put), so an unexplained
	// rejection is genuinely undecided rather than confirming.
	var duplicateish bool
	for _, w := range []string{"already", "exists", "duplicate", "unique", "in use"} {
		if strings.Contains(low, w) {
			duplicateish = true
			break
		}
	}
	if duplicateish && strings.Contains(low, strings.ToLower(f.Name)) {
		v.Verdict = "UNIQUE"
	} else {
		v.Verdict = "AMBIGUOUS"
	}
	return v
}

// seed builds the smallest row worth trying: the candidate fields the menu has,
// plus the safety belt.
func (p *prober) seed(m schema.Menu, n int) rest.Record {
	row := rest.Record{}
	for _, name := range candidates {
		if f := fieldOf(m, name); f != nil && f.Access == schema.Writable {
			row[name] = p.value(*f, n)
		}
	}
	// Disabled first, because this is what stops a firewall rule or a service
	// row from severing the connection the probe is using.
	if f := fieldOf(m, "disabled"); f != nil && f.Access == schema.Writable {
		row["disabled"] = "yes"
	}
	if f := fieldOf(m, "comment"); f != nil && f.Access == schema.Writable {
		row["comment"] = fmt.Sprintf("%s-%d", marker, n)
	}
	return row
}

// vary returns a copy of base with every value changed except keep, so that a
// rejection can only be about keep.
func (p *prober) vary(m schema.Menu, base rest.Record, keep string) rest.Record {
	out := rest.Record{}
	for name, v := range base {
		if name == keep || name == "disabled" {
			out[name] = v
			continue
		}
		f := fieldOf(m, name)
		if f == nil {
			out[name] = v
			continue
		}
		if nv := p.value(*f, 1); nv != v {
			out[name] = nv
		} else {
			// A field with one legal value cannot be varied. Dropping it is
			// wrong (it may be required), so it stays and the verdict for this
			// candidate can only ever be AMBIGUOUS, which is honest.
			out[name] = v
		}
	}
	if keep != "comment" {
		if f := fieldOf(m, "comment"); f != nil && f.Access == schema.Writable {
			out["comment"] = fmt.Sprintf("%s-%d", marker, 1)
		}
	}
	return out
}

// value synthesises something the router will accept, from what the IR says
// about the field. n picks between two distinct values so a second row can
// differ.
//
// This is where driving from the IR pays: uniqprobe needed a hand-written
// override per resource because a Terraform schema says only "string". Here the
// router's own vocabulary, stated type and bounds are all available.
func (p *prober) value(f schema.Field, n int) string {
	if f.Bool {
		return "no"
	}
	// A closed vocabulary is the strongest thing available: pick from it, and
	// prefer a member that is not a disabling or empty choice.
	if len(f.Values) > 0 {
		vals := make([]string, 0, len(f.Values))
		for _, v := range f.Values {
			if v == "" || strings.HasPrefix(v, "!") || v == "none" || v == "no" {
				continue
			}
			vals = append(vals, v)
		}
		if len(vals) > 0 {
			return vals[n%len(vals)]
		}
	}

	t := strings.ToLower(f.Type)
	switch {
	case strings.Contains(t, "ipv6"):
		return fmt.Sprintf("2001:db8:%d::1", n+1)
	case strings.Contains(t, "ip address") && strings.Contains(t, "prefix"):
		return fmt.Sprintf("192.0.2.%d/30", 1+n*4)
	case strings.Contains(t, "ip address"):
		return fmt.Sprintf("192.0.2.%d", 1+n)
	case strings.Contains(t, "mac address"):
		return fmt.Sprintf("02:00:00:00:00:%02d", n+1)
	case strings.Contains(t, "time interval"):
		return fmt.Sprintf("%dm", n+1)
	case strings.Contains(t, "hexadecimal"):
		return fmt.Sprintf("0x0%d", n+1)
	case strings.Contains(t, "integer number"), strings.Contains(t, "integer value"):
		return strconv.Itoa(p.inRange(f, n))
	}
	return fmt.Sprintf("keyprobe%d", n+1)
}

// inRange picks a number the router will accept, using the bounds it stated
// rather than a constant that happens to fit most fields.
func (p *prober) inRange(f schema.Field, n int) int {
	lo, hi := 1, 0
	for _, r := range f.Ranges {
		a, b, ok := strings.Cut(r, "..")
		if !ok {
			continue
		}
		x, err1 := strconv.Atoi(strings.TrimSpace(a))
		y, err2 := strconv.Atoi(strings.TrimSpace(b))
		if err1 != nil || err2 != nil || y <= x {
			continue
		}
		lo, hi = x, y
		break
	}
	v := lo + n
	if hi > lo && v > hi {
		v = hi
	}
	// Zero is a legal value in many ranges but reads as "unset" in some menus,
	// so step off it when the range allows.
	if v == 0 && hi > 0 {
		v = 1
	}
	return v
}

func fieldOf(m schema.Menu, name string) *schema.Field {
	for i := range m.Fields {
		if m.Fields[i].Name == name {
			return &m.Fields[i]
		}
	}
	return nil
}

func clone(r rest.Record) rest.Record {
	out := make(rest.Record, len(r))
	for k, v := range r {
		out[k] = v
	}
	return out
}

func errDetail(err error) string {
	var e *rest.Error
	if errors.As(err, &e) {
		return cmpDetail(e)
	}
	return err.Error()
}

func cmpDetail(e *rest.Error) string {
	if e.Detail != "" {
		return e.Detail
	}
	if e.Message != "" {
		return e.Message
	}
	return strconv.Itoa(e.Status)
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
