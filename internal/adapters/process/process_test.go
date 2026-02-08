package process

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

func TestAdapter_ProcessInfo(t *testing.T) {
	ctx := context.Background()
	adapter := New(slog.Default())
	pid := adapter.GetPID()
	if pid != os.Getpid() {
		t.Fatalf("expected pid %d, got %d", os.Getpid(), pid)
	}

	if !adapter.IsProcessRunning(ctx, pid) {
		t.Fatal("expected current process to be running")
	}

	info, err := adapter.GetProcessInfo(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	if info.PID != pid {
		t.Fatal("expected process info pid")
	}
}

func TestAdapter_KillProcess_InvalidPID(t *testing.T) {
	ctx := context.Background()
	adapter := New(slog.Default())
	if err := adapter.KillProcess(ctx, 0); err == nil {
		t.Fatal("expected error for invalid pid")
	}
}

func TestAdapter_InvalidPIDChecks(t *testing.T) {
	ctx := context.Background()
	adapter := New(slog.Default())
	if adapter.IsProcessRunning(ctx, 0) {
		t.Fatal("expected false for invalid pid")
	}
	if _, err := adapter.GetProcessInfo(ctx, 0); err == nil {
		t.Fatal("expected error for invalid pid")
	}
}

func TestAdapter_KillProcess(t *testing.T) {
	ctx := context.Background()
	adapter := New(slog.Default())

	cmd := newHelperProcess(t, 5*time.Second)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper process: %v", err)
	}

	if err := adapter.KillProcess(ctx, cmd.Process.Pid); err != nil {
		_ = cmd.Process.Kill()
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("process did not exit")
	}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	if len(args) < 3 || args[1] != "--" || args[2] != "sleep" {
		os.Exit(2)
	}
	if len(args) < 4 {
		os.Exit(2)
	}
	seconds, err := strconv.Atoi(args[3])
	if err != nil || seconds <= 0 {
		os.Exit(2)
	}
	time.Sleep(time.Duration(seconds) * time.Second)
	os.Exit(0)
}

func newHelperProcess(t *testing.T, d time.Duration) *exec.Cmd {
	t.Helper()
	seconds := int(d.Seconds())
	if seconds <= 0 {
		seconds = 1
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--", "sleep", strconv.Itoa(seconds))
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	return cmd
}
