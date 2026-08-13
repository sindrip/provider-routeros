package main

import (
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/sindrip/routeros"
)

// /ip has no commands and GETs 400 (none); /ip/address is rows;
// /ip/settings a singleton; /ip/odd has print but GETs 400
// (contradiction); /ip/sick GETs 500 (device error, never "none").
func fakeRouter(t *testing.T) *httptest.Server {
	t.Helper()

	children := map[string][]map[string]string{
		"": {
			{"type": "self", "name": "/", "node-type": "path"},
			{"type": "child", "name": "ip", "node-type": "dir"},
			{"type": "child", "name": "beep", "node-type": "cmd"},
		},
		"ip": {
			{"type": "child", "name": "address", "node-type": "dir"},
			{"type": "child", "name": "settings", "node-type": "dir"},
			{"type": "child", "name": "odd", "node-type": "dir"},
			{"type": "child", "name": "sick", "node-type": "dir"},
		},
		"ip,address": {
			{"type": "child", "name": "add", "node-type": "cmd"},
			{"type": "child", "name": "move", "node-type": "cmd"},
			{"type": "child", "name": "print", "node-type": "cmd"},
			{"type": "child", "name": "set", "node-type": "cmd"},
		},
		"ip,settings": {
			{"type": "child", "name": "print", "node-type": "cmd"},
			{"type": "child", "name": "set", "node-type": "cmd"},
		},
		"ip,odd": {
			{"type": "child", "name": "print", "node-type": "cmd"},
		},
		"ip,sick": {},
		"ip,address,add": {
			{"type": "self", "name": "add", "node-type": "cmd"},
			{"type": "child", "name": "address", "node-type": "arg"},
			{"type": "child", "name": "interface", "node-type": "arg"},
		},
		"ip,address,move":  {},
		"ip,address,print": {},
		"ip,address,set": {
			{"type": "child", "name": "address", "node-type": "arg"},
			{"type": "child", "name": "disabled", "node-type": "arg"},
			{"type": "child", "name": "interface", "node-type": "arg"},
		},
		"ip,settings,print": {},
		"ip,settings,set": {
			{"type": "child", "name": "arp-timeout", "node-type": "arg"},
		},
	}

	completions := map[string][]map[string]string{
		"/ip/address/set disabled=":        {{"completion": "no", "show": "true"}, {"completion": "yes", "show": "true"}, {"completion": "!", "show": "true", "style": "syntax-meta"}},
		"/ip/address/set interface=":       {{"completion": "ether1", "show": "true"}, {"completion": "<value>"}},
		"/ip/address/set address=":         {{"completion": "<value>"}},
		"/ip/address/print where ":         {{"completion": "address", "show": "true"}, {"completion": "disabled", "show": "true"}, {"completion": "dynamic", "show": "true"}, {"completion": "interface", "show": "true"}, {"completion": ".id", "show": "true"}},
		"/ip/address/print where dynamic=": {{"completion": "no", "show": "true"}, {"completion": "yes", "show": "true"}},
		"/ip/settings/set arp-timeout=":    {{"completion": "<value>"}},
		"/ip/settings/get ":                {{"completion": "arp-timeout", "show": "true"}, {"completion": "value-name", "show": "true"}},
	}

	var addressRows []map[string]string

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/rest/console/inspect" {
			var args map[string]string
			if err := json.UnmarshalRead(r.Body, &args); err != nil {
				t.Errorf("inspect body: %v", err)
			}

			switch args["request"] {
			case "child":
				rows, ok := children[args["path"]]
				if !ok {
					t.Errorf("unexpected inspect path %q", args["path"])
				}

				_ = json.MarshalWrite(w, rows)
			case "completion":
				rows, ok := completions[args["input"]]
				if !ok {
					t.Errorf("unexpected completion input %q", args["input"])
				}

				_ = json.MarshalWrite(w, rows)
			case "syntax":
				w.Write([]byte(`[{"depth": 0, "symbol": "address", "symbol-type": "value", "text": "ip address"}]`))
			default:
				t.Errorf("unexpected inspect request %q", args["request"])
			}

			return
		}

		switch r.Method + " " + r.URL.Path {
		case "PUT /rest/ip/address":
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error": 400, "message": "Bad Request", "detail": "missing =address="}`))
		case "PUT /rest/ip/odd", "PUT /rest/ip/sick":
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error": 400, "message": "no such command"}`))
		case "GET /rest/ip/address":
			_ = json.MarshalWrite(w, append([]map[string]string{{"address": "10.0.0.1/24"}}, addressRows...))
		case "GET /rest/ip/settings":
			w.Write([]byte(`{"arp-timeout": "30s"}`))
		case "GET /rest/ip/sick":
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": 500, "message": "internal error"}`))
		default:
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error": 400, "message": "no such command"}`))
		}
	}))
}

