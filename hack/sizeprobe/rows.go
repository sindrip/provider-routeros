package main

import (
	"fmt"

	ros "github.com/sindrip/provider-routeros/schema"
)

// typicalRows is what a large real-world filter looks like: a handful of
// archetypes — established/related accepts, invalid drops, service allows,
// list-based drops, dnat follow-ups, tenant jumps — cycled with varied
// addresses, ports and comments so no two rows compress into each other.
// Field names are the menu's own; only the values are synthetic.
func typicalRows(n int) []map[string]string {
	rows := make([]map[string]string, 0, n)
	for i := 0; len(rows) < n; i++ {
		for _, r := range []map[string]string{
			{
				"chain":            "input",
				"action":           "accept",
				"connection-state": "established,related,untracked",
				"comment":          fmt.Sprintf("accept established,related,untracked (block %d)", i),
			},
			{
				"chain":            "input",
				"action":           "drop",
				"connection-state": "invalid",
				"comment":          fmt.Sprintf("drop invalid (block %d)", i),
			},
			{
				"chain":    "input",
				"action":   "accept",
				"protocol": "icmp",
				"comment":  fmt.Sprintf("accept ICMP (block %d)", i),
			},
			{
				"chain":             "input",
				"action":            "accept",
				"protocol":          "tcp",
				"dst-port":          fmt.Sprintf("22,443,%d", 8000+i%1000),
				"src-address-list":  "mgmt-hosts",
				"in-interface-list": "LAN",
				"comment":           fmt.Sprintf("allow management from LAN segment %d", i),
			},
			{
				"chain":            "forward",
				"action":           "drop",
				"src-address-list": fmt.Sprintf("blacklist-feed-%d", i%16),
				"log":              "true",
				"log-prefix":       fmt.Sprintf("bl-drop-%d", i%16),
				"comment":          fmt.Sprintf("drop blacklisted sources, feed %d", i%16),
			},
			{
				"chain":                "forward",
				"action":               "accept",
				"protocol":             "tcp",
				"dst-address":          fmt.Sprintf("10.0.%d.%d", i/250%250, 10+i%250),
				"dst-port":             fmt.Sprintf("%d", 1024+i%40000),
				"connection-nat-state": "dstnat",
				"in-interface":         "ether1",
				"comment":              fmt.Sprintf("dstnat follow-up for svc-%d", i),
			},
			{
				"chain":       "forward",
				"action":      "jump",
				"jump-target": fmt.Sprintf("tenant-%d", i%8),
				"comment":     fmt.Sprintf("hand off to tenant chain %d", i%8),
			},
		} {
			if len(rows) == n {
				break
			}
			rows = append(rows, r)
		}
	}
	return rows
}

// denseRows sets every writable field the IR knows on every row — the row no
// human writes and the honest worst case for a menu whose rows are stored
// whole. copy-from and place-before are write directives rather than row
// state, so they are skipped; everything else gets a value shaped like its
// probed type.
func denseRows(m ros.Menu, n int) []map[string]string {
	rows := make([]map[string]string, n)
	for i := range rows {
		row := map[string]string{}
		for _, f := range m.Fields {
			if f.Access != ros.Writable || f.Name == "copy-from" || f.Name == "place-before" {
				continue
			}
			row[f.Name] = synthesize(f, i)
		}
		row["comment"] = fmt.Sprintf("dense synthetic rule %d, every field set", i)
		rows[i] = row
	}
	return rows
}

// readOnlyFields is what the device adds to a row the spec never wrote —
// counters and flags on /ip/firewall/filter — for the observed mirror.
func readOnlyFields(m ros.Menu) map[string]string {
	out := map[string]string{}
	for _, f := range m.Fields {
		if f.Access != ros.ReadOnly {
			continue
		}
		out[f.Name] = synthesize(f, 0)
	}
	return out
}

// synthesize is a value shaped like the field's probed type. The router will
// never see it, so only its size has to be right, but keeping the shape
// honest means a reader can eyeball the pinned rows against a real export.
func synthesize(f ros.Field, i int) string {
	if f.Bool {
		return "false"
	}
	if f.Kind == ros.KindEnum && len(f.Values) > 0 {
		return f.Values[i%len(f.Values)]
	}
	switch f.Type {
	case "string":
		return fmt.Sprintf("synthetic-%s-%d", f.Name, i)
	case "integer number":
		return "1234567"
	case "time interval":
		return "1d2h3m4s"
	case "IP address range":
		return fmt.Sprintf("10.%d.%d.0/24", i/250%250, i%250)
	case "MAC address":
		return "AA:BB:CC:DD:EE:0F"
	case "max 15 times": // port lists
		return "80,443,8080-8090"
	case "integer number|time interval":
		return "100/1m"
	default:
		if len(f.Values) > 0 {
			return f.Values[i%len(f.Values)]
		}
		return fmt.Sprintf("value-%d", i)
	}
}
