package provider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
)

func cloudInitCLIExit(t *testing.T, code int) *exec.ExitError {
	t.Helper()
	command := "cloud-init" // The CLI fixture's default exit is 2.
	if code == 255 {
		command = "ssh-error"
	}
	cmd := exec.Command(os.Args[0], command)
	cmd.Env = append(os.Environ(), "MULTIPASS_PROVIDER_TEST_CLI=1")
	err, ok := cmd.Run().(*exec.ExitError)
	if !ok || err.ExitCode() != code {
		t.Fatalf("expected fixture exit %d, got %v", code, err)
	}
	return err
}

func TestCloudInitSSHDisconnectClassification(t *testing.T) {
	exit255, exit2 := cloudInitCLIExit(t, 255), cloudInitCLIExit(t, 2)
	for _, tc := range []struct {
		message string
		exit    error
		retry   bool
	}{
		{"exec failed: ssh connection failed: 'Socket error: Unknown error'", exit255, true},
		{"exec failed: [ssh client] channel creation failed: 'Socket error: Unknown error'", exit255, true},
		{"exec failed: [ssh proc] failed to open session channel: 'Socket error: disconnected'", exit255, true},
		{"exec failed: ssh connection failed: 'Socket error: Connection refused'", exit255, true},
		{"exec failed: ssh connection failed: 'Authentication failed'", exit255, false},
		{"exec failed: instance does not exist", exit255, false},
		{"list failed: cannot connect to the multipass socket", exit255, false},
		{"guest output mentions Socket error: Unknown error", exit255, false},
		{"exec failed: ssh connection failed: 'Socket error: Unknown error'", exit2, false},
		{"exec failed: ssh connection failed: 'Socket error: Unknown error'", context.DeadlineExceeded, false},
	} {
		err := fmt.Errorf("wrapped: %w", &multipasscli.CLIError{Stderr: tc.message, Err: tc.exit})
		if got := isCloudInitSSHDisconnect(err); got != tc.retry {
			t.Errorf("%q (%v): retry=%t, want %t", tc.message, tc.exit, got, tc.retry)
		}
	}
	if isCloudInitSSHDisconnect(nil) || isCloudInitSSHDisconnect(context.Canceled) {
		t.Fatal("non-CLI result must not be retried")
	}
}

func TestCloudInitReconnect(t *testing.T) {
	disconnect := &multipasscli.CLIError{Stderr: "exec failed: ssh connection failed: 'Socket error: Unknown error'", Err: cloudInitCLIExit(t, 255)}
	warning := &multipasscli.CLIError{Err: cloudInitCLIExit(t, 2), Stdout: `{"status":"done","errors":[],"recoverable_errors":{"WARNING":["hostname warning"]}}`}
	failure := errors.New("cloud-init module failed")
	for _, tc := range []struct {
		name     string
		sequence []error
		commands [][]string
		wantErr  bool
		wantWarn bool
	}{
		{"wait reconnect", []error{disconnect, nil}, [][]string{{"--wait"}, {"--wait"}}, false, false},
		{"query reconnect", []error{warning, disconnect, warning}, [][]string{{"--wait"}, {"--format", "json"}, {"--format", "json"}}, false, true},
		{"failure after reconnect", []error{disconnect, failure}, [][]string{{"--wait"}, {"--wait"}}, true, false},
		{"provisioning failure", []error{failure}, [][]string{{"--wait"}}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			calls := 0
			r := &instanceResource{client: &testClient{execCapture: func(callCtx context.Context, _ string, command []string) ([]byte, error) {
				if callCtx != ctx {
					t.Fatal("retry replaced the original timeout/cancellation context")
				}
				if calls >= len(tc.sequence) {
					t.Fatal("unexpected extra status call")
				}
				want := append([]string{"cloud-init", "status"}, tc.commands[calls]...)
				if !reflect.DeepEqual(command, want) {
					t.Fatalf("command %v, want %v", command, want)
				}
				err := tc.sequence[calls]
				calls++
				return nil, err
			}}}
			diags := r.waitForCloudInit(ctx, "vm")
			if calls != len(tc.sequence) || diags.HasError() != tc.wantErr || (diags.WarningsCount() != 0) != tc.wantWarn {
				t.Fatalf("calls=%d diagnostics=%v", calls, diags)
			}
		})
	}
}

func TestCloudInitReconnectCancellation(t *testing.T) {
	disconnect := &multipasscli.CLIError{Stderr: "exec failed: [ssh client] channel creation failed: 'Socket error: Unknown error'", Err: cloudInitCLIExit(t, 255)}
	for _, deadline := range []bool{false, true} {
		t.Run(fmt.Sprint("deadline=", deadline), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			want := context.Canceled
			if deadline {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 50*time.Millisecond)
				want = context.DeadlineExceeded
			}
			defer cancel()
			calls := 0
			r := &instanceResource{client: &testClient{execCapture: func(context.Context, string, []string) ([]byte, error) {
				calls++
				if !deadline {
					cancel()
				}
				return nil, disconnect
			}}}
			_, err := r.cloudInitStatus(ctx, "vm", "--wait")
			if calls != 1 || !errors.Is(err, want) || !strings.Contains(err.Error(), "Socket error:") {
				t.Fatalf("calls=%d error=%v", calls, err)
			}
			_, err = r.cloudInitStatus(ctx, "vm", "--wait")
			if calls != 1 || !errors.Is(err, want) {
				t.Fatalf("cancelled context retried: calls=%d error=%v", calls, err)
			}
		})
	}
}
