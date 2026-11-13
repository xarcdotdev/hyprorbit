package hyprctl

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// PATH contains only the fake executable. Tests cannot reach a real compositor.
func fakeHyprctl(t *testing.T) func() []string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$CALL_LOG"
if [ "$1" = eval ]; then
 if [ "$PROBE_HANG" = 1 ]; then while :; do :; done; fi
 printf '%s' "$PROBE"
 exit "${PROBE_EXIT:-0}"
fi
if [ "$HANG" = 1 ]; then while :; do :; done; fi
printf '%s' "$REPLY"
printf '%s' "$STDERR" >&2
exit "${REPLY_EXIT:-0}"
`
	if err := os.WriteFile(filepath.Join(dir, "hyprctl"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("CALL_LOG", log)
	for _, key := range []string{"PROBE_EXIT", "REPLY_EXIT", "STDERR", "HANG", "PROBE_HANG"} {
		t.Setenv(key, "")
	}
	t.Setenv("PROBE", "ok")
	t.Setenv("REPLY", "ok")
	return func() []string {
		data, err := os.ReadFile(log)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			t.Fatal(err)
		}
		return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	}
}

func TestDispatchTranslations(t *testing.T) {
	cases := []struct{ name, selector, want string }{
		{"focusmonitor", "eDP-1", `hl.dsp.focus({monitor="eDP-1"})`},
		{"focusmonitor", "name:DP-1", `hl.dsp.focus({monitor="name:DP-1"})`},
		{"workspace", "name:work", `hl.dsp.focus({workspace="name:work"})`},
		{"focuswindow", "address:0x123", `hl.dsp.focus({window="address:0x123"})`},
		{"movetoworkspace", "name:work", `hl.dsp.window.move({workspace="name:work",follow=true})`},
		{"movetoworkspacesilent", "name:work", `hl.dsp.window.move({workspace="name:work",follow=false})`},
		{"togglefloating", "address:0x123", `hl.dsp.window.float({action="toggle",window="address:0x123"})`},
	}
	for _, tc := range cases {
		for _, lua := range []bool{false, true} {
			t.Run(tc.name+tc.selector+map[bool]string{false: "legacy", true: "lua"}[lua], func(t *testing.T) {
				calls := fakeHyprctl(t)
				if !lua {
					t.Setenv("PROBE", "unknown request")
				}
				c := NewClient(Options{})
				if err := c.Dispatch(context.Background(), tc.name, tc.selector); err != nil {
					t.Fatal(err)
				}
				want := "dispatch " + tc.name + " " + tc.selector
				if lua {
					want = "dispatch " + tc.want
				}
				if got := calls(); !reflect.DeepEqual(got, []string{"eval return nil", want}) {
					t.Fatalf("calls: %q", got)
				}
			})
		}
	}
}

func TestMoveWithWindowSelector(t *testing.T) {
	cases := []struct {
		name string
		lua  bool
		want string
	}{
		{"movetoworkspacesilent", false, "dispatch movetoworkspacesilent name:work,address:0x1"},
		{"movetoworkspace", false, "dispatch movetoworkspace name:work,address:0x1"},
		{"movetoworkspacesilent", true, `dispatch hl.dsp.window.move({workspace="name:work",follow=false,window="address:0x1"})`},
		{"movetoworkspace", true, `dispatch hl.dsp.window.move({workspace="name:work",follow=true,window="address:0x1"})`},
	}
	for _, tc := range cases {
		t.Run(tc.name+map[bool]string{false: "legacy", true: "lua"}[tc.lua], func(t *testing.T) {
			calls := fakeHyprctl(t)
			if !tc.lua {
				t.Setenv("PROBE", "unknown request")
			}
			if err := NewClient(Options{}).Dispatch(context.Background(), tc.name, "name:work", "address:0x1"); err != nil {
				t.Fatal(err)
			}
			if got := calls(); !reflect.DeepEqual(got, []string{"eval return nil", tc.want}) {
				t.Fatalf("calls: %q", got)
			}
		})
	}
	for _, bad := range []string{"", "address:0x1,x", "a\x00"} {
		if _, err := serializeDispatch(luaParser, []string{"movetoworkspace", "name:work", bad}); err == nil {
			t.Fatalf("expected error for window %q", bad)
		}
	}
}

func TestParserDetection(t *testing.T) {
	for _, tc := range []struct {
		probe, exit string
		success     bool
	}{
		{" ok \n", "0", true}, {"unknown request", "0", true},
		{"eval is only supported with the lua config manager", "0", true},
		{"", "0", false}, {"ok\nwarning", "0", false}, {"Unknown request", "0", false},
		{"ok", "7", false}, {"unknown request", "7", false},
	} {
		t.Run(tc.probe+tc.exit, func(t *testing.T) {
			calls := fakeHyprctl(t)
			t.Setenv("PROBE", tc.probe)
			t.Setenv("PROBE_EXIT", tc.exit)
			err := NewClient(Options{}).Dispatch(context.Background(), "workspace", "1")
			if (err == nil) != tc.success {
				t.Fatalf("error: %v", err)
			}
			n := 1
			if tc.success {
				n = 2
			}
			if len(calls()) != n {
				t.Fatalf("calls: %q", calls())
			}
		})
	}
}

func TestParserNotCached(t *testing.T) {
	calls := fakeHyprctl(t)
	c := NewClient(Options{})
	if err := c.Dispatch(nil, "workspace", "1"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROBE", "unknown request")
	if err := c.Dispatch(nil, "workspace", "2"); err != nil {
		t.Fatal(err)
	}
	if got := calls(); len(got) != 4 || got[2] != "eval return nil" || got[3] != "dispatch workspace 2" {
		t.Fatal(got)
	}
}

func TestLuaEscapingAndSharedSerializer(t *testing.T) {
	calls := fakeHyprctl(t)
	selector := "name:a\"\\\n\r\t\x00\x1f\x7f;[]9é"
	escaped := `"name:a\"\\\010\013\009\000\031\127\059\091\0939é"`
	if got := luaString(selector); got != escaped {
		t.Fatalf("got %q want %q", got, escaped)
	}
	c := NewClient(Options{})
	if err := c.Dispatch(nil, "workspace", selector); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Batch(nil, []string{"dispatch", "workspace", selector}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.BatchDispatch(nil, []string{"workspace", selector}); err != nil {
		t.Fatal(err)
	}
	got := calls()
	want := "dispatch hl.dsp.focus({workspace=" + escaped + "})"
	if len(got) != 6 || got[1] != want || got[3] != "--batch "+want || got[5] != got[3] {
		t.Fatalf("calls: %q", got)
	}
}

func TestBatchPreflight(t *testing.T) {
	for _, tc := range []struct {
		probe string
		bad   []string
	}{
		{"ok", []string{"dispatch", "killworkspace", "name:1"}},
		{"ok", []string{"keyword", "general:gaps_in", "2"}},
		{"ok", []string{"dispatch", "workspace"}},
		{"ok", []string{"dispatch", "movetoworkspace", "1,address:0x1"}},
		{"unknown request", []string{"dispatch", "workspace", "x;dispatch exit"}},
		{"unknown request", []string{"dispatch", "workspace", "x[y]"}},
		{"unknown request", []string{"dispatch", "workspace", "x\ny"}},
	} {
		t.Run(strings.Join(tc.bad, " "), func(t *testing.T) {
			calls := fakeHyprctl(t)
			t.Setenv("PROBE", tc.probe)
			_, err := NewClient(Options{}).Batch(nil, []string{"dispatch", "workspace", "1"}, tc.bad)
			if err == nil || len(calls()) != 1 {
				t.Fatalf("err=%v calls=%q", err, calls())
			}
		})
	}
}

func TestBatchFramingAndErrors(t *testing.T) {
	for _, tc := range []struct {
		reply   string
		success bool
	}{
		{"ok\n\n\nok", true}, {"ok\n\n\nok\n", true}, {"ok\n\n\nok\n\n\n", true},
		{"ok\nok", false}, {"ok", false}, {"ok\n\n\nok\n\n\nextra", false},
		{"ok\n\n\ninvalid workspace", false}, {"error\n\n\nok", false}, {"", false},
	} {
		t.Run(tc.reply, func(t *testing.T) {
			calls := fakeHyprctl(t)
			t.Setenv("REPLY", tc.reply)
			_, err := NewClient(Options{}).BatchDispatch(nil, []string{"workspace", "1"}, []string{"workspace", "2"})
			if (err == nil) != tc.success || len(calls()) != 2 {
				t.Fatalf("err=%v calls=%q", err, calls())
			}
		})
	}
}

func TestDispatchResponseAndProcessErrors(t *testing.T) {
	for _, reply := range []string{"", "invalid dispatcher", "ok\nerror"} {
		t.Run(reply, func(t *testing.T) {
			calls := fakeHyprctl(t)
			t.Setenv("REPLY", reply)
			if err := NewClient(Options{}).Dispatch(nil, "workspace", "1"); err == nil {
				t.Fatal("accepted error reply")
			}
			if len(calls()) != 2 {
				t.Fatal(calls())
			}
		})
	}
	for _, stderr := range []string{"", "stderr diagnostic"} {
		t.Run(stderr, func(t *testing.T) {
			calls := fakeHyprctl(t)
			t.Setenv("REPLY", "stdout diagnostic")
			t.Setenv("STDERR", stderr)
			t.Setenv("REPLY_EXIT", "7")
			err := NewClient(Options{}).Dispatch(nil, "workspace", "1")
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 7 || !strings.Contains(err.Error(), "stdout diagnostic") || !strings.Contains(err.Error(), stderr) {
				t.Fatalf("error: %v", err)
			}
			if len(calls()) != 2 {
				t.Fatal(calls())
			}
		})
	}
}

func TestCancellationAndTimeout(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(map[bool]string{false: "timeout", true: "cancel"}[cancel], func(t *testing.T) {
			fakeHyprctl(t)
			t.Setenv("HANG", "1")
			ctx, stop := context.WithCancel(context.Background())
			defer stop()
			want := context.DeadlineExceeded
			if cancel {
				want = context.Canceled
				time.AfterFunc(30*time.Millisecond, stop)
			}
			_, err := NewClient(Options{Timeout: 80 * time.Millisecond}).runCombined(ctx, "clients", "-j")
			if !errors.Is(err, want) {
				t.Fatalf("error: %v", err)
			}
		})
	}
	t.Run("probe timeout", func(t *testing.T) {
		calls := fakeHyprctl(t)
		t.Setenv("PROBE_HANG", "1")
		err := NewClient(Options{Timeout: 40 * time.Millisecond}).Dispatch(nil, "workspace", "1")
		if !errors.Is(err, context.DeadlineExceeded) || len(calls()) != 1 {
			t.Fatalf("err=%v calls=%q", err, calls())
		}
	})
	t.Run("probe canceled", func(t *testing.T) {
		calls := fakeHyprctl(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := NewClient(Options{}).Dispatch(ctx, "workspace", "1")
		if !errors.Is(err, context.Canceled) || len(calls()) != 0 {
			t.Fatalf("err=%v calls=%q", err, calls())
		}
	})
}

func TestQueriesAndEmptyBatchUnchanged(t *testing.T) {
	calls := fakeHyprctl(t)
	t.Setenv("PROBE", "must not probe")
	t.Setenv("REPLY", "[]")
	c := NewClient(Options{})
	if _, err := c.Clients(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Monitors(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Workspaces(nil); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REPLY", "{\n  \"id\": 1\n}\n\n\n[]\n")
	results, err := c.Batch(nil, []string{"j/activeworkspace"}, nil, []string{"clients", "-j"})
	if err != nil || len(results) != 2 || string(results[0].Output) != "{\n  \"id\": 1\n}" {
		t.Fatalf("results=%v err=%v", results, err)
	}
	if _, err := c.Batch(nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.BatchDispatch(nil); err != nil {
		t.Fatal(err)
	}
	if len(calls()) != 4 {
		t.Fatal(calls())
	}
}
