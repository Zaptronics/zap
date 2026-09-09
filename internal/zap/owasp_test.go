//go:build claims && owasp

// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Adversarial acceptance tests for the OWASP review. Failures identify gaps;
// all writes, including the cleanup probe, are confined to disposable fixtures.
func TestOWASPCleanProtectsProjectAncestor(t *testing.T) {
	fixture := t.TempDir()
	parent := filepath.Join(fixture, "disposable-parent")
	project := filepath.Join(parent, "project")
	marker := filepath.Join(project, "source.txt")
	claimWrite(t, marker, "keep")
	// Validate the destructive test target before calling production code.
	rel, err := filepath.Rel(fixture, parent)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		t.Fatal("unsafe test fixture")
	}
	if err := safeRemoveBuildDir(project, parent); err == nil {
		t.Error("clean accepted an ancestor of the project")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Error("clean deleted the disposable project's source")
	}
}

func TestOWASPDuplicateMetadataRejected(t *testing.T) {
	t.Run("project", func(t *testing.T) {
		_, err := ParseConfig("schema: 5\nproject:\n  target: app\ndependencies:\n  demo:\n    type: path\n    uri: ../reviewed\n    uri: ../replacement\n")
		if err == nil {
			t.Fatal("duplicate URI silently accepted; last value wins")
		}
	})
	t.Run("remote", func(t *testing.T) {
		_, err := ParsePackageManifest("schema: 2\npackage:\n  name: demo\ndependencies:\n  child:\n    type: git\n    uri: https://reviewed.invalid/repo\n    uri: https://replacement.invalid/repo\n    version: v1.0.0\n")
		if err == nil {
			t.Fatal("duplicate remote dependency URI silently accepted")
		}
	})
	t.Run("lock", func(t *testing.T) {
		_, err := ParseLock("schema: 1\ndependencies:\n  demo:\n    type: path\n    uri: ../reviewed\n  demo:\n    type: path\n    uri: ../replacement\n")
		if err == nil {
			t.Fatal("duplicate locked dependency silently accepted")
		}
	})
}

func TestOWASPFetchContentNamesCannotAlias(t *testing.T) {
	cmake, err := exec.LookPath("cmake")
	if err != nil {
		t.Fatal("review requires CMake")
	}
	p := OpenProject(t.TempDir())
	left, right := filepath.Join(p.Root, "left"), filepath.Join(p.Root, "right")
	claimWrite(t, filepath.Join(left, "CMakeLists.txt"), "file(WRITE \"${CMAKE_BINARY_DIR}/left.marker\" \"included\")\n")
	claimWrite(t, filepath.Join(right, "CMakeLists.txt"), "file(WRITE \"${CMAKE_BINARY_DIR}/right.marker\" \"included\")\n")
	claimWrite(t, p.CMakePath, "cmake_minimum_required(VERSION 3.20)\nproject(app LANGUAGES NONE)\nadd_custom_target(app)\n")
	c := claimConfig("path", left)
	c.Dependencies = map[string]*DependencyConfig{"Demo": {Type: "path", URI: left}, "demo": {Type: "path", URI: right}}
	c.DependencyOrder = []string{"Demo", "demo"}
	if err := p.WriteConfig(c); err != nil {
		return
	}
	if err := p.Sync(c); err != nil {
		return
	} // Rejecting the collision is secure.
	if err := p.Generate(c); err != nil {
		return
	}
	build := filepath.Join(p.Root, "build")
	binary := filepath.Join(t.TempDir(), "zap.exe")
	buildZap := exec.Command("go", "build", "-o", binary, "./cmd/zap")
	buildZap.Dir = filepath.Join("..", "..")
	if out, err := buildZap.CombinedOutput(); err != nil {
		t.Fatalf("verifier fixture: %v\n%s", err, out)
	}
	cmd := exec.Command(cmake, "-S", p.Root, "-B", build, "-DZAP_EXECUTABLE="+binary)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("CMake fixture failed: %v\n%s", err, out)
	}
	for _, name := range []string{"left.marker", "right.marker"} {
		if _, err := os.Stat(filepath.Join(build, name)); err != nil {
			t.Errorf("two accepted dependency identities silently collapsed; %s missing", name)
		}
	}
}

func TestOWASPAuditReportsTruncatedInput(t *testing.T) {
	root := t.TempDir()
	claimWrite(t, filepath.Join(root, "CMakeLists.txt"), "#"+strings.Repeat("x", 70*1024)+"\nFetchContent_Declare(hidden URL https://hidden.invalid/source.tar.gz)\n")
	sources, err := scanExternalSources(root)
	if err == nil && len(sources) == 0 {
		t.Fatal("audit silently reports no sources after Scanner token overflow")
	}
}

func TestOWASPGitRewriteCannotBypassLocalOptIn(t *testing.T) {
	_, _, repo := claimFixture(t)
	t.Setenv("ZAP_ALLOW_LOCAL_GIT", "")
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "url."+fileURI(repo.path)+".insteadOf")
	t.Setenv("GIT_CONFIG_VALUE_0", "https://review.invalid/repo")
	// Exact rewrite targets our local fixture; no network request is made.
	tags, err := listStableTags(repo.git, "https://review.invalid/repo")
	if err == nil && len(tags) > 0 {
		t.Fatal("declared HTTPS source used local Git through inherited insteadOf configuration despite opt-in being disabled")
	}
}

