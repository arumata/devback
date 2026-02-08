//go:build windows

package lock

import (
	"os"
	"testing"
	"time"
)

func TestGetProcessStartTimeWindows(t *testing.T) {
	startTime, ok := getProcessStartTimeWindows(os.Getpid())
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
