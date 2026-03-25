package multipasscli

import "testing"

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
