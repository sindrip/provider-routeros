package main

import (
	"context"
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

	gets := map[string]string{
		"/rest/ip/address":  `[{"address": "10.0.0.1/24"}]`,
		"/rest/ip/settings": `{"arp-timeout": "30s"}`,
	}

	completions := map[string][]map[string]string{
		"/ip/address/set disabled=":        {{"completion": "no", "show": "true"}, {"completion": "yes", "show": "true"}},
		"/ip/address/set interface=":       {{"completion": "ether1", "show": "true"}, {"completion": "<value>"}},
		"/ip/address/set address=":         {{"completion": "<value>"}},
		"/ip/address/print where ":         {{"completion": "address", "show": "true"}, {"completion": "disabled", "show": "true"}, {"completion": "dynamic", "show": "true"}, {"completion": "interface", "show": "true"}, {"completion": ".id", "show": "true"}},
		"/ip/address/print where dynamic=": {{"completion": "no", "show": "true"}, {"completion": "yes", "show": "true"}},
		"/ip/settings/set arp-timeout=":    {{"completion": "<value>"}},
		"/ip/settings/get ":                {{"completion": "arp-timeout", "show": "true"}, {"completion": "value-name", "show": "true"}},
	}

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

		if r.URL.Path == "/rest/ip/sick" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": 500, "message": "internal error"}`))

			return
		}

		body, ok := gets[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error": 400, "message": "no such command"}`))

			return
		}

		w.Write([]byte(body))
	}))
}

func TestInventory(t *testing.T) {
	srv := fakeRouter(t)
	defer srv.Close()

	got, err := inventory(context.Background(), routeros.New(srv.URL, "admin", ""))
	if err != nil {
		t.Fatal(err)
	}

	want := []pathInfo{
		{Path: "/ip", NodeType: "dir", Get: "400", GetError: "no such command", Class: "none"},
		{Path: "/ip/address", NodeType: "dir", Commands: []string{"add", "move", "print", "set"}, Get: "rows", Class: "rows"},
		{Path: "/ip/odd", NodeType: "dir", Commands: []string{"print"}, Get: "400", GetError: "no such command", Class: "unknown"},
		{Path: "/ip/settings", NodeType: "dir", Commands: []string{"print", "set"}, Get: "record", Class: "singleton"},
		{Path: "/ip/sick", NodeType: "dir", Get: "500", GetError: "internal error", Class: "unknown"},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d paths, want %d: %+v", len(got), len(want), got)
	}

	for i, w := range want {
		g := got[i]

		if g.Path != w.Path || g.NodeType != w.NodeType || g.Get != w.Get || g.GetError != w.GetError || g.Class != w.Class {
			t.Errorf("path %d: got %+v, want %+v", i, g, w)
		}

		if len(g.Commands) != len(w.Commands) {
			t.Errorf("%s commands: got %v, want %v", w.Path, g.Commands, w.Commands)

			continue
		}

		for j := range w.Commands {
			if g.Commands[j] != w.Commands[j] {
				t.Errorf("%s commands: got %v, want %v", w.Path, g.Commands, w.Commands)
				break
			}
		}
	}
}

func TestFields(t *testing.T) {
	srv := fakeRouter(t)
	defer srv.Close()

	c := routeros.New(srv.URL, "admin", "")

	paths, err := inventory(t.Context(), c)
	if err != nil {
		t.Fatal(err)
	}

	got, err := fields(t.Context(), c, paths)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 2 {
		t.Fatalf("got %d menus: %+v", len(got), got)
	}

	addr := got[0]
	if addr.Path != "/ip/address" || !slices.Equal(addr.Args["add"], []string{"address", "interface"}) || len(addr.Args["move"]) != 0 {
		t.Errorf("address = %+v", addr)
	}

	settings := got[1]
	if settings.Path != "/ip/settings" || !slices.Equal(settings.Args["set"], []string{"arp-timeout"}) {
		t.Errorf("settings = %+v", settings)
	}
}

func TestTypes(t *testing.T) {
	srv := fakeRouter(t)
	defer srv.Close()

	c := routeros.New(srv.URL, "admin", "")

	paths, err := inventory(t.Context(), c)
	if err != nil {
		t.Fatal(err)
	}

	menus, err := fields(t.Context(), c, paths)
	if err != nil {
		t.Fatal(err)
	}

	got, err := types(t.Context(), c, paths, menus)
	if err != nil {
		t.Fatal(err)
	}

	byKey := map[string]*argType{}
	for _, a := range got {
		byKey[a.Path+" "+a.Arg] = a
	}

	checks := []struct{ key, access, kind, values string }{
		{"/ip/address disabled", "writable", "enum", "no,yes"},
		{"/ip/address interface", "writable", "open-enum", "ether1"},
		{"/ip/address address", "writable", "scalar", ""},
		{"/ip/address dynamic", "read-only", "enum", "no,yes"},
		{"/ip/settings arp-timeout", "writable", "scalar", ""},
	}

	for _, want := range checks {
		g := byKey[want.key]
		if g == nil {
			t.Errorf("%s: missing", want.key)
			continue
		}

		if g.Access != want.access || g.Kind != want.kind || strings.Join(g.Values, ",") != want.values {
			t.Errorf("%s: got %s %s %v", want.key, g.Access, g.Kind, g.Values)
		}

		if g.Kind == "scalar" && len(g.Syntax) == 0 {
			t.Errorf("%s: scalar without syntax", want.key)
		}
	}

	if a := byKey["/ip/address .id"]; a != nil {
		t.Errorf(".id should be skipped, got %+v", a)
	}
}
