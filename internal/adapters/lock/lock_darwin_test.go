//go:build darwin

package lock

import (
	"os"
	"testing"
	"time"
)

func TestGetProcessStartTimeDarwin(t *testing.T) {
	startTime, ok := getProcessStartTimeDarwin(os.Getpid())
	if !ok {
		t.Fatal("expected start time to be available")
	}
	if startTime.IsZero() {
		t.Fatal("expected non-zero start time")
	}
	if startTime.After(time.Now().Add(1 * time.Second)) {
		t.Fatalf("expected start time to be in the past, got %v", startTime)
	}
}
