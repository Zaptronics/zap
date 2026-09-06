// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func fileURI(path string) string {
	p := filepath.ToSlash(path)
	if runtime.GOOS == "windows" {
		return "file:///" + p
	}
	return "file://" + p
}

func TestSparseGitSync(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(repo, "drivers", "chip"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "modules", "board"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "modules", "other"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite := func(rel, data string) {
		t.Helper()
		p := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("CMakeLists.txt", "cmake_minimum_required(VERSION 3.20)\n")
	mustWrite("common/README.txt", "common")
	mustWrite("drivers/chip/chip.c", "chip")
	mustWrite("modules/board/board.c", "board")
	mustWrite("modules/other/other.c", "other")
	mustWrite("zap-package.yml", `schema: 1
package:
  name: Test
  paths:
    - common
components:
  demo::chip:
    kind: driver
    paths:
      - drivers/chip
  demo::board:
    kind: module
    depends:
      - demo::chip
    paths:
      - modules/board
  demo::other:
    kind: module
    paths:
      - modules/other
`)
	commands := [][]string{{"init", "-q"}, {"config", "user.email", "test@example.com"}, {"config", "user.name", "Test"}, {"add", "."}, {"commit", "-qm", "base"}, {"tag", "v1.0.0"}}
	for _, a := range commands {
		cmd := exec.Command(git, a...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(a, " "), err, out)
		}
	}

	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	p := OpenProject(projectRoot)
	cfg := &Config{Schema: 1, Project: ProjectConfig{Environment: "generic", Target: "app", BuildDir: "build", DepsDir: "deps"}, Dependencies: map[string]*DependencyConfig{"demo": {Type: "git", URI: fileURI(repo), Version: "v1.0.0", Components: []string{"demo::board"}}}, DependencyOrder: []string{"demo"}}
	if err := p.Sync(cfg); err != nil {
		t.Fatal(err)
	}
	dep := filepath.Join(projectRoot, "deps", "demo")
	for _, rel := range []string{"common/README.txt", "drivers/chip/chip.c", "modules/board/board.c"} {
		if _, err := os.Stat(filepath.Join(dep, rel)); err != nil {
			t.Fatalf("required sparse path missing %s: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dep, "modules", "other", "other.c")); !os.IsNotExist(err) {
		t.Fatalf("unselected component unexpectedly present: %v", err)
	}
}

func TestVerifyDetectsMovedTag(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(git, args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "CMakeLists.txt"), []byte("cmake_minimum_required(VERSION 3.20)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-qm", "one")
	first := run("rev-parse", "HEAD")
	run("tag", "-a", "v1.0.0", "-m", "v1")

	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	p := OpenProject(projectRoot)
	cfg := &Config{Schema: ConfigSchema, Project: ProjectConfig{Environment: "generic", Target: "app", BuildDir: "build", DepsDir: "deps"}, Dependencies: map[string]*DependencyConfig{"demo": {Type: "git", URI: fileURI(repo), Version: "v1.0.0", Commit: first}}, DependencyOrder: []string{"demo"}}
	if err := p.Verify(cfg, VerifyOptions{}); err != nil {
		t.Fatalf("initial verification failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repo, "second.txt"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-qm", "two")
	run("tag", "-f", "v1.0.0")
	if err := p.Verify(cfg, VerifyOptions{}); err == nil || !strings.Contains(err.Error(), "integrity failure") {
		t.Fatalf("expected moved tag integrity failure, got %v", err)
	}
}

func TestOfflineVerifyChecksLocalLockedCopy(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := func(args ...string) {
		t.Helper()
		c := exec.Command(git, args...)
		c.Dir = repo
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	cmd("init", "-q")
	cmd("config", "user.email", "test@example.com")
	cmd("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "CMakeLists.txt"), []byte("cmake_minimum_required(VERSION 3.20)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd("add", ".")
	cmd("commit", "-qm", "base")
	shaCmd := exec.Command(git, "rev-parse", "HEAD")
	shaCmd.Dir = repo
	out, _ := shaCmd.Output()
	sha := strings.TrimSpace(string(out))
	cmd("tag", "v1.0.0")

	root := t.TempDir()
	p := OpenProject(root)
	cfg := &Config{Schema: ConfigSchema, Project: ProjectConfig{Environment: "generic", Target: "app", BuildDir: "build", DepsDir: "deps"}, Dependencies: map[string]*DependencyConfig{"demo": {Type: "git", URI: fileURI(repo), Version: "v1.0.0", Commit: sha}}, DependencyOrder: []string{"demo"}}
	if err := p.Sync(cfg); err != nil {
		t.Fatal(err)
	}
	if err := p.Verify(cfg, VerifyOptions{Offline: true}); err != nil {
		t.Fatal(err)
	}
	depFile := filepath.Join(root, "deps", "demo", "local.txt")
	if err := os.WriteFile(depFile, []byte("modified"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := p.Verify(cfg, VerifyOptions{Offline: true}); err == nil || !strings.Contains(err.Error(), "modifications") {
		t.Fatalf("expected dirty checkout failure, got %v", err)
	}
}

func TestSyncLocksRemoteWhileLocalOverrideIsActive(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(git, args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "CMakeLists.txt"), []byte("cmake_minimum_required(VERSION 3.20)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-qm", "base")
	sha := run("rev-parse", "HEAD")
	run("tag", "-a", "v1.0.0", "-m", "v1")

	override := filepath.Join(t.TempDir(), "override")
	if err := os.MkdirAll(override, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(override, "CMakeLists.txt"), []byte("cmake_minimum_required(VERSION 3.20)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_ZAP_SOURCE", override)

	root := t.TempDir()
	p := OpenProject(root)
	cfg := &Config{Schema: 1, Project: ProjectConfig{Environment: "generic", Target: "app", BuildDir: "build", DepsDir: "deps"}, Dependencies: map[string]*DependencyConfig{"demo": {Type: "git", URI: fileURI(repo), Version: "v1.0.0", OverrideVar: "TEST_ZAP_SOURCE"}}, DependencyOrder: []string{"demo"}}
	if err := p.Sync(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Schema != ConfigSchema {
		t.Fatalf("schema = %d, want %d", cfg.Schema, ConfigSchema)
	}
	if !strings.EqualFold(cfg.Dependencies["demo"].Commit, sha) {
		t.Fatalf("commit = %q, want %q", cfg.Dependencies["demo"].Commit, sha)
	}
	if _, err := os.Stat(filepath.Join(root, "deps", "demo")); !os.IsNotExist(err) {
		t.Fatalf("managed dependency checkout should not be created while override is active: %v", err)
	}
}

func TestVerifyStillChecksRemoteLockWithLocalOverride(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(git, args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "CMakeLists.txt"), []byte("cmake_minimum_required(VERSION 3.20)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-qm", "one")
	first := run("rev-parse", "HEAD")
	run("tag", "-a", "v1.0.0", "-m", "v1")

	override := filepath.Join(t.TempDir(), "override")
	if err := os.MkdirAll(override, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_ZAP_SOURCE", override)

	root := t.TempDir()
	p := OpenProject(root)
	cfg := &Config{Schema: ConfigSchema, Project: ProjectConfig{Environment: "generic", Target: "app", BuildDir: "build", DepsDir: "deps"}, Dependencies: map[string]*DependencyConfig{"demo": {Type: "git", URI: fileURI(repo), Version: "v1.0.0", Commit: first, OverrideVar: "TEST_ZAP_SOURCE"}}, DependencyOrder: []string{"demo"}}
	if err := p.Verify(cfg, VerifyOptions{}); err != nil {
		t.Fatalf("initial verification failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repo, "second.txt"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-qm", "two")
	run("tag", "-f", "v1.0.0")
	if err := p.Verify(cfg, VerifyOptions{}); err == nil || !strings.Contains(err.Error(), "integrity failure") {
		t.Fatalf("expected moved tag integrity failure with override active, got %v", err)
	}
}
