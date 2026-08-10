// valueprobe types the fields the console refuses to describe.
//
// hack/typedump asks the router what a property accepts, which works wherever
// the console has a cursor position to ask about. A read-only property of a
// menu that holds no rows has none — `set` will not take it and `print where`
// has nothing to filter — so typedump records it as unknown rather than
// guessing. That leaves 182 fields across the settings singletons untyped, and
// it lands hardest on the menus telemetry cares about: every field of
// /system/resource is in that set.
//
// This probe closes the gap with weaker evidence of a different kind. It reads
// each affected menu over REST and classifies what came back, so uptime is a
// time interval because the router returned "4h10m29s", not because anyone
// declared it one.
//
// The evidence is genuinely weaker and is labelled so. It rests on a single
// sample from one device in one state: a counter reading 0 is indistinguishable
// from anything else that reads 0, and a field that happens to be empty says
// nothing at all. A field the router did not return, or returned empty, yields
// no verdict rather than a guess; and every verdict carries the sample it came
// from, so a reader can disagree with it.
//
// Usage, against hack/chr:
//
//	cd hack/valueprobe && go run -buildvcs=false . > ../../config/observed-types.json
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"

	"github.com/sindrip/provider-routeros/rest"
)

var (
	endpoint = flag.String("endpoint", "http://127.0.0.1:18080", "RouterOS REST endpoint")
	user     = flag.String("user", "admin", "RouterOS user")
	pass     = flag.String("pass", "", "RouterOS password")
	typesIn  = flag.String("types", "../../config/arg-types.json", "the typedump artifact to find gaps in")
)

type argType struct {
	Path   string `json:"path"`
	Arg    string `json:"arg"`
	Access string `json:"access"`
	Kind   string `json:"kind"`
}

type verdict struct {
	Path   string `json:"path"`
	Arg    string `json:"arg"`
	Type   string `json:"type"`
	Sample string `json:"sample"`
}

type dump struct {
	RouterOSVersion string `json:"routeros_version"`
	// The platform, because a value read off one device is evidence about
	// that device: the menu set differs by architecture on the same RouterOS.
	Architecture string    `json:"architecture"`
	BoardName    string    `json:"board_name"`
	GeneratedBy  string    `json:"generated_by"`
	Note            string    `json:"note"`
	Verdicts        []verdict `json:"verdicts"`
}

const note = "Types inferred from the values a live router returned, for fields the console " +
	"would not describe: a read-only property of a rowless menu has no cursor position for " +
	"/console/inspect to answer about. This is weaker evidence than hack/typedump's and is " +
	"marked observed rather than probed in the IR. Each verdict carries its sample. A field " +
	"the router did not return, or returned empty, yields no verdict at all."

func main() {
	flag.Parse()
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	raw, err := os.ReadFile(*typesIn)
	if err != nil {
		return fmt.Errorf("reading %s: %w", *typesIn, err)
	}
	var types struct {
		Args []argType `json:"args"`
	}
	if err := json.Unmarshal(raw, &types); err != nil {
		return fmt.Errorf("parsing %s: %w", *typesIn, err)
	}

	// The gap: read-only properties the console would not type.
	gaps := map[string][]string{}
	for _, a := range types.Args {
		if a.Kind == "unknown" && a.Access == "read-only" && a.Path != "" {
			gaps[a.Path] = append(gaps[a.Path], a.Arg)
		}
	}

	c, err := rest.New(*endpoint, rest.WithBasicAuth(*user, *pass))
	if err != nil {
		return err
	}
	ctx := context.Background()

	version, arch, board := "unknown", "", ""
	if res, err := c.Get(ctx, "/system/resource"); err == nil {
		version = res.String("version")
		arch, board = res.String("architecture-name"), res.String("board-name")
	}

	paths := make([]string, 0, len(gaps))
	for p := range gaps {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	var verdicts []verdict
	var reached, unreachable int
	for _, path := range paths {
		rec, err := c.Get(ctx, "/"+path)
		if err != nil {
			// A menu the device does not have on this hardware. Silence is
			// the right answer; a guess is not.
			unreachable++
			fmt.Fprintf(os.Stderr, "skip /%s: %v\n", path, err)
			continue
		}
		reached++
		for _, arg := range gaps[path] {
			v, present := rec[arg]
			if !present || v == "" {
				continue
			}
			verdicts = append(verdicts, verdict{Path: path, Arg: arg, Type: classify(v), Sample: v})
		}
	}

	sort.Slice(verdicts, func(i, j int) bool {
		if verdicts[i].Path != verdicts[j].Path {
			return verdicts[i].Path < verdicts[j].Path
		}
		return verdicts[i].Arg < verdicts[j].Arg
	})

	out, err := json.MarshalIndent(dump{
		RouterOSVersion: version,
		Architecture:    arch,
		BoardName:       board,
		GeneratedBy:     "hack/valueprobe",
		Note:            note,
		Verdicts:        verdicts,
	}, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(out))

	var gapFields int
	for _, args := range gaps {
		gapFields += len(args)
	}
	fmt.Fprintf(os.Stderr, "%d menus reached, %d unreachable; %d of %d untyped fields now typed\n",
		reached, unreachable, len(verdicts), gapFields)
	return nil
}

var (
	// A duration is a run of count-and-unit terms and nothing else, so a bare
	// "0" is not one. ms leads the alternation for the reason it always does.
	reDuration = regexp.MustCompile(`^(\d+(ms|w|d|h|m|s))+$`)
	reDateTime = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`)
	reInteger  = regexp.MustCompile(`^-?\d+$`)
	reHex      = regexp.MustCompile(`^0[xX][0-9a-fA-F]+$`)
	reFloat    = regexp.MustCompile(`^-?\d+\.\d+$`)
	reMAC      = regexp.MustCompile(`^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$`)
	reIPv4     = regexp.MustCompile(`^(\d{1,3}\.){3}\d{1,3}(/\d{1,2})?$`)
)

// classify names the type a value is evidence for.
//
// Order matters, most specific first, and the fallback is deliberate rather
// than lazy: everything RouterOS returns is transported as a string, so
// "string value" is the one answer that cannot be wrong about the wire. It
// claims no parsing, so nothing downstream can mis-parse on its account —
// unlike guessing at a number or an interval, where being wrong turns a
// readable value into a silent zero.
func classify(v string) string {
	switch {
	case v == "true" || v == "false":
		return "boolean"
	case reDateTime.MatchString(v):
		return "date time"
	case reDuration.MatchString(v):
		return "time interval"
	case reHex.MatchString(v):
		return "hexadecimal number"
	case reInteger.MatchString(v):
		return "integer number"
	case reFloat.MatchString(v):
		return "decimal number"
	case reMAC.MatchString(v):
		return "MAC address"
	case reIPv4.MatchString(v):
		return "IP address"
	default:
		return "string value"
	}
}
