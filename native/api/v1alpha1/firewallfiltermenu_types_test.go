package v1alpha1

import (
	"reflect"
	"testing"
)

func TestFirewallFilterRuleFieldsPreservesPresence(t *testing.T) {
	empty := ""
	chain := "forward"
	disabled := false
	rule := FirewallFilterRule{
		Chain:    &chain,
		Comment:  &empty,
		Disabled: &disabled,
	}

	want := map[string]string{
		"chain":    "forward",
		"comment":  "",
		"disabled": "false",
	}
	if got := rule.Fields(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Fields() = %#v, want %#v", got, want)
	}
	if _, present := rule.Fields()["log"]; present {
		t.Fatal("omitted log unexpectedly appeared in REST fields")
	}
}

func TestFirewallFilterRuleFieldsUsesRouterOSNames(t *testing.T) {
	srcMAC := "00:11:22:33:44:55"
	tcpMSS := "1300-65535"
	tlsHost := "example.com"
	rule := FirewallFilterRule{SrcMACAddress: &srcMAC, TCPMSS: &tcpMSS, TLSHost: &tlsHost}

	got := rule.Fields()
	for key, want := range map[string]string{
		"src-mac-address": srcMAC,
		"tcp-mss":         tcpMSS,
		"tls-host":        tlsHost,
	} {
		if got[key] != want {
			t.Errorf("Fields()[%q] = %q, want %q", key, got[key], want)
		}
	}
}
