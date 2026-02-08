package main

import "testing"

func TestExitCodes(t *testing.T) {
	tests := []struct {
		name     string
		code     int
		expected int
	}{
		{"exitSuccess", exitSuccess, 0},
		{"exitCriticalError", exitCriticalError, 1},
		{"exitLockBusy", exitLockBusy, 76},
		{"exitUsageError", exitUsageError, 2},
		{"exitInterrupted", exitInterrupted, 130},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.code != tt.expected {
				t.Errorf("Expected %s to be %d, got %d", tt.name, tt.expected, tt.code)
			}
		})
	}
}
