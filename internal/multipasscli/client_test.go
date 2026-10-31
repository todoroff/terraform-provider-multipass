package multipasscli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestAliasCommand_noDir(t *testing.T) {
	t.Parallel()
	got := aliasCommand("ls -la", "")
	if got != "ls -la" {
		t.Fatalf("expected unchanged command, got %q", got)
	}
}

func TestAliasCommandLiteralDirectory(t *testing.T) {
	shell, err := exec.LookPath("bash")
	if runtime.GOOS == "windows" {
		shell = `C:\Program Files\Git\bin\bash.exe`
		_, err = os.Stat(shell)
	}
	if err != nil {
		t.Skip("bash is required for shell quoting regression")
	}
	for _, name := range []string{"space name", "single'quote", "$USER", "$(printf injected)", "`printf injected`"} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), name)
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "sentinel"), []byte("literal directory"), 0600); err != nil {
				t.Fatal(err)
			}
			out, err := exec.Command(shell, "--noprofile", "--norc", "-c", aliasCommand("cat 'sentinel'", filepath.ToSlash(dir))).CombinedOutput()
			if err != nil || string(out) != "literal directory" {
				t.Fatalf("output=%q error=%v", out, err)
			}
		})
	}
	for _, value := range []string{`double"quote`, `back\slash`, "line\nbreak", "single'quote", "$(printf injected)"} {
		out, err := exec.Command(shell, "--noprofile", "--norc", "-c", "printf %s "+shellQuote(value)).CombinedOutput()
		if err != nil || string(out) != value {
			t.Fatalf("literal %q: output=%q error=%v", value, out, err)
		}
	}
}
func TestBuildTransferArgs_sources(t *testing.T) {
	t.Parallel()
	got := buildTransferArgs(TransferOptions{
		Sources:     []string{"/host/file"},
		Destination: "vm:/tmp/file",
	})
	want := []string{"transfer", "/host/file", "vm:/tmp/file"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %v\nwant %v", got, want)
	}
}

func TestBuildTransferArgs_recursiveAndParents(t *testing.T) {
	t.Parallel()
	got := buildTransferArgs(TransferOptions{
		Sources:     []string{"/host/dir"},
		Destination: "vm:/tmp/dir",
		Recursive:   true,
		Parents:     true,
	})
	want := []string{"transfer", "--recursive", "--parents", "/host/dir", "vm:/tmp/dir"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %v\nwant %v", got, want)
	}
}

func TestBuildTransferArgs_stdin(t *testing.T) {
	t.Parallel()
	// When Stdin is set, the source is "-" (read from stdin) and any
	// Sources slice is ignored. This is how we avoid a tempfile for inline
	// content so that snap-confined multipass installs can read it.
	got := buildTransferArgs(TransferOptions{
		Sources:     []string{"/should/be/ignored"},
		Destination: "vm:/tmp/file",
		Stdin:       []byte("hello"),
	})
	want := []string{"transfer", "-", "vm:/tmp/file"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %v\nwant %v", got, want)
	}
}

func TestBuildTransferArgs_stdinEmptyBytesStillUsesDash(t *testing.T) {
	t.Parallel()
	// A non-nil but empty byte slice still means "pipe via stdin".
	got := buildTransferArgs(TransferOptions{
		Destination: "vm:/tmp/empty",
		Stdin:       []byte{},
	})
	want := []string{"transfer", "-", "vm:/tmp/empty"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %v\nwant %v", got, want)
	}
}

func TestTransferCapture_rejectsStdin(t *testing.T) {
	t.Parallel()
	// TransferCapture reads bytes into memory via destination="-"; pairing
	// that with Stdin (source="-") would cause multipass to block forever
	// waiting for input. Reject it up front instead.
	c := &client{binaryPath: "multipass"}
	_, err := c.TransferCapture(context.Background(), TransferOptions{
		Sources:     []string{"vm:/tmp/file"},
		Destination: "-",
		Stdin:       []byte("nope"),
	})
	if err == nil {
		t.Fatal("expected error when Stdin is set on TransferCapture")
	}
	if !strings.Contains(err.Error(), "stdin") {
		t.Fatalf("expected error to mention stdin, got: %v", err)
	}
}
