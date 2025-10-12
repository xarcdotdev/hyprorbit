package service

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"testing"

	"hyprorbit/internal/hyprctl"
	"hyprorbit/internal/ipc"
	"hyprorbit/internal/orbit"
	"hyprorbit/internal/regex"
	"hyprorbit/internal/runtime"
	"hyprorbit/internal/window"
)

func TestResolveWindowSelection_RegexFieldMatching(t *testing.T) {
	ctx := context.Background()
	hypr := &fakeHyprClient{
		workspace: hyprctl.Workspace{Name: "comm-alpha"},
		clients: []hyprctl.ClientInfo{
			{Title: "Inbox – Thunderbird", Class: "thunderbird", Workspace: hyprctl.WorkspaceHandle{Name: "comm-alpha"}, Tags: hyprctl.HyprTags{"mail"}},
			{Title: "Terminal", Class: "foot", Workspace: hyprctl.WorkspaceHandle{Name: "code-alpha"}},
			{Title: "Media Player", Class: "mpv", Workspace: hyprctl.WorkspaceHandle{Name: "media-beta"}},
		},
	}

	orbitProv := fakeOrbitProvider{name: "alpha"}

	clients, err := window.ResolveSelection(ctx, hypr, nil, "class:thunderbird", false)
	if err != nil {
		t.Fatalf("ResolveSelection returned error: %v", err)
	}
	if len(clients) != 1 {
		t.Fatalf("expected 1 client, got %d", len(clients))
	}
	if clients[0].Class != "thunderbird" {
		t.Fatalf("expected thunderbird class, got %q", clients[0].Class)
	}

	clients, err = window.ResolveSelection(ctx, hypr, nil, "title:Thunderbird", false)
	if err != nil {
		t.Fatalf("ResolveSelection returned error: %v", err)
	}
	if len(clients) != 1 {
		t.Fatalf("expected 1 client, got %d", len(clients))
	}
	if clients[0].Title == "" || !regexp.MustCompile("Thunderbird").MatchString(clients[0].Title) {
		t.Fatalf("unexpected title match %q", clients[0].Title)
	}

	clients, err = window.ResolveSelection(ctx, hypr, nil, "tag:mail", false)
	if err != nil {
		t.Fatalf("ResolveSelection returned error: %v", err)
	}
	if len(clients) != 1 {
		t.Fatalf("expected 1 client for tag match, got %d", len(clients))
	}

	clients, err = window.ResolveSelection(ctx, hypr, nil, "regex:mail", false)
	if err != nil {
		t.Fatalf("ResolveSelection returned error: %v", err)
	}
	if len(clients) != 0 {
		t.Fatalf("expected 0 clients for unqualified tag match, got %d", len(clients))
	}

	clients, err = window.ResolveSelection(ctx, hypr, nil, "regex:class:thunderbird", false)
	if err != nil {
		t.Fatalf("legacy regex prefix should still work, got error: %v", err)
	}
	if len(clients) != 1 {
		t.Fatalf("expected 1 client for legacy syntax, got %d", len(clients))
	}

	clients, err = window.ResolveSelection(ctx, hypr, orbitProv, "orbit:class:foot", false)
	if err != nil {
		t.Fatalf("orbit scoped selection returned error: %v", err)
	}
	if len(clients) != 1 || clients[0].Class != "foot" {
		t.Fatalf("expected orbit scope to return foot client, got %+v", clients)
	}

	clients, err = window.ResolveSelection(ctx, hypr, orbitProv, "global:class:mpv", false)
	if err != nil {
		t.Fatalf("global scoped selection returned error: %v", err)
	}
	if len(clients) != 1 || clients[0].Class != "mpv" {
		t.Fatalf("expected global scope to return mpv client, got %+v", clients)
	}
}

