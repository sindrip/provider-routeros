// telemetry is the spike that answers whether rest/ plus the generated types
// can carry the OTel collector's receiver.
//
// The receiver at homelab/images/routeros-collector currently owns ~90 lines
// of transport and parsing — get, dur, num, flt, boolOf — which is the third
// copy of that code in this codebase and the one with the fewest tests. This
// program reads the same six menus from the same device and produces the same
// numbers, importing rest for the transport and gen/routeros for the typing.
// Nothing here parses a string.
//
// It is a spike, not a receiver: there is no pdata, no mdatagen, no config. The
// question it answers is narrower and comes first — does the substrate reach
// every value the receiver needs, on a real device, without a hand-rolled
// parser anywhere. Where it does not, it says so rather than papering over it.
//
//	cd hack/telemetry && go run -buildvcs=false . -pass ''
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/sindrip/provider-routeros/gen/routeros"
	"github.com/sindrip/provider-routeros/rest"
)

var (
	endpoint = flag.String("endpoint", "http://127.0.0.1:18080", "RouterOS REST endpoint")
	user     = flag.String("user", "admin", "RouterOS user")
	pass     = flag.String("pass", "", "RouterOS password")
)

func main() {
	flag.Parse()
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// sample is one metric reading, in the shape the receiver would hand to
// mdatagen: a name, a value, and the attributes that distinguish the series.
type sample struct {
	name  string
	value string
	attrs string
}

func run() error {
	c, err := rest.New(*endpoint, rest.WithBasicAuth(*user, *pass))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var out []sample
	emit := func(name, value, attrs string) {
		out = append(out, sample{name, value, attrs})
	}

	// Resource attributes first, because without both service.name and
	// service.instance.id the Prometheus exporter drops the whole resource and
	// takes model, serial and version with it — silently.
	instance := *endpoint
	if res, err := c.Get(ctx, routeros.SystemResourcePath); err != nil {
		warn(routeros.SystemResourcePath, err)
	} else {
		v := routeros.DecodeSystemResource(res)
		emit("resource:routeros.os.version", v.Version, "")
		// Two readings the hand-rolled receiver never took, because reaching
		// them meant writing another parser. Here they are already typed.
		emit("routeros.uptime", v.Uptime.String(), "")
		emit("routeros.cpu.load", fmt.Sprint(v.CpuLoad), "")
		emit("routeros.memory.usage", fmt.Sprint(v.TotalMemory-v.FreeMemory), "state=used")
		emit("routeros.memory.usage", fmt.Sprint(v.FreeMemory), "state=free")
	}

	if ifs, err := c.List(ctx, routeros.InterfacePath); err != nil {
		warn(routeros.InterfacePath, err)
	} else {
		for _, r := range ifs {
			v := routeros.DecodeInterface(r)
			emit("routeros.interface.io", fmt.Sprint(v.RxByte), attr(v.Name, "receive"))
			emit("routeros.interface.io", fmt.Sprint(v.TxByte), attr(v.Name, "transmit"))
			emit("routeros.interface.up", boolMetric(v.Running), "interface="+v.Name)
		}
	}

	if ss, err := c.List(ctx, routeros.RoutingBgpSessionPath); err != nil {
		warn(routeros.RoutingBgpSessionPath, err)
	} else {
		for _, r := range ss {
			v := routeros.DecodeRoutingBgpSession(r)
			a := "session=" + v.Name + ",remote_as=" + v.RemoteAs
			emit("routeros.bgp.session.prefix_count", fmt.Sprint(v.PrefixCount), a)
			emit("routeros.bgp.session.up", boolMetric(v.Established), a)
		}
	}

	// The sensor table: name identifies the reading, type carries the unit.
	if hs, err := c.List(ctx, routeros.SystemHealthPath); err != nil {
		warn(routeros.SystemHealthPath, err)
	} else {
		for _, r := range hs {
			v := routeros.DecodeSystemHealth(r)
			if v.Type != "C" {
				continue
			}
			// Value stays a string all the way here on purpose: the router
			// states this field's vocabulary as ok/fail/idle/no-input/
			// not-present, so a health row holds either a number or a state
			// word. Parsing it as a float unconditionally, which the receiver
			// does, turns "no-input" into a reported 0 °C.
			emit("routeros.hw.temperature", v.Value, "sensor="+v.Name)
		}
	}

	// Rules are addressed by comment, the same string the Crossplane CR uses
	// as its external-name, which is what makes the two halves line up.
	if fs, err := c.List(ctx, routeros.IpFirewallFilterPath); err != nil {
		warn(routeros.IpFirewallFilterPath, err)
	} else {
		for _, r := range fs {
			v := routeros.DecodeIpFirewallFilter(r)
			if v.Comment == "" {
				continue
			}
			emit("routeros.firewall.rule.io", fmt.Sprint(v.Bytes), "rule="+v.Comment)
			emit("routeros.firewall.rule.packets", fmt.Sprint(v.Packets), "rule="+v.Comment)
		}
	}

	// /system/routerboard is absent on CHR — a virtual router has no board —
	// and absent from the IR for the same reason, so there is no generated
	// type to decode with. Reading it raw is the honest fallback and says
	// exactly what is missing.
	if board, err := c.Get(ctx, "/system/routerboard"); err != nil {
		warn("/system/routerboard", err)
	} else {
		emit("resource:routeros.device.model", board.String("model"), "")
		if sn := board.String("serial-number"); sn != "" {
			emit("resource:routeros.device.serial", sn, "")
			instance = sn
		}
	}
	emit("resource:service.instance.id", instance, "")

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, s := range out {
		fmt.Fprintf(w, "%s\t%s\t%s\n", s.name, s.value, s.attrs)
	}
	fmt.Fprintf(os.Stderr, "\n%d samples\n", len(out))
	return w.Flush()
}

func attr(name, direction string) string {
	return "interface=" + name + ",io.direction=" + direction
}

// boolMetric renders an up/down gauge. The receiver's boolOf returns int64 for
// the same reason: a gauge cannot hold a bool.
func boolMetric(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// warn reports a menu that did not answer. A menu the hardware does not have
// is not a failure worth stopping for, but the distinction between that and a
// real fault is exactly what rest's typed errors exist to preserve.
func warn(path string, err error) {
	var e *rest.Error
	switch {
	case errors.Is(err, rest.ErrAddressRejected):
		fmt.Fprintf(os.Stderr, "skip %s: the router refused the source address\n", path)
	case errors.As(err, &e):
		fmt.Fprintf(os.Stderr, "skip %s: %s\n", path, e)
	default:
		fmt.Fprintf(os.Stderr, "skip %s: %v\n", path, err)
	}
}
