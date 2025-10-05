package service

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"hyprorbit/internal/hyprctl"
	"hyprorbit/internal/regex"
	"hyprorbit/internal/runtime"
)

func TestResolveWindowSelection_RegexFieldMatching(t *testing.T) {
	ctx := context.Background()
	hypr := &fakeHyprClient{
		workspace: hyprctl.Workspace{Name: "alpha"},
		clients: []hyprctl.ClientInfo{
			{Title: "Inbox – Thunderbird", Class: "thunderbird", Workspace: hyprctl.WorkspaceHandle{Name: "alpha"}, Tags: hyprctl.HyprTags{"mail"}},
			{Title: "Terminal", Class: "foot", Workspace: hyprctl.WorkspaceHandle{Name: "alpha"}},
		},
	}

	d := &Dispatcher{}

	clients, err := d.resolveWindowSelection(ctx, hypr, "class:thunderbird")
	if err != nil {
		t.Fatalf("resolveWindowSelection returned error: %v", err)
	}
	if len(clients) != 1 {
		t.Fatalf("expected 1 client, got %d", len(clients))
	}
	if clients[0].Class != "thunderbird" {
		t.Fatalf("expected thunderbird class, got %q", clients[0].Class)
	}

	clients, err = d.resolveWindowSelection(ctx, hypr, "title:Thunderbird")
	if err != nil {
		t.Fatalf("resolveWindowSelection returned error: %v", err)
	}
	if len(clients) != 1 {
		t.Fatalf("expected 1 client, got %d", len(clients))
	}
	if clients[0].Title == "" || !regexp.MustCompile("Thunderbird").MatchString(clients[0].Title) {
		t.Fatalf("unexpected title match %q", clients[0].Title)
	}

	clients, err = d.resolveWindowSelection(ctx, hypr, "tag:mail")
	if err != nil {
		t.Fatalf("resolveWindowSelection returned error: %v", err)
	}
	if len(clients) != 1 {
		t.Fatalf("expected 1 client for tag match, got %d", len(clients))
	}

	clients, err = d.resolveWindowSelection(ctx, hypr, "regex:mail")
	if err != nil {
		t.Fatalf("resolveWindowSelection returned error: %v", err)
	}
	if len(clients) != 0 {
		t.Fatalf("expected 0 clients for unqualified tag match, got %d", len(clients))
	}

	clients, err = d.resolveWindowSelection(ctx, hypr, "regex:class:thunderbird")
	if err != nil {
		t.Fatalf("legacy regex prefix should still work, got error: %v", err)
	}
	if len(clients) != 1 {
		t.Fatalf("expected 1 client for legacy syntax, got %d", len(clients))
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

var _ runtime.HyprctlClient = (*fakeHyprClient)(nil)

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
