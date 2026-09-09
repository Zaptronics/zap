//go:build claims

// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Acceptance tests are split by intent: TestCurrentClaim* must pass for shipped
// guarantees; TestRoadmapClaim* remains failing until the stronger architecture exists.
func claimConfig(kind, uri string) *Config {
	return &Config{Schema: ConfigSchema, Project: ProjectConfig{Environment: "generic", Target: "app", BuildDir: "build", DepsDir: "deps"}, Dependencies: map[string]*DependencyConfig{
		"demo": {Type: kind, URI: uri, Version: "v1.0.0", Hash: "SHA256=" + strings.Repeat("a", 64)},
	}, DependencyOrder: []string{"demo"}}
}

func claimWrite(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func claimRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func claimFixture(t *testing.T) (*Project, *Config, *testGitRepo) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatal("claim suite requires Git")
	}
	// Do not inherit user Git hooks, filters, signing, or alternate object stores.
	for _, key := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT"} {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "absent-config"))
	t.Setenv("GIT_TEMPLATE_DIR", t.TempDir())
	t.Setenv("ZAP_ALLOW_LOCAL_GIT", "1")
	t.Setenv("ZAP_CACHE_DIR", t.TempDir())
	r := newTestGitRepo(t)
	r.run("config", "core.autocrlf", "false")
	r.release("v1.0.0", "schema: 2\npackage:\n  name: demo\n")
	p := OpenProject(t.TempDir())
	c := claimConfig("git", fileURI(r.path))
	if err := p.WriteConfig(c); err != nil {
		t.Fatal(err)
	}
	if err := p.Sync(c); err != nil {
		t.Fatal(err)
	}
	if err := p.Verify(c, VerifyOptions{Offline: true}); err != nil {
		t.Fatalf("pristine control: %v", err)
	}
	return p, c, r
}

func TestRoadmapClaimLockSchema2(t *testing.T) {
	p, _, _ := claimFixture(t)
	l, err := p.ReadLock()
	if err != nil {
		t.Fatal(err)
	}
	if l.Schema != 2 {
		t.Fatalf("planned lock schema 2 is not implemented; generated schema is %d", l.Schema)
	}
}

func TestCurrentClaimURLRequiresHTTPS(t *testing.T) {
	for _, uri := range []string{"http://example.invalid/a.tar.gz", "file:///tmp/a.tar.gz", "ftp://example.invalid/a.tar.gz"} {
		t.Run(uri, func(t *testing.T) {
			p := OpenProject(t.TempDir())
			if err := p.Sync(claimConfig("url", uri)); err == nil {
				t.Fatal("non-HTTPS archive accepted by sync")
			}
		})
	}
}

func TestCurrentClaimURLRequiresHash(t *testing.T) {
	for _, hash := range []string{"", "MD5=" + strings.Repeat("a", 32), "SHA256=garbage"} {
		t.Run(hash, func(t *testing.T) {
			c := claimConfig("url", "https://example.invalid/a.tar.gz")
			c.Dependencies["demo"].Hash = hash
			if err := OpenProject(t.TempDir()).Sync(c); err == nil {
				t.Fatal("invalid hash accepted")
			}
		})
	}
}

func TestCurrentClaimLocalGitRejectedByDefault(t *testing.T) {
	_, c, _ := claimFixture(t)
	t.Setenv("ZAP_ALLOW_LOCAL_GIT", "")
	if err := OpenProject(t.TempDir()).Sync(c); err == nil {
		t.Fatal("file:// Git source accepted without development escape hatch")
	}
}

func TestRoadmapClaimIndependentSourceTampering(t *testing.T) {
	for _, mode := range []string{"ordinary", "assume-unchanged", "ignored-file"} {
		t.Run(mode, func(t *testing.T) {
			p, c, r := claimFixture(t)
			before := claimRead(t, p.LockPath)
			dep := filepath.Join(p.Root, "deps", "demo")
			if mode == "assume-unchanged" {
				cmd := exec.Command(r.git, "-C", dep, "update-index", "--assume-unchanged", "VERSION")
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("fixture: %v: %s", err, out)
				}
			}
			if mode == "ignored-file" {
				claimWrite(t, filepath.Join(dep, ".git", "info", "exclude"), "injected.c\n")
				claimWrite(t, filepath.Join(dep, "injected.c"), "/* unexpected build input */\n")
			} else {
				claimWrite(t, filepath.Join(dep, "VERSION"), "tampered source\n")
			}
			if err := p.Verify(c, VerifyOptions{Offline: true}); err == nil {
				t.Error("modified materialised source accepted against unchanged trusted lock")
			}
			if !bytes.Equal(before, claimRead(t, p.LockPath)) {
				t.Error("verification modified the trusted lock")
			}
		})
	}
}

