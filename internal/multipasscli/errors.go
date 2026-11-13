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

// Only absence of the requested entity permits Terraform to discard its state.
func isNotFoundError(args []string, stderr string) bool {
	if len(args) < 2 {
		return false
	}
	command := args[0]
	targets := args[1:]
	for len(targets) > 0 && (targets[0] == "--purge" || targets[0] == "--raw" || targets[0] == "--cancel") {
		targets = targets[1:]
	}
	if len(targets) == 0 {
		return false
	}
	target := targets[0]
	if command == "unalias" {
		return stderr == "Nonexistent alias: "+target+"."
	}
	detail, ok := strings.CutPrefix(stderr, command+" failed: ")
	if !ok {
		return false
	}
	detail = strings.TrimPrefix(detail, "The following errors occurred:\n")
	switch command {
	case "info", "delete", "recover", "start", "stop", "suspend", "restart":
		if command == "delete" {
			if detail == "No such snapshot: "+target {
				return true
			}
			if dot := strings.LastIndexByte(target, '.'); dot >= 0 {
				target = target[:dot]
			}
		}
		return detail == fmt.Sprintf("instance %q does not exist", target) || detail == fmt.Sprintf("instance '%s' does not exist", target)
	case "get", "set":
		key, _, _ := strings.Cut(target, "=")
		instance, ok := strings.CutPrefix(key, "local.")
		dot := strings.LastIndexByte(instance, '.')
		if !ok || dot <= 0 {
			return false
		}
		operation := "obtain"
		if command == "set" {
			operation = "update"
		}
		return detail == fmt.Sprintf("Cannot %s instance settings; instance: %s; reason: No such instance", operation, instance[:dot])
	}
	return false
}
