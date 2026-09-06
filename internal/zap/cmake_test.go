// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testCommit = "0123456789abcdef0123456789abcdef01234567"

func TestGeneratedCMakeUsesLockedCommitAndVerifier(t *testing.T) {
	c := &Config{Schema: ConfigSchema, Project: ProjectConfig{Environment: "generic", Target: "app", DepsDir: "deps"}, Dependencies: map[string]*DependencyConfig{"zapee": {Type: "git", URI: "https://example.invalid/zap-ee.git", Version: "v1.2.3", Commit: testCommit, Components: []string{"zap::ztm_pwm"}}}, DependencyOrder: []string{"zapee"}}
	s, err := GenerateCMake(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{`EXISTS "${ZAP_DEPS_DIR}/zapee/CMakeLists.txt"`, `GIT_REPOSITORY [[https://example.invalid/zap-ee.git]]`, `GIT_TAG        [[` + testCommit + `]]`, `LINK_LIBRARIES zap::ztm_pwm`, `verify --cmake`, `ZAP_OFFLINE`, `ZAP_VERIFY_DEPENDENCIES`} {
		if !strings.Contains(s, needle) {
			t.Fatalf("generated CMake missing %q\n%s", needle, s)
		}
	}
	if strings.Contains(s, "GIT_SHALLOW") {
		t.Fatal("commit-hash CMake fallback must not use GIT_SHALLOW")
	}
}

func TestEnsureCMakeIntegrationPreservesUserContent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "CMakeLists.txt")
	original := `cmake_minimum_required(VERSION 3.20)
project(Demo C)
# USER BEFORE
add_executable(app main.c)
# USER AFTER
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{Schema: ConfigSchema, Project: ProjectConfig{Environment: "generic", Target: "app", DepsDir: "deps"}, Dependencies: map[string]*DependencyConfig{}}
	if err := EnsureCMakeIntegration(root, cfg); err != nil {
		t.Fatal(err)
	}
	firstBytes, _ := os.ReadFile(path)
	first := string(firstBytes)
	for _, user := range []string{"# USER BEFORE", "add_executable(app main.c)", "# USER AFTER"} {
		if !strings.Contains(first, user) {
			t.Fatalf("user content %q was lost\n%s", user, first)
		}
	}
	if !strings.Contains(first, "User CMake outside this block is never rewritten by zap.") {
		t.Fatal("managed block assurance comment missing")
	}
	// User changes outside the managed blocks survive a later regeneration.
	first = strings.Replace(first, "# USER AFTER", "# USER AFTER\nset(USER_SETTING 123)", 1)
	if err := os.WriteFile(path, []byte(first), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureCMakeIntegration(root, cfg); err != nil {
		t.Fatal(err)
	}
	secondBytes, _ := os.ReadFile(path)
	if !strings.Contains(string(secondBytes), "set(USER_SETTING 123)") {
		t.Fatal("user modification outside Zap markers was changed")
	}
}

func TestMalformedManagedMarkersAreRefused(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "CMakeLists.txt")
	if err := os.WriteFile(path, []byte("project(Demo C)\n"+includeBegin+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{Schema: ConfigSchema, Project: ProjectConfig{Environment: "generic", Target: "app", DepsDir: "deps"}, Dependencies: map[string]*DependencyConfig{}}
	if err := EnsureCMakeIntegration(root, cfg); err == nil {
		t.Fatal("expected malformed marker refusal")
	}
}

func TestSemverOrdering(t *testing.T) {
	vv := []semVersion{{0, 4, 9, "v0.4.9"}, {0, 5, 0, "v0.5.0"}, {1, 0, 0, "v1.0.0"}}
	if !(vv[2].major > vv[1].major && vv[1].minor > vv[0].minor) {
		t.Fatal("semver fields")
	}
}