func TestCurrentClaimMissingGitSourceRejectedOnline(t *testing.T) {
	p, c, _ := claimFixture(t)
	// Rename only this test's disposable dependency; never remove user data.
	if err := os.Rename(filepath.Join(p.Root, "deps", "demo"), filepath.Join(p.Root, "saved-demo")); err != nil {
		t.Fatal(err)
	}
	if err := p.Verify(c, VerifyOptions{}); err == nil {
		t.Fatal("online verifier accepted missing managed source")
	}
}

func TestRoadmapClaimOfflineSyncFromWarmCache(t *testing.T) {
	p, _, r := claimFixture(t)
	before := claimRead(t, p.LockPath)
	if err := os.Rename(r.path, r.path+"-unavailable"); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(p.Root, "deps", "demo"), filepath.Join(p.Root, "saved-demo")); err != nil {
		t.Fatal(err)
	}
	if err := runSync(p, []string{"--offline", "--no-configure"}); err != nil {
		t.Fatalf("offline sync after successful online sync: %v", err)
	}
	if got := string(claimRead(t, filepath.Join(p.Root, "deps", "demo", "VERSION"))); got != "v1.0.0\n" {
		t.Fatalf("restored source = %q", got)
	}
	if !bytes.Equal(before, claimRead(t, p.LockPath)) {
		t.Fatal("offline sync changed lock")
	}
}

func TestCurrentClaimURLVerifyRequiresMaterialisedContent(t *testing.T) {
	p := OpenProject(t.TempDir())
	c := claimConfig("url", "https://example.invalid/a.tar.gz")
	if err := p.Sync(c); err != nil {
		t.Fatal(err)
	}
	if err := p.Verify(c, VerifyOptions{Offline: true}); err == nil {
		t.Fatal("offline URL verification passed without any downloaded content to hash")
	}
}

func TestCurrentClaimSyncPreservesCompatibleLock(t *testing.T) {
	p, c, r := claimFixture(t)
	c.Dependencies["demo"].Version = "^1.0.0"
	if err := p.Sync(c); err != nil {
		t.Fatal(err)
	}
	before := claimRead(t, p.LockPath)
	r.release("v1.1.0", "schema: 2\npackage:\n  name: demo\n")
	if err := p.Sync(c); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, claimRead(t, p.LockPath)) {
		t.Fatal("new compatible tag silently changed lock")
	}
	if got := string(claimRead(t, filepath.Join(p.Root, "deps", "demo", "VERSION"))); got != "v1.0.0\n" {
		t.Fatalf("source changed to %q", got)
	}
}

func TestCurrentClaimRemoteManifestRejectsExecutablePolicy(t *testing.T) {
	_, err := ParsePackageManifest("schema: 2\npackage:\n  name: demo\nupload:\n  default: injected\n")
	if err == nil {
		t.Fatal("remote package accepted project-level upload policy")
	}
}

func TestCurrentClaimGeneratedCMakeRejectsMissingGit(t *testing.T) {
	cmake, err := exec.LookPath("cmake")
	if err != nil {
		t.Fatal("CMake required for configure-time acceptance test")
	}
	p, c, _ := claimFixture(t)
	claimWrite(t, p.CMakePath, "cmake_minimum_required(VERSION 3.20)\nproject(app LANGUAGES NONE)\nadd_custom_target(app)\n")
	if err := p.Generate(c); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "zap.exe")
	build := exec.Command("go", "build", "-o", binary, "./cmd/zap")
	build.Dir = filepath.Join("..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build verifier: %v\n%s", err, out)
	}
	configure := func(buildDir string, extra ...string) (string, error) {
		args := []string{"-S", p.Root, "-B", filepath.Join(p.Root, buildDir), "-DZAP_EXECUTABLE=" + binary}
		cmd := exec.Command(cmake, append(args, extra...)...)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := configure("pristine"); err != nil {
		t.Fatalf("pristine configure control: %v\n%s", err, out)
	}
	if err := os.Rename(filepath.Join(p.Root, "deps", "demo"), filepath.Join(p.Root, "saved-demo")); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"online", "offline-bypass"} {
		t.Run(mode, func(t *testing.T) {
			var flags []string
			if mode == "offline-bypass" {
				flags = []string{"-DZAP_OFFLINE=ON", "-DZAP_VERIFY_DEPENDENCIES=OFF"}
			}
			out, err := configure(mode, flags...)
			if err == nil || !strings.Contains(out, "zap sync") {
				t.Fatalf("missing source should stop before fetch: %v\n%s", err, out)
			}
			if mode == "online" && !strings.Contains(out, "has no accessible managed Git checkout") {
				t.Fatalf("online verification did not reject the missing managed checkout: %v\n%s", err, out)
			}
			if mode == "offline-bypass" && !strings.Contains(out, "Managed Git dependency is missing CMakeLists.txt") {
				t.Fatalf("generated CMake did not reject the missing managed source: %v\n%s", err, out)
			}
			if _, err := os.Stat(filepath.Join(p.Root, mode, "_deps")); !os.IsNotExist(err) {
				t.Fatalf("unexpected FetchContent work: %v", err)
			}
		})
	}
}