func TestOWASPVerifyDoesNotExecuteLocalFSMonitor(t *testing.T) {
	p, c, repo := claimFixture(t)
	fixture := t.TempDir()
	marker := filepath.Join(fixture, "hook-ran")
	hook := filepath.Join(fixture, "fsmonitor-hook")
	// Git for Windows runs its hooks with bundled sh. Payload only writes a marker.
	claimWrite(t, hook, "#!/bin/sh\nprintf 'ran' > '"+strings.ReplaceAll(filepath.ToSlash(marker), "'", "'\"'\"'")+"'\nexit 0\n")
	if err := os.Chmod(hook, 0o755); err != nil {
		t.Fatal(err)
	}
	dep := filepath.Join(p.Root, "deps", "demo")
	for _, pair := range [][2]string{{"core.fsmonitor", filepath.ToSlash(hook)}, {"core.fsmonitorHookVersion", "1"}} {
		cmd := exec.Command(repo.git, "-C", dep, "config", pair[0], pair[1])
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("hook fixture: %v\n%s", err, out)
		}
	}
	_ = p.Verify(c, VerifyOptions{Offline: true})
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("read-only offline verification executed checkout-controlled fsmonitor hook")
	}
}

func TestOWASPSourceCredentialsNotPersisted(t *testing.T) {
	uri := "https://review-user:OWASP_SYNTHETIC_SECRET@example.invalid/repo"
	if err := validateSourceURI("git", uri); err == nil {
		c := claimConfig("git", uri)
		if strings.Contains(FormatConfig(c), "OWASP_SYNTHETIC_SECRET") {
			t.Fatal("credential-bearing source URI accepted and persisted verbatim")
		}
	}
}

func TestOWASPCommandErrorsRedactCredentials(t *testing.T) {
	_, err := runCapture("", filepath.Join(t.TempDir(), "nonexistent-tool"), "https://user:OWASP_SYNTHETIC_SECRET@example.invalid/repo")
	if err == nil {
		t.Fatal("missing executable unexpectedly succeeded")
	}
	if strings.Contains(err.Error(), "OWASP_SYNTHETIC_SECRET") {
		t.Fatal("command error includes synthetic source password verbatim")
	}
}

func TestOWASPCopyProtectsSameFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "firmware.uf2")
	claimWrite(t, file, "firmware fixture")
	_ = copyFile(file, file)
	if got := string(claimRead(t, file)); got != "firmware fixture" {
		t.Fatal("copying an artifact onto itself truncated it")
	}
}

func TestOWASPSemverRejectsIntegerOverflow(t *testing.T) {
	if _, ok := parseSemver(strings.Repeat("9", 100) + ".1.0"); ok {
		t.Fatal("oversized semver integer accepted after ignored Atoi error")
	}
	maxInt := int(^uint(0) >> 1)
	if _, err := parseVersionSpec("^" + strconv.Itoa(maxInt) + ".0.0"); err == nil {
		t.Fatal("compatible range overflow accepted at maximum integer major version")
	}
}

func TestOWASPReleaseTagNotInterpolatedIntoPowerShell(t *testing.T) {
	workflow := string(claimRead(t, filepath.Join("..", "..", ".github", "workflows", "release.yml")))
	unsafe := []string{
		"$version = '${{ github.ref_name }}'",
		"$tag = '${{ github.ref_name }}'",
	}
	for _, needle := range unsafe {
		if strings.Contains(workflow, needle) {
			t.Fatalf("release workflow interpolates the Git tag directly into PowerShell source: %s", needle)
		}
	}
	if !strings.Contains(workflow, "ZAP_RELEASE_TAG: ${{ github.ref_name }}") {
		t.Fatal("release workflow does not pass the Git tag through the reviewed environment variable")
	}
	if !strings.Contains(workflow, `^v[0-9]+\.[0-9]+\.[0-9]+$`) {
		t.Fatal("release workflow does not enforce the stable vMAJOR.MINOR.PATCH tag format")
	}
}

func TestOWASPReleaseTagCannotInjectPowerShell(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	ps, err := exec.LookPath("powershell")
	if err != nil {
		t.Skip("dynamic PowerShell reproduction is Windows-specific")
	}
	tag := "v1';Write-Output('OWASP_TAG_EXECUTED');#"
	if out, err := exec.Command(git, "check-ref-format", "refs/tags/"+tag).CombinedOutput(); err != nil {
		t.Fatalf("tag fixture invalid: %v\n%s", err, out)
	}
	// The workflow now consumes the tag from an environment variable and validates
	// the stable release form before PowerShell uses it. Reproduce the PowerShell
	// assignment with the hostile value to prove it remains data, not script text.
	cmd := `$version = $env:ZAP_RELEASE_TAG.TrimStart('v'); Write-Output $version`
	c := exec.Command(ps, "-NoProfile", "-NonInteractive", "-Command", cmd)
	c.Env = append(os.Environ(), "ZAP_RELEASE_TAG="+tag)
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("PowerShell fixture error: %v\n%s", err, out)
	}
	if got, want := strings.TrimSpace(string(out)), strings.TrimPrefix(tag, "v"); got != want {
		t.Fatalf("environment-provided Git tag was not treated as literal data: got %q want %q", got, want)
	}
}

func TestOWASPLockTempCannotClobberLinkedFile(t *testing.T) {
	fixture := t.TempDir()
	p := OpenProject(filepath.Join(fixture, "project"))
	if err := os.MkdirAll(p.Root, 0o755); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(fixture, "unrelated.txt")
	claimWrite(t, victim, "preserve unrelated data")
	if err := os.Link(victim, p.LockPath+".tmp"); err != nil {
		t.Skipf("hardlink fixture unavailable: %v", err)
	}
	_ = p.WriteLock(&Lockfile{Schema: LockSchema, Dependencies: map[string]*LockedDependency{}})
	if string(claimRead(t, victim)) != "preserve unrelated data" {
		t.Fatal("predictable lock temporary file followed a hardlink and overwrote unrelated fixture data")
	}
}
