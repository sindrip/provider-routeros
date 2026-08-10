package routeros

import (
	"testing"
	"time"
)

// TestDecodeIpAddress runs a record exactly as CHR 7.23.2 returned it through
// the generated decoder. The point is that a caller stops writing
// r["disabled"] == "true" and stops being wrong about it.
func TestDecodeIpAddress(t *testing.T) {
	row := map[string]string{
		".id":              "*1",
		"address":          "10.0.0.1/24",
		"network":          "10.0.0.0",
		"interface":        "ether1",
		"actual-interface": "ether1",
		"disabled":         "false",
		"dynamic":          "true",
		"invalid":          "false",
		"comment":          "managed",
	}
	v := DecodeIpAddress(row)
	if v.ID != "*1" || v.Address != "10.0.0.1/24" || v.Comment != "managed" {
		t.Errorf("scalars decoded wrong: %+v", v)
	}
	if v.Disabled {
		t.Error("Disabled should be false")
	}
	if !v.Dynamic {
		t.Error("Dynamic should be true")
	}
	// slave was absent, which is the third boolean encoding and means unset.
	if v.Slave {
		t.Error("an absent boolean must read as false")
	}
	// "interface" needs no escaping: goName capitalises, and Go keywords are
	// all lower case.
	if v.Interface != "ether1" {
		t.Errorf("Interface = %q", v.Interface)
	}
}

// TestEncodeOmitsReadOnly is the property that stops a caller sending back a
// field the router will reject as an unknown parameter.
func TestEncodeOmitsReadOnly(t *testing.T) {
	v := IpAddress{ID: "*1", Address: "10.0.0.1/24", Dynamic: true, Invalid: true, ActualInterface: "ether1"}
	enc := v.Encode()
	for _, readOnly := range []string{"dynamic", "invalid", "actual-interface", "slave", "vrf", ".id"} {
		if _, present := enc[readOnly]; present {
			t.Errorf("%q is read-only and must not be sent back", readOnly)
		}
	}
	if enc["address"] != "10.0.0.1/24" {
		t.Errorf("address = %q", enc["address"])
	}
	// A bool leaves as the wire spelling, not the console's no/yes.
	if enc["disabled"] != "false" {
		t.Errorf("disabled = %q, want the REST spelling", enc["disabled"])
	}
}

// TestPresentButEmptyIsTrue covers the encoding that gets eBGP backwards when
// a reader treats "" as false.
func TestPresentButEmptyIsTrue(t *testing.T) {
	if !parseBool(map[string]string{"flag": ""}, "flag") {
		t.Error(`a present but empty value is a set flag, not false`)
	}
	if parseBool(map[string]string{}, "flag") {
		t.Error("an absent value is unset")
	}
	if !parseBool(map[string]string{"flag": "yes"}, "flag") {
		t.Error(`the console spelling "yes" must also read as true`)
	}
}

// TestBridgePriorityIsHex checks the field the corpus flagged: the bound is
// stated bare as 0..FFFF but the value reads back 0x-prefixed, so a base-10
// reader silently gets zero.
func TestBridgePriorityIsHex(t *testing.T) {
	if got := parseInt("0x8000"); got != 32768 {
		t.Errorf("parseInt(0x8000) = %d, want 32768", got)
	}
	// And a zero-padded decimal is not octal, which strconv base 0 would
	// quietly assume.
	if got := parseInt("010"); got != 10 {
		t.Errorf("parseInt(010) = %d, want 10", got)
	}
}

func TestDurationRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want time.Duration
	}{
		{"1m20s", 80 * time.Second},
		{"52m34s530ms", 52*time.Minute + 34*time.Second + 530*time.Millisecond},
		{"3d12h12m44s", 3*24*time.Hour + 12*time.Hour + 12*time.Minute + 44*time.Second},
	} {
		if got := parseDuration(tc.in); got != tc.want {
			t.Errorf("parseDuration(%q) = %v, want %v", tc.in, got, tc.want)
		}
		if got := parseDuration(formatDuration(tc.want)); got != tc.want {
			t.Errorf("%v did not survive a round trip through %q", tc.want, formatDuration(tc.want))
		}
	}
}

// TestDecodeSystemResource is the menu that had no fields at all until the
// types came from observation rather than from the console. The record below
// is exactly what CHR 7.23.2 returned.
func TestDecodeSystemResource(t *testing.T) {
	v := DecodeSystemResource(map[string]string{
		"architecture-name":       "arm64",
		"board-name":              "CHR QEMU QEMU Virtual Machine",
		"build-time":              "2026-07-03 09:08:08",
		"cpu-count":               "2",
		"cpu-load":                "0",
		"free-memory":             "796278784",
		"total-memory":            "1073741824",
		"uptime":                  "4h10m29s",
		"version":                 "7.23.2 (stable)",
		"write-sect-since-reboot": "10824",
	})
	if v.Uptime != 4*time.Hour+10*time.Minute+29*time.Second {
		t.Errorf("Uptime = %v", v.Uptime)
	}
	if v.CpuCount != 2 || v.CpuLoad != 0 {
		t.Errorf("cpu: count=%d load=%d", v.CpuCount, v.CpuLoad)
	}
	if v.TotalMemory != 1073741824 || v.FreeMemory != 796278784 {
		t.Errorf("memory: total=%d free=%d", v.TotalMemory, v.FreeMemory)
	}
	// A counter big enough to matter stays exact rather than becoming a float.
	if v.WriteSectSinceReboot != 10824 {
		t.Errorf("WriteSectSinceReboot = %d", v.WriteSectSinceReboot)
	}
	// The version string carries a suffix, so it is not a number however much
	// it looks like one at the start.
	if v.Version != "7.23.2 (stable)" {
		t.Errorf("Version = %q", v.Version)
	}
	if SystemResourcePath != "/system/resource" {
		t.Errorf("path = %q", SystemResourcePath)
	}
}

// TestSystemResourceIsReadOnly checks the other half: the device maintains
// this menu, so there is nothing to send back.
func TestSystemResourceIsReadOnly(t *testing.T) {
	if enc := (SystemResource{CpuLoad: 5}).Encode(); len(enc) != 0 {
		t.Errorf("Encode() = %v, want nothing writable", enc)
	}
}
