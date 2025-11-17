package provider

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHashPathFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "script.sh")
	content := []byte("#!/bin/bash\necho hi\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	got, err := hashPath(path, false)
	if err != nil {
		t.Fatalf("hashPath returned error: %v", err)
	}
	want := hashBytes(content)
	if got != want {
		t.Fatalf("hash mismatch: got %s want %s", got, want)
	}
}

func TestHashPathDirectoryRequiresRecursive(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if _, err := hashPath(dir, false); err == nil {
		t.Fatalf("expected error when hashing directory without recursion")
	}
}

func TestHashDirectoryDetectsChanges(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	write := func(path, data string) {
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}

	write(filepath.Join(dir, "a.txt"), "one")
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	write(filepath.Join(dir, "nested", "b.txt"), "two")

	initial, err := hashPath(dir, true)
	if err != nil {
		t.Fatalf("hashPath initial: %v", err)
	}

	write(filepath.Join(dir, "a.txt"), "changed")
	updated, err := hashPath(dir, true)
	if err != nil {
		t.Fatalf("hashPath updated: %v", err)
	}

	if initial == updated {
		t.Fatalf("expected hash to change after modifying directory contents")
	}
}
