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

func TestGitIgnoresInheritedRepositorySelection(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	intended, other := filepath.Join(root, "intended"), filepath.Join(root, "other")
	for _, path := range []string{intended, other} {
		if out, err := exec.Command(git, "init", "-q", path).CombinedOutput(); err != nil {
			t.Fatalf("init: %v: %s", err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(intended, "tracked.txt"), []byte("intended source"), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(git, "-C", intended, "add", "tracked.txt").CombinedOutput(); err != nil {
		t.Fatalf("stage fixture: %v: %s", err, out)
	}
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)
	t.Setenv("GIT_COMMON_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_INDEX_FILE", filepath.Join(other, ".git", "index"))
	t.Setenv("GIT_OBJECT_DIRECTORY", filepath.Join(other, ".git", "objects"))
	t.Setenv("GIT_ALTERNATE_OBJECT_DIRECTORIES", filepath.Join(other, ".git", "objects"))
	contents, err := runCapture("", git, "-C", intended, "show", ":tracked.txt")
	if err != nil || contents != "intended source" {
		t.Fatalf("Git read a redirected index/object database: %q, %v", contents, err)
	}
	out, err := runCapture("", git, "-C", intended, "rev-parse", "--show-toplevel")
	if err != nil {
		t.Fatal(err)
	}
	a, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.Stat(intended)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(a, b) {
		t.Error("captured Git command was redirected to another repository")
	}
	if err := runStreaming("", git, "-C", intended, "config", "--local", "zap.boundary-test", "intended"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{intended, other} {
		data, err := os.ReadFile(filepath.Join(path, ".git", "config"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "boundary-test") != (path == intended) {
			t.Errorf("streaming Git wrote the wrong config: %s", path)
		}
	}
}

func TestCleanProtectsResolvedProjectPaths(t *testing.T) {
	for _, scenario := range []string{"build-through-link", "project-through-link", "ordinary-build"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := t.TempDir()
			container := filepath.Join(fixture, "container")
			project := filepath.Join(container, "project")
			if err := os.MkdirAll(project, 0700); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(project, "source.txt")
			if err := os.WriteFile(marker, []byte("keep"), 0600); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(fixture, "alias")
			if runtime.GOOS == "windows" {
				// Junctions work without the symlink privilege. Both arguments are
				// generated disposable paths; no project or user input reaches cmd.
				if out, err := exec.Command("cmd", "/d", "/c", "mklink", "/J", link, container).CombinedOutput(); err != nil {
					t.Fatalf("junction fixture: %v: %s", err, out)
				}
			} else if err := os.Symlink(container, link); err != nil {
				t.Fatal(err)
			}
			root, build := project, filepath.Join(link, "project")
			if scenario == "project-through-link" {
				root, build = filepath.Join(link, "project"), container
			}
			if scenario == "ordinary-build" {
				root, build = filepath.Join(link, "project"), filepath.Join(project, "build")
				if err := os.Mkdir(build, 0700); err != nil {
					t.Fatal(err)
				}
			}
			// Check the actual deletion target stays within this disposable fixture.
			resolved, err := filepath.EvalSymlinks(build)
			if err != nil {
				t.Fatal(err)
			}
			resolvedFixture, err := filepath.EvalSymlinks(fixture)
			if err != nil {
				t.Fatal(err)
			}
			rel, err := filepath.Rel(resolvedFixture, resolved)
			if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
				t.Fatal("unsafe fixture")
			}
			err = safeRemoveBuildDir(root, build)
			if scenario == "ordinary-build" {
				if err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(build); !os.IsNotExist(err) {
					t.Fatal("build was not removed")
				}
			} else if err == nil {
				t.Error("accepted an alias of project root or ancestor")
			}
			if _, err := os.Stat(marker); err != nil {
				t.Error("deleted disposable project source")
			}
		})
	}
}
