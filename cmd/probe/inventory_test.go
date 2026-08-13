package main

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"slices"
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
		"ip,address,move":   {},
		"ip,address,print":  {},
		"ip,address,set":    {},
		"ip,settings,print": {},
		"ip,settings,set": {
			{"type": "child", "name": "arp-timeout", "node-type": "arg"},
		},
	}

	gets := map[string]string{
		"/rest/ip/address":  `[{"address": "10.0.0.1/24"}]`,
		"/rest/ip/settings": `{"arp-timeout": "30s"}`,
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/rest/console/inspect" {
			var args map[string]string
			if err := json.UnmarshalRead(r.Body, &args); err != nil {
				t.Errorf("inspect body: %v", err)
			}

			rows, ok := children[args["path"]]
			if !ok {
				t.Errorf("unexpected inspect path %q", args["path"])
			}

			_ = json.MarshalWrite(w, rows)

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
