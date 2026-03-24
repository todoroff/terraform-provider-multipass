package multipasscli

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrNotFound indicates the requested entity does not exist.
	ErrNotFound = errors.New("not found")

	// ErrTimeout indicates the command timed out.
	ErrTimeout = errors.New("command timed out")
)

// isTimeoutError checks whether a CLI error's stderr indicates a timeout.
func isTimeoutError(stderr string) bool {
	lower := strings.ToLower(stderr)
	return strings.Contains(lower, "timed out") || strings.Contains(lower, "timeout")
}

// CLIError represents a failure raised by the multipass CLI.
type CLIError struct {
	Command string
	Stdout  string
	Stderr  string
	Err     error
}

func (e *CLIError) Error() string {
	return fmt.Sprintf("multipass %s failed: %v (stderr: %s)", e.Command, e.Err, e.Stderr)
}

func (e *CLIError) Unwrap() error {
	return e.Err
}
