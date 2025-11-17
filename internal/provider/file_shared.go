package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

func hashPath(p string, recursive bool) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}

	if info.IsDir() {
		if !recursive {
			return "", fmt.Errorf("path %q is a directory; set `recursive = true`", p)
		}
		return hashDirectory(abs)
	}
	return hashFile(abs)
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashDirectory(root string) (string, error) {
	h := sha256.New()
	if err := walkDirectory(root, root, h); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func walkDirectory(root, current string, h io.Writer) error {
	entries, err := os.ReadDir(current)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		path := filepath.Join(current, entry.Name())
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if _, err := h.Write([]byte(filepath.ToSlash(rel))); err != nil {
			return err
		}

		if entry.IsDir() {
			if err := walkDirectory(root, path, h); err != nil {
				return err
			}
			continue
		}

		contentHash, err := hashFile(path)
		if err != nil {
			return err
		}
		if _, err := h.Write([]byte(contentHash)); err != nil {
			return err
		}
	}
	return nil
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
