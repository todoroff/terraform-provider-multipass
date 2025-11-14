package multipasscli

import (
	"errors"
	"fmt"
)

var (
	// ErrNotFound indicates the requested entity does not exist.
	ErrNotFound = errors.New("not found")
)

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
