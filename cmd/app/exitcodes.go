package main

import (
	"fmt"
	"os"
)

const (
	exitSuccess       = 0
	exitCriticalError = 1
	exitLockBusy      = 76
	exitUsageError    = 2
	exitInterrupted   = 130
)

// handleCmdError prints error to stderr and sets exit code.
func handleCmdError(exitCode *int, err error) {
	if err == nil {
		*exitCode = exitSuccess
		return
	}
	fmt.Fprintln(os.Stderr, err)
	*exitCode = mapExitCode(err)
}

// mapExitCodeWithLog prints error to stderr and returns exit code.
func mapExitCodeWithLog(err error) int {
	if err == nil {
		return exitSuccess
	}
	fmt.Fprintln(os.Stderr, err)
	return mapExitCode(err)
}
