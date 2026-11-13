package provider

import (
	"context"
	"errors"
	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
	"testing"
	"time"
)

func TestAcceptanceReadOnlySSHRetry(t *testing.T) {
	transport := &multipasscli.CLIError{Err: cloudInitCLIExit(t, 255), Stderr: "exec failed: ssh connection failed: 'Timeout connecting to 10.0.0.1'"}
	guest := errors.New("guest assertion failed")
	for _, tc := range []struct {
		name   string
		first  error
		cancel bool
		calls  int
		want   error
	}{
		{"reconnect", transport, false, 2, nil},
		{"guest failure", guest, false, 1, guest},
		{"cancellation", transport, true, 1, context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			calls := 0
			c := &testClient{exec: func(context.Context, string, []string) error {
				calls++
				if tc.cancel {
					cancel()
				}
				if calls == 1 {
					return tc.first
				}
				return nil
			}}
			err := testAccReadOnlyExec(ctx, c, "vm", []string{"test", "-f", "/ready"})
			if !errors.Is(err, tc.want) || calls != tc.calls {
				t.Fatalf("calls=%d error=%v", calls, err)
			}
		})
	}
}