func TestDescribe(t *testing.T) {
	srv := fakeRouter(t)
	defer srv.Close()

	menus, err := describe(t.Context(), routeros.New(srv.URL, "admin", ""))
	if err != nil {
		t.Fatal(err)
	}

	byPath := map[string]*menuDesc{}
	for _, m := range menus {
		byPath[m.Path] = m
	}

	verdicts := []struct{ path, class, get, getError string }{
		{"/ip", "none", "400", "no such command"},
		{"/ip/address", "rows", "rows", ""},
		{"/ip/odd", "unknown", "400", "no such command"},
		{"/ip/settings", "singleton", "record", ""},
		{"/ip/sick", "unknown", "500", "internal error"},
	}

	if len(menus) != len(verdicts) {
		t.Fatalf("got %d menus: %+v", len(menus), menus)
	}

	for _, want := range verdicts {
		m := byPath[want.path]
		if m == nil {
			t.Errorf("%s: missing", want.path)
			continue
		}

		if m.Class != want.class || m.Get != want.get || m.GetError != want.getError {
			t.Errorf("%s: got class=%s get=%s err=%q", want.path, m.Class, m.Get, m.GetError)
		}
	}

	addr := byPath["/ip/address"]
	if !slices.Equal(addr.Args["add"], []string{"address", "interface"}) || len(addr.Args["move"]) != 0 {
		t.Errorf("address args = %+v", addr.Args)
	}

	props := map[string]*property{}
	for _, p := range addr.Properties {
		props[p.Arg] = p
	}

	checks := []struct{ arg, access, kind, values string }{
		{"disabled", "writable", "enum", "no,yes"},
		{"interface", "writable", "open-enum", "ether1"},
		{"address", "writable", "scalar", ""},
		{"dynamic", "read-only", "enum", "no,yes"},
	}

	for _, want := range checks {
		p := props[want.arg]
		if p == nil {
			t.Errorf("%s: missing", want.arg)
			continue
		}

		if p.Access != want.access || p.Kind != want.kind || strings.Join(p.Values, ",") != want.values {
			t.Errorf("%s: got %s %s %v", want.arg, p.Access, p.Kind, p.Values)
		}

		if p.Kind != "unknown" && len(p.Syntax) == 0 {
			t.Errorf("%s: no syntax recorded", want.arg)
		}
	}

	if p := props["disabled"]; p != nil && !p.Negatable {
		t.Error("disabled: syntax-meta ! should mark negatable")
	}

	if _, ok := props[".id"]; ok {
		t.Error(".id should be skipped")
	}
}

func TestBehave(t *testing.T) {
	srv := fakeRouter(t)
	defer srv.Close()

	c := routeros.New(srv.URL, "admin", "")

	menus, err := describe(t.Context(), c)
	if err != nil {
		t.Fatal(err)
	}

	got := behaveOne(t.Context(), c, "/ip/address")
	if got.putEmpty != "error" || got.err != "Bad Request: missing =address=" {
		t.Errorf("behaveOne = %+v", got)
	}

	for _, m := range menus {
		if m.Path == "/ip/address" && m.Class != "rows" {
			t.Errorf("class changed: %+v", m)
		}
	}
}

func TestMerge(t *testing.T) {
	ss := []sample{
		{putEmpty: "created", created: map[string]string{".id": "*1", "mtu": "1500", "mac-address": "AA:AA"}, deleted: "ok"},
		{putEmpty: "created", created: map[string]string{".id": "*1", "mtu": "1500", "mac-address": "BB:BB"}, deleted: "ok"},
	}

	b := merge("/interface/bridge", ss)

	if b.PutEmpty != "created" || b.Deleted != "ok" {
		t.Errorf("merge = %+v", b)
	}

	if b.Created["mtu"] != "1500" || b.Created[".id"] != "*1" {
		t.Errorf("stable fields = %v", b.Created)
	}

	if _, ok := b.Created["mac-address"]; ok || !slices.Equal(b.Generated, []string{"mac-address"}) {
		t.Errorf("generated = %v, created = %v", b.Generated, b.Created)
	}

	ss[1].putEmpty = "error"
	if got := merge("/x", ss).PutEmpty; !strings.Contains(got, "disagree") {
		t.Errorf("PutEmpty = %q", got)
	}
}
