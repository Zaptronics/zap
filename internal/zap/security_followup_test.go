// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanProtectsProjectCaseAlias(t *testing.T) {
	fixture := t.TempDir()
	root := filepath.Join(fixture, "MixedCaseProject")
	alias := filepath.Join(fixture, "mixedcaseproject")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "source.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := os.Stat(alias)
	if os.IsNotExist(err) {
		t.Skip("filesystem is case-sensitive")
	}
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(a, b) {
		t.Fatal("fixture paths are not aliases")
	}
	// Both paths must remain inside the disposable fixture before cleanup runs.
	rel, err := filepath.Rel(fixture, alias)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		t.Fatal("unsafe cleanup fixture")
	}
	if err := safeRemoveBuildDir(root, alias); err == nil {
		t.Error("accepted project root alias")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Error("project source was deleted")
	}
	build := filepath.Join(root, "build")
	if err := os.Mkdir(build, 0700); err != nil {
		t.Fatal(err)
	}
	if err := safeRemoveBuildDir(root, build); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(build); !os.IsNotExist(err) {
		t.Fatal("ordinary build cleanup failed")
	}
}

func TestDuplicateURIWithTrailingColon(t *testing.T) {
	for _, document := range []string{"project", "package", "lock"} {
		t.Run(document, func(t *testing.T) {
			prefix := "schema: 5\nproject:\n  target: app\n"
			parse := func(s string) error { _, err := ParseConfig(s); return err }
			if document == "package" {
				prefix = "schema: 2\npackage:\n  name: demo\n"
				parse = func(s string) error { _, err := ParsePackageManifest(s); return err }
			} else if document == "lock" {
				prefix = "schema: 1\n"
				parse = func(s string) error { _, err := ParseLock(s); return err }
			}
			base := prefix + "dependencies:\n  child:\n    type: git\n    uri: https://reviewed.invalid/repo:\n    version: v1.0.0\n"
			if document == "lock" {
				base = strings.Replace(base, "    version: v1.0.0\n", "    resolved: v1.0.0\n    commit: "+strings.Repeat("a", 40)+"\n", 1)
			}
			// Ensure rejection is caused by the duplicate, not an invalid fixture.
			if err := parse(base); err != nil {
				t.Fatalf("valid scalar rejected: %v", err)
			}
			for _, field := range []string{"    uri: https://replacement.invalid/repo\n", "    uri:https://replacement.invalid/repo:\n"} {
				if err := parse(base + field); err == nil || !strings.Contains(err.Error(), "repeats mapping key") {
					t.Fatalf("duplicate was not detected: %v", err)
				}
			}
		})
	}
}

func TestDuplicateCheckPreservesNamespacedComponents(t *testing.T) {
	manifest := "schema: 2\npackage:\n  name: demo\ncomponents:\n  demo::one:\n    paths:\n      - one\n  demo::two:\n    paths:\n      - two\n"
	if _, err := ParsePackageManifest(manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePackageManifest(manifest + "  demo::one:\n    paths:\n      - replacement\n"); err == nil {
		t.Fatal("duplicate namespaced component accepted")
	}
}