func TestHandleWindowMoveList(t *testing.T) {
	ctx := context.Background()
	hypr := &fakeHyprClient{
		clients: []hyprctl.ClientInfo{
			{Address: "0xabc", Class: "foot", Title: " Terminal ", Workspace: hyprctl.WorkspaceHandle{Name: "code-alpha"}},
			{Address: "0xdef", Class: "firefox", Title: "Firefox", Workspace: hyprctl.WorkspaceHandle{Name: "web-beta"}},
			{Address: "0xghi", Class: "spotify", Title: "", Workspace: hyprctl.WorkspaceHandle{Name: "special:music"}},
		},
	}

	d := &Dispatcher{state: &DaemonState{hyprctl: hypr}}
	req := ipc.NewRequest("window", "list")

	resp, _, err := d.handleWindow(ctx, req)
	if err != nil {
		t.Fatalf("handleWindow returned error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success response, got %+v", resp)
	}
	if len(resp.Data) == 0 {
		t.Fatalf("expected response data, got none")
	}

	var entries []windowMoveListEntry
	if err := json.Unmarshal(resp.Data, &entries); err != nil {
		t.Fatalf("decode entries: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	first := entries[0]
	if first.Workspace != "code-alpha" || first.Module != "code" || first.Orbit != "alpha" {
		t.Fatalf("first entry mismatch: %+v", first)
	}
	if first.Title != "Terminal" {
		t.Fatalf("expected sanitized title, got %q", first.Title)
	}

	second := entries[1]
	if second.Workspace != "special:music" {
		t.Fatalf("expected second workspace special:music, got %q", second.Workspace)
	}
	if second.Module != "" || second.Orbit != "" {
		t.Fatalf("expected no module/orbit for special workspace, got %+v", second)
	}

	third := entries[2]
	if third.Workspace != "web-beta" || third.Module != "web" || third.Orbit != "beta" {
		t.Fatalf("third entry mismatch: %+v", third)
	}
	if third.Address != "0xdef" {
		t.Fatalf("expected address 0xdef, got %q", third.Address)
	}
}

type fakeHyprClient struct {
	workspace hyprctl.Workspace
	clients   []hyprctl.ClientInfo
	window    *hyprctl.Window
}

func (f *fakeHyprClient) Dispatch(context.Context, ...string) error {
	return errors.New("not implemented")
}
func (f *fakeHyprClient) Clients(context.Context) ([]byte, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeHyprClient) DecodeClients(_ context.Context, out any) error {
	ptr, ok := out.(*[]hyprctl.ClientInfo)
	if !ok {
		return errors.New("unexpected type")
	}
	*ptr = append([]hyprctl.ClientInfo(nil), f.clients...)
	return nil
}
func (f *fakeHyprClient) InvalidateClients() {}
func (f *fakeHyprClient) Workspaces(context.Context) ([]hyprctl.Workspace, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeHyprClient) ActiveWorkspace(context.Context) (*hyprctl.Workspace, error) {
	return &f.workspace, nil
}
func (f *fakeHyprClient) ActiveWindow(context.Context) (*hyprctl.Window, error) { return f.window, nil }
func (f *fakeHyprClient) Monitors(context.Context) ([]hyprctl.Monitor, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeHyprClient) Batch(context.Context, ...[]string) ([]hyprctl.BatchResult, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeHyprClient) BatchDispatch(context.Context, ...[]string) ([]hyprctl.BatchResult, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeHyprClient) SwitchWorkspace(context.Context, string) error {
	return errors.New("not implemented")
}
func (f *fakeHyprClient) FocusWindow(context.Context, string) error {
	return errors.New("not implemented")
}
func (f *fakeHyprClient) MoveToWorkspace(context.Context, string, string) error {
	return errors.New("not implemented")
}

func (f *fakeHyprClient) MoveToWorkspaceFollow(context.Context, string, string) error {
	return errors.New("not implemented")
}

func (f *fakeHyprClient) MoveToWorkspaceSilent(context.Context, string, string) error {
	return errors.New("not implemented")
}

var _ runtime.HyprctlClient = (*fakeHyprClient)(nil)

type fakeOrbitProvider struct {
	name string
}

func (f fakeOrbitProvider) ActiveOrbit(context.Context) (*orbit.Record, error) {
	return &orbit.Record{Name: f.name}, nil
}

func TestParseWindowSelector(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		pattern string
		field   regex.Field
		ok      bool
	}{
		{name: "class matcher", input: "class:foo", pattern: "foo", field: regex.FieldClass, ok: true},
		{name: "title case insensitive", input: "TITLE:bar", pattern: "bar", field: regex.FieldTitle, ok: true},
		{name: "initial title", input: "initialTitle:^win$", pattern: "^win$", field: regex.FieldInitialTitle, ok: true},
		{name: "initial class legacy prefix", input: "regex:initialClass:vim", pattern: "vim", field: regex.FieldInitialClass, ok: true},
		{name: "tag uppercase", input: "TAG:prod", pattern: "prod", field: regex.FieldTag, ok: true},
		{name: "legacy any", input: "regex:firefox", pattern: "firefox", field: regex.FieldAny, ok: true},
		{name: "unknown classifier", input: "something:else", pattern: "else", field: regex.FieldAny, ok: true},
		{name: "missing classifier", input: "firefox", ok: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			selector, ok, err := regex.ParseWindowSelector(tc.input)
			if ok != tc.ok {
				t.Fatalf("expected ok=%v, got %v", tc.ok, ok)
			}
			if !ok {
				if err != nil {
					t.Fatalf("unexpected error for %q: %v", tc.input, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseWindowSelector(%q) returned error: %v", tc.input, err)
			}
			if selector.Pattern != tc.pattern {
				t.Fatalf("expected pattern %q, got %q", tc.pattern, selector.Pattern)
			}
			if selector.Field != tc.field {
				t.Fatalf("expected field %v, got %v", tc.field, selector.Field)
			}
		})
	}
}

func TestParseReferenceScopes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		input   string
		scope   window.Scope
		field   regex.Field
		pattern string
		ok      bool
	}{
		{name: "default workspace", input: "class:foo", scope: window.ScopeWorkspace, field: regex.FieldClass, pattern: "foo", ok: true},
		{name: "orbit scope", input: "orbit:class:bar", scope: window.ScopeOrbit, field: regex.FieldClass, pattern: "bar", ok: true},
		{name: "global scope", input: "global:title:baz", scope: window.ScopeGlobal, field: regex.FieldTitle, pattern: "baz", ok: true},
		{name: "workspace explicit", input: "workspace:regex:qux", scope: window.ScopeWorkspace, field: regex.FieldAny, pattern: "qux", ok: true},
		{name: "unsupported", input: "orbit:foo", ok: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reference, ok, err := window.ParseReference(tc.input)
			if tc.ok != ok {
				t.Fatalf("expected ok=%v, got %v (err=%v)", tc.ok, ok, err)
			}
			if !tc.ok {
				return
			}
			if err != nil {
				t.Fatalf("ParseReference(%q) returned error: %v", tc.input, err)
			}
			if reference.Scope != tc.scope {
				t.Fatalf("expected scope %v, got %v", tc.scope, reference.Scope)
			}
			if reference.Selector.Field != tc.field {
				t.Fatalf("expected field %v, got %v", tc.field, reference.Selector.Field)
			}
			if reference.Selector.Pattern != tc.pattern {
				t.Fatalf("expected pattern %q, got %q", tc.pattern, reference.Selector.Pattern)
			}
		})
	}
}
