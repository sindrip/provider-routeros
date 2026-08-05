// uniqprobe sweeps a disposable RouterOS instance to determine, per
// terraform-routeros resource, whether RouterOS enforces uniqueness on the
// name field. For each candidate it creates an item with a throwaway name,
// then attempts a second create with the same name (varying other required
// fields where possible so only name collides). Verdicts:
//
//	UNIQUE      second create rejected with a duplicate/exists error
//	DUPLICATE   second create succeeded (name is not enforced unique)
//	UNTESTED    first create failed (missing deps/unsupported on CHR)
//	AMBIGUOUS   second create failed with an error not clearly about the name
//
// Created items (and fixtures) are deleted afterwards.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

const base = "http://127.0.0.1:18080/rest"

var client = &http.Client{}

func rest(method, path string, body any) (int, []byte, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		rdr = strings.NewReader(string(b))
	}
	req, err := http.NewRequest(method, base+path, rdr)
	if err != nil {
		return 0, nil, err
	}
	req.SetBasicAuth("admin", "")
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data, nil
}

// synthesize returns a plausible value for a required field, or nil if we
// cannot fabricate one. second toggles variation for the duplicate attempt so
// that only the name collides.
func synthesize(field string, s *schema.Schema, second bool) any {
	switch field {
	case "interface", "master_interface", "interfaces":
		v := any("ether2")
		if s.Type == schema.TypeList || s.Type == schema.TypeSet {
			return "ether2"
		}
		return v
	case "vlan_id", "vlan_ids":
		if second {
			return "43"
		}
		return "42"
	case "address", "local_address":
		if second {
			return "192.0.2.9/30"
		}
		return "192.0.2.1/30"
	case "remote_address":
		if second {
			return "192.0.2.10"
		}
		return "192.0.2.2"
	case "ranges":
		if second {
			return "198.51.100.65-198.51.100.94"
		}
		return "198.51.100.1-198.51.100.30"
	case "connect_to":
		return "192.0.2.100"
	case "user", "password":
		if second {
			return "uqpcred2"
		}
		return "uqpcred"
	case "code":
		if second {
			return "201"
		}
		return "200"
	case "type":
		return "A"
	case "public_key":
		if second {
			return "l3TFRRilOUdSDVcCTVCSj+d7SZ64pnkwJQliMWWi9Wc="
		}
		return "iAey5+1xVYYQMbXzHkqIhK1w6xOl40dqZI95BAgFXQE="
	case "source":
		return ":put 1"
	case "on_event":
		return ":put 1"
	}
	switch s.Type {
	case schema.TypeString:
		return "uqpval"
	case schema.TypeInt:
		if second {
			return "8082"
		}
		return "8081"
	case schema.TypeBool:
		return "yes"
	case schema.TypeList, schema.TypeSet:
		return "uqpval"
	}
	return nil
}

// overrides supplies resource-specific field values the generic synthesizer
// cannot guess: [first, second] per field, empty string meaning omit.
var overrides = map[string]map[string][2]string{
	"routeros_system_user":              {"group": {"read", "read"}, "password": {"uqp-pw-1", "uqp-pw-2"}},
	"routeros_wireguard_peer":           {"interface": {"uqp-fix-wg", "uqp-fix-wg"}},
	"routeros_interface_wireguard_peer": {"interface": {"uqp-fix-wg", "uqp-fix-wg"}},
	"routeros_interface_vxlan":          {"vni": {"1001", "1002"}},
	"routeros_interface_bonding":        {"slaves": {"ether2", "ether3"}},
	"routeros_ipv6_pool":                {"prefix": {"2001:db8:1::/48", "2001:db8:2::/48"}, "prefix_length": {"64", "64"}},
	"routeros_queue_simple":             {"target": {"192.0.2.0/30", "192.0.2.4/30"}},
	"routeros_queue_tree":               {"parent": {"global", "global"}},
	"routeros_queue_type":               {"kind": {"pfifo", "pfifo"}},
	"routeros_tool_netwatch":            {"host": {"192.0.2.1", "192.0.2.2"}},
	"routeros_dns_record":               {"address": {"192.0.2.1", "192.0.2.2"}},
	"routeros_ip_dns_record":            {"address": {"192.0.2.1", "192.0.2.2"}},
	"routeros_ip_dhcp_relay":            {"interface": {"ether2", "ether3"}, "dhcp_server": {"192.0.2.5", "192.0.2.6"}},
	"routeros_interface_eoip":           {"interface": {"", ""}, "tunnel_id": {"33", "34"}},
}

type verdict struct {
	Resource string `json:"resource"`
	Path     string `json:"path"`
	Verdict  string `json:"verdict"`
	Detail   string `json:"detail,omitempty"`
}

