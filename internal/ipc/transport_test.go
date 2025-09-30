package ipc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestDialContextDaemonOffline(t *testing.T) {
	missingSocket := filepath.Join(t.TempDir(), "missing.sock")
	t.Setenv("HYPR_ORBITS_SOCKET", missingSocket)

	_, err := DialContext(context.Background(), DialOptions{})
	if err == nil {
		t.Fatalf("expected error when dialing missing socket")
	}

	var offlineErr *DaemonOfflineError
	if !errors.As(err, &offlineErr) {
		t.Fatalf("expected DaemonOfflineError, got %T: %v", err, err)
	}

	if offlineErr.Path != missingSocket {
		t.Fatalf("expected path %q, got %q", missingSocket, offlineErr.Path)
	}

	if msg := offlineErr.Error(); !strings.Contains(msg, "hyprorbits daemon is not running") || !strings.Contains(msg, missingSocket) {
		t.Fatalf("error message missing context: %q", msg)
	}

	if !(errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EPERM)) {
		t.Fatalf("expected original error to unwrap to a not-running condition, got %v", err)
	}
}
