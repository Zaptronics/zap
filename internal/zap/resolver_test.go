// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionConstraints(t *testing.T) {
	cases := []struct {
		spec string
		yes  []string
		no   []string
	}{
		{"1.2.3", []string{"1.2.3", "v1.2.3"}, []string{"1.2.4"}},
		{"^1.2.3", []string{"1.2.3", "1.9.9"}, []string{"2.0.0", "1.2.2"}},
		{"^0.5.1", []string{"0.5.1", "0.5.9"}, []string{"0.6.0", "0.5.0"}},
		{"~1.2.3", []string{"1.2.3", "1.2.99"}, []string{"1.3.0"}},
		{">=1.2.0 <2.0.0", []string{"1.2.0", "1.99.0"}, []string{"1.1.9", "2.0.0"}},
	}
	for _, tc := range cases {
		spec, err := parseVersionSpec(tc.spec)
		if err != nil {
			t.Fatalf("%s: %v", tc.spec, err)
		}
		for _, raw := range tc.yes {
			v, _ := parseSemver(raw)
			if !spec.matchesVersion(v) {
				t.Errorf("%s should match %s", tc.spec, raw)
			}
		}
		for _, raw := range tc.no {
			v, _ := parseSemver(raw)
			if spec.matchesVersion(v) {
				t.Errorf("%s should not match %s", tc.spec, raw)
			}
		}
	}
}

func TestLockRoundTrip(t *testing.T) {
	in := &Lockfile{Schema: 1, Dependencies: map[string]*LockedDependency{
		"zapee": {Type: "git", URI: "https://example.invalid/zap-ee.git", Requested: []string{"^0.5.0"}, RootRequested: "^0.5.0", Resolved: "v0.5.3", Commit: "0123456789abcdef0123456789abcdef01234567", ManifestSHA256: strings.Repeat("a", 64), Direct: true, Parents: []string{"root"}, RootComponents: []string{"zap::ztm_pwm"}, Components: []string{"zap::ztm_pwm"}},
	}, DependencyOrder: []string{"zapee"}}
	text := FormatLock(in)
	out, err := ParseLock(text)
	if err != nil {
		t.Fatal(err)
	}
	if out.Dependencies["zapee"].Resolved != "v0.5.3" || out.Dependencies["zapee"].RootRequested != "^0.5.0" {
		t.Fatalf("unexpected round-trip: %#v\n%s", out.Dependencies["zapee"], text)
	}
}

type testGitRepo struct {
	path string
	git  string
	t    *testing.T
}

func newTestGitRepo(t *testing.T) *testGitRepo {
	t.Helper()
	t.Setenv("ZAP_ALLOW_LOCAL_GIT", "1")
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	r := &testGitRepo{path: filepath.Join(t.TempDir(), "repo"), git: git, t: t}
	if err := os.MkdirAll(r.path, 0o755); err != nil {
		t.Fatal(err)
	}
	r.run("init", "-q")
	r.run("config", "user.email", "test@example.com")
	r.run("config", "user.name", "Test")
	return r
}

func (r *testGitRepo) run(args ...string) string {
	r.t.Helper()
	cmd := exec.Command(r.git, args...)
	cmd.Dir = r.path
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func (r *testGitRepo) release(tag, manifest string) {
	r.t.Helper()
	if err := os.WriteFile(filepath.Join(r.path, "CMakeLists.txt"), []byte("cmake_minimum_required(VERSION 3.20)\n"), 0o644); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.path, "zap-package.yml"), []byte(manifest), 0o644); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.path, "VERSION"), []byte(tag+"\n"), 0o644); err != nil {
		r.t.Fatal(err)
	}
	r.run("add", ".")
	r.run("commit", "-qm", tag)
	r.run("tag", tag)
}