func itemID(data []byte) string {
	var m map[string]any
	if json.Unmarshal(data, &m) != nil {
		return ""
	}
	if id, ok := m[".id"].(string); ok {
		return id
	}
	return ""
}

func kebab(s string) string { return strings.ReplaceAll(s, "_", "-") }

func probe(name string, r *schema.Resource) verdict {
	pathS, ok := r.Schema[routeros.MetaResourcePath]
	if !ok {
		return verdict{Resource: name, Verdict: "UNTESTED", Detail: "no ___path___ meta"}
	}
	path, _ := pathS.Default.(string)
	idS := r.Schema[routeros.MetaId]
	if idS == nil {
		return verdict{Resource: name, Path: path, Verdict: "UNTESTED", Detail: "no ___id___ meta"}
	}
	if idt, _ := idS.Default.(int); idt != int(routeros.Id) {
		return verdict{Resource: name, Path: path, Verdict: "SKIP", Detail: "already Name-identified upstream"}
	}
	if _, ok := r.Schema["name"]; !ok {
		return verdict{Resource: name, Path: path, Verdict: "SKIP", Detail: "no name field"}
	}

	// Fields that are Optional in the TF schema but mandatory on the router.
	alwaysSend := map[string]bool{
		"remote_address": true, "connect_to": true, "apn": true,
		"user": true, "interface": true, "vlan_id": true, "code": true,
	}
	mkBody := func(second bool) map[string]any {
		body := map[string]any{"name": "uqp-probe"}
		for f, s := range r.Schema {
			if f == "name" || (!s.Required && !alwaysSend[f]) {
				continue
			}
			if _, ok := r.Schema[f]; !ok {
				continue
			}
			if v := synthesize(f, s, second); v != nil {
				body[kebab(f)] = v
			}
		}
		for f, pair := range overrides[name] {
			v := pair[0]
			if second {
				v = pair[1]
			}
			if v == "" {
				delete(body, kebab(f))
			} else {
				body[kebab(f)] = v
			}
		}
		return body
	}

	var created []string
	defer func() {
		for _, id := range created {
			rest("DELETE", path+"/"+id, nil)
		}
	}()

	code1, data1, err := rest("PUT", path, mkBody(false))
	if err != nil {
		return verdict{Resource: name, Path: path, Verdict: "UNTESTED", Detail: err.Error()}
	}
	if code1 >= 300 {
		return verdict{Resource: name, Path: path, Verdict: "UNTESTED", Detail: fmt.Sprintf("first create: %d %s", code1, trim(data1))}
	}
	if id := itemID(data1); id != "" {
		created = append(created, id)
	}

	code2, data2, err := rest("PUT", path, mkBody(true))
	if err != nil {
		return verdict{Resource: name, Path: path, Verdict: "AMBIGUOUS", Detail: err.Error()}
	}
	if code2 < 300 {
		if id := itemID(data2); id != "" {
			created = append(created, id)
		}
		return verdict{Resource: name, Path: path, Verdict: "DUPLICATE"}
	}
	msg := strings.ToLower(trim(data2))
	if strings.Contains(msg, "name") || strings.Contains(msg, "already") || strings.Contains(msg, "exists") || strings.Contains(msg, "unique") {
		return verdict{Resource: name, Path: path, Verdict: "UNIQUE", Detail: trim(data2)}
	}
	return verdict{Resource: name, Path: path, Verdict: "AMBIGUOUS", Detail: fmt.Sprintf("%d %s", code2, trim(data2))}
}

func trim(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 160 {
		s = s[:160]
	}
	return s
}

func main() {
	// Fixtures other resources commonly reference. ether2 exists only if the
	// VM has a second NIC; a bridge named ether2 stands in otherwise.
	rest("PUT", "/interface/bridge", map[string]any{"name": "uqp-fix-bridge"})
	rest("PUT", "/interface/wireguard", map[string]any{"name": "uqp-fix-wg"})
	cleanupFixture := func(path, name string) {
		if _, data, err := rest("GET", path+"?name="+name, nil); err == nil {
			var items []map[string]any
			if json.Unmarshal(data, &items) == nil {
				for _, it := range items {
					if id, ok := it[".id"].(string); ok {
						rest("DELETE", path+"/"+id, nil)
					}
				}
			}
		}
	}
	defer cleanupFixture("/interface/bridge", "uqp-fix-bridge")
	defer cleanupFixture("/interface/wireguard", "uqp-fix-wg")

	p := routeros.Provider()
	names := make([]string, 0, len(p.ResourcesMap))
	for n := range p.ResourcesMap {
		names = append(names, n)
	}
	sort.Strings(names)

	var out []verdict
	for _, n := range names {
		v := probe(n, p.ResourcesMap[n])
		if v.Verdict == "SKIP" {
			continue
		}
		out = append(out, v)
		fmt.Fprintf(os.Stderr, "%-55s %s\n", n, v.Verdict)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}
