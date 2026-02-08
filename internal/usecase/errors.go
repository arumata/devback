package usecase

import "errors"

var (
	// ErrUsage indicates user input/usage errors.
	ErrUsage = errors.New("usage error")
	// ErrCritical indicates critical failures that should exit with error.
	ErrCritical = errors.New("critical error")
	// ErrLockBusy indicates an active lock held by another process.
	ErrLockBusy = errors.New("lock busy")
	// ErrInterrupted indicates a canceled or interrupted operation.
	ErrInterrupted = errors.New("interrupted")
)