func TestTransitiveResolutionAndUpdatePolicy(t *testing.T) {
	b := newTestGitRepo(t)
	b.release("v1.0.0", `schema: 2
package:
  name: B
components:
  b::core:
    kind: driver
`)
	b.release("v1.2.0", `schema: 2
package:
  name: B
components:
  b::core:
    kind: driver
`)
	b.release("v2.0.0", `schema: 2
package:
  name: B
components:
  b::core:
    kind: driver
`)

	a := newTestGitRepo(t)
	a.release("v1.0.0", `schema: 2
package:
  name: A
dependencies:
  b:
    type: git
    uri: `+fileURI(b.path)+`
    version: ^1.0.0
    components:
      - b::core
components:
  a::core:
    kind: module
`)

	root := t.TempDir()
	p := OpenProject(root)
	cfg := &Config{Schema: ConfigSchema, Project: ProjectConfig{Environment: "generic", Target: "app", BuildDir: "build", DepsDir: "deps"}, Dependencies: map[string]*DependencyConfig{
		"a": {Type: "git", URI: fileURI(a.path), Version: "^1.0.0", Components: []string{"a::core"}},
	}, DependencyOrder: []string{"a"}}
	lock, err := p.ResolveDependencies(cfg, nil, ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := lock.Dependencies["b"].Resolved; got != "v1.2.0" {
		t.Fatalf("b resolved %q, want v1.2.0", got)
	}
	if len(lock.DependencyOrder) != 2 || lock.DependencyOrder[0] != "b" || lock.DependencyOrder[1] != "a" {
		t.Fatalf("dependency order = %#v, want child before parent", lock.DependencyOrder)
	}
	if !containsString(lock.Dependencies["b"].Parents, "a") {
		t.Fatalf("b parents = %#v", lock.Dependencies["b"].Parents)
	}

	b.release("v1.3.0", `schema: 2
package:
  name: B
components:
  b::core:
    kind: driver
`)
	kept, err := p.ResolveDependencies(cfg, lock, ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := kept.Dependencies["b"].Resolved; got != "v1.2.0" {
		t.Fatalf("sync should preserve lock, got %q", got)
	}
	updated, err := p.ResolveDependencies(cfg, lock, ResolveOptions{Update: map[string]bool{"b": true}})
	if err != nil {
		t.Fatal(err)
	}
	if got := updated.Dependencies["b"].Resolved; got != "v1.3.0" {
		t.Fatalf("update should select v1.3.0, got %q", got)
	}
}

func TestTransitiveConflictExplainsParents(t *testing.T) {
	b := newTestGitRepo(t)
	b.release("v1.5.0", `schema: 2
package:
  name: B
components:
`)
	b.release("v2.1.0", `schema: 2
package:
  name: B
components:
`)

	makeParent := func(name, version string) *testGitRepo {
		r := newTestGitRepo(t)
		r.release("v1.0.0", `schema: 2
package:
  name: `+name+`
dependencies:
  b:
    type: git
    uri: `+fileURI(b.path)+`
    version: `+version+`
components:
`)
		return r
	}
	a := makeParent("A", "^1.0.0")
	cRepo := makeParent("C", "^2.0.0")
	p := OpenProject(t.TempDir())
	cfg := &Config{Schema: ConfigSchema, Project: ProjectConfig{Environment: "generic", Target: "app", BuildDir: "build", DepsDir: "deps"}, Dependencies: map[string]*DependencyConfig{
		"a": {Type: "git", URI: fileURI(a.path), Version: "v1.0.0"},
		"c": {Type: "git", URI: fileURI(cRepo.path), Version: "v1.0.0"},
	}, DependencyOrder: []string{"a", "c"}}
	_, err := p.ResolveDependencies(cfg, nil, ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "dependency conflict: b") || !strings.Contains(err.Error(), "a -> b requires ^1.0.0") || !strings.Contains(err.Error(), "c -> b requires ^2.0.0") {
		t.Fatalf("unexpected conflict error: %v", err)
	}
}

func TestPathDependencyLockStaysConfigCompatibleButSyncReresolves(t *testing.T) {
	root := t.TempDir()
	depDir := filepath.Join(root, "local-lib")
	if err := os.MkdirAll(depDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `schema: 2
package:
  name: LocalLib
components:
  local::core:
    kind: driver
`
	if err := os.WriteFile(filepath.Join(depDir, "zap-package.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	p := OpenProject(root)
	cfg := &Config{Schema: ConfigSchema, Project: ProjectConfig{Environment: "generic", Target: "app", BuildDir: "build", DepsDir: "deps"}, Dependencies: map[string]*DependencyConfig{
		"local": {Type: "path", URI: "local-lib", Components: []string{"local::core"}},
	}, DependencyOrder: []string{"local"}}
	lock, err := p.ResolveDependencies(cfg, nil, ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := lock.Dependencies["local"].URI; got != "local-lib" {
		t.Fatalf("direct path lock URI = %q, want portable manifest path", got)
	}
	if !lockCompatibleWithConfig(lock, cfg) {
		t.Fatal("path lock should be structurally compatible with zap.yml")
	}
	if !lockContainsMutablePath(lock) {
		t.Fatal("path dependency should force sync-time re-resolution")
	}
}

func TestRootZephyrIntentIsDistinctFromTransitiveRequirement(t *testing.T) {
	lock := &Lockfile{Schema: LockSchema, Dependencies: map[string]*LockedDependency{
		"shared": {
			Type: "git", URI: "https://example.invalid/shared.git", Requested: []string{"^1.0.0"}, RootRequested: "^1.0.0",
			Resolved: "v1.0.0", Commit: "0123456789abcdef0123456789abcdef01234567", Direct: true,
			ZephyrModule: true, RootZephyrModule: false, Parents: []string{"root", "parent"},
		},
	}, DependencyOrder: []string{"shared"}}
	cfg := &Config{Schema: ConfigSchema, Dependencies: map[string]*DependencyConfig{
		"shared": {Type: "git", URI: "https://example.invalid/shared.git", Version: "^1.0.0", ZephyrModule: false},
	}, DependencyOrder: []string{"shared"}}
	if !lockCompatibleWithConfig(lock, cfg) {
		t.Fatal("aggregate transitive zephyr requirement must not make direct zap.yml intent look stale")
	}
	cfg.Dependencies["shared"].ZephyrModule = true
	if lockCompatibleWithConfig(lock, cfg) {
		t.Fatal("changing direct zephyr_module intent must stale the lock")
	}
}

func TestLockRejectsFilesystemNameCollisions(t *testing.T) {
	lock := &Lockfile{Schema: LockSchema, Dependencies: map[string]*LockedDependency{
		"foo-bar": {Type: "path", URI: "../one"},
		"foo_bar": {Type: "path", URI: "../two"},
	}, DependencyOrder: []string{"foo-bar", "foo_bar"}}
	if err := validateLock(lock); err == nil || !strings.Contains(err.Error(), "collide") {
		t.Fatalf("expected collision error, got %v", err)
	}
}

func TestDirectAndTransitivePathRequirementCanShareSamePackage(t *testing.T) {
	root := t.TempDir()
	appDir := filepath.Join(root, "app")
	aDir := filepath.Join(root, "a")
	bDir := filepath.Join(root, "b")
	for _, dir := range []string{appDir, aDir, bDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(bDir, "zap-package.yml"), []byte(`schema: 2
package:
  name: B
components:
  b::core:
    kind: driver
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(aDir, "zap-package.yml"), []byte(`schema: 2
package:
  name: A
dependencies:
  b:
    type: path
    uri: ../b
    components:
      - b::core
components:
  a::core:
    kind: module
`), 0o644); err != nil {
		t.Fatal(err)
	}

	p := OpenProject(appDir)
	cfg := &Config{Schema: ConfigSchema, Project: ProjectConfig{Environment: "generic", Target: "app", BuildDir: "build", DepsDir: "deps"}, Dependencies: map[string]*DependencyConfig{
		"a": {Type: "path", URI: "../a", Components: []string{"a::core"}},
		"b": {Type: "path", URI: "../b", Components: []string{"b::core"}},
	}, DependencyOrder: []string{"a", "b"}}
	lock, err := p.ResolveDependencies(cfg, nil, ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	b := lock.Dependencies["b"]
	if b == nil || !b.Direct || b.URI != "../b" {
		t.Fatalf("unexpected shared path lock: %#v", b)
	}
	if !containsString(b.Parents, "root") || !containsString(b.Parents, "a") {
		t.Fatalf("shared path parents = %#v, want root and a", b.Parents)
	}
}
