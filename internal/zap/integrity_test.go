// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestIntegritySnapshotChanges(t *testing.T) {
	root := t.TempDir()
	write := func(name, text string) {
		t.Helper()
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("src/a.c", "before")
	write("delete.c", "old")
	write(".gitignore", "ignored.c")
	write("ignored.c", "covered")
	write(".zap/history", "excluded")
	write("build/output", "excluded")
	roots := []integrityRoot{{ID: "project", Path: root, Exclude: []string{filepath.Join(root, "build")}}}
	before, err := scanIntegrity(roots, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Files) != 4 {
		t.Fatalf("scope: %+v", before.Files)
	}
	write("src/a.c", "after")
	write("ignored.c", "tampered")
	write("added.c", "new")
	if err := os.Remove(filepath.Join(root, "delete.c")); err != nil {
		t.Fatal(err)
	}
	after, err := scanIntegrity(roots, "")
	if err != nil {
		t.Fatal(err)
	}
	changes := integrityChanges(before, after)
	if len(changes) != 4 {
		t.Fatalf("changes: %+v", changes)
	}
	again, err := scanIntegrity(roots, "")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(integrityBytes(after), integrityBytes(again)) {
		t.Fatal("unstable ordering")
	}
}
func TestIntegrityRejectsMissingRoot(t *testing.T) {
	_, err := scanIntegrity([]integrityRoot{{ID: "dep", Path: filepath.Join(t.TempDir(), "absent")}}, "")
	if err == nil {
		t.Fatal("missing dependency accepted")
	}
}
func TestIntegrityRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := scanIntegrity([]integrityRoot{{ID: "project", Path: root}}, ""); err == nil {
		t.Fatal("uncovered symlink accepted")
	}
}
func TestIntegrityStoresExactContent(t *testing.T) {
	root := t.TempDir()
	objects := t.TempDir()
	content := []byte{0, 1, 2, 255, 10}
	if err := os.WriteFile(filepath.Join(root, "binary"), content, 0600); err != nil {
		t.Fatal(err)
	}
	m, err := scanIntegrity([]integrityRoot{{ID: "project", Path: root}}, objects)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(objects, m.Files[0].SHA256))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Fatal("snapshot lost bytes")
	}
}
