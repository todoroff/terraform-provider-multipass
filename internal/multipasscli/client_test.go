package multipasscli

import (
	"reflect"
	"testing"
)

func TestAliasCommand_noDir(t *testing.T) {
	t.Parallel()
	got := aliasCommand("ls -la", "")
	if got != "ls -la" {
		t.Fatalf("expected unchanged command, got %q", got)
	}
}

func TestAliasCommand_withDir(t *testing.T) {
	t.Parallel()
	got := aliasCommand("ls", "/workspace")
	want := `bash -c 'cd "/workspace" && exec ls'`
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestAliasCommand_withDirAndArgs(t *testing.T) {
	t.Parallel()
	got := aliasCommand("grep foo bar.txt", "/home/ubuntu/project")
	want := `bash -c 'cd "/home/ubuntu/project" && exec grep foo bar.txt'`
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestAliasCommand_dirWithSpaces(t *testing.T) {
	t.Parallel()
	got := aliasCommand("bash", "/my project")
	want := `bash -c 'cd "/my project" && exec bash'`
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestAliasCommand_commandWithSingleQuotes(t *testing.T) {
	t.Parallel()
	got := aliasCommand("grep 'foo' bar.txt", "/workspace")
	want := `bash -c 'cd "/workspace" && exec grep '\''foo'\'' bar.txt'`
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestAliasCommand_dirWithSingleQuotes(t *testing.T) {
	t.Parallel()
	// Single quote in dir is escaped with '\'' for the outer wrapper,
	// but stays literal inside the double-quoted cd argument.
	got := aliasCommand("ls", "/tmp/user's files")
	want := `bash -c 'cd "/tmp/user'\''s files" && exec ls'`
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
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
