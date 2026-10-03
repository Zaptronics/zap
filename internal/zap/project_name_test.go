// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectNameRoundTripAndFallback(t *testing.T) {
	c, err := ParseConfig("schema: 5\nproject:\n  name: 'Hub: Controller'\n  environment: generic\n  target: firmware\n")
	if err != nil {
		t.Fatal(err)
	}
	next, err := ParseConfig(FormatConfig(c))
	if err != nil {
		t.Fatal(err)
	}
	if next.Project.Name != "Hub: Controller" || next.Project.Target != "firmware" {
		t.Fatalf("name/target changed: %+v", next.Project)
	}
	p := OpenProject(filepath.Join(t.TempDir(), "Hub - Copy"))
	if p.displayName(next) != "Hub: Controller" {
		t.Fatal(p.displayName(next))
	}
	next.Project.Name = ""
	if p.displayName(next) != "Hub - Copy" {
		t.Fatal("legacy folder fallback lost")
	}
	if strings.Contains(FormatConfig(next), "  name:") {
		t.Fatal("unconfigured name inserted")
	}
}

func TestRepositoryNameSuggestion(t *testing.T) {
	for remote, want := range map[string]string{
		"https://github.com/Zaptronics/Hub.git":             "Hub",
		"git@github.com:Zaptronics/Hub.git":                 "Hub",
		"ssh://git@example.com:2222/team/Hub.git":           "Hub",
		"https://example.com/team/Hub.git?token=secret#ref": "Hub",
		"C:\\repos\\Hub.git":                                "Hub",
		"/repos/Hub.git/":                                   "Hub",
		"https://example.com/":                              "",
		"https://example.com/Hub%20Controller.git":          "Hub Controller",
		"https://example.com/Hub%1b.git":                    "",
	} {
		if got := repositoryName(remote); got != want {
			t.Errorf("%q: got %q want %q", remote, got, want)
		}
	}
	fs, opts := InitFlagSet()
	if err := fs.Parse([]string{"--name", "Hub Controller", "--non-interactive"}); err != nil {
		t.Fatal(err)
	}
	if opts.Name != "Hub Controller" {
		t.Fatal("name override lost")
	}
}

func TestProjectNameFromLocalGit(t *testing.T) {
	git, err := findProgram("git")
	if err != nil {
		t.Skip(err)
	}
	root := t.TempDir()
	p := OpenProject(root)
	if got := p.suggestedName(""); got != filepath.Base(root) {
		t.Fatal(got)
	}
	if _, err := runCapture(root, git, "init"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCapture(root, git, "config", "remote.origin.url", "git@example.invalid:team/Original.git"); err != nil {
		t.Fatal(err)
	}
	if got := p.suggestedName(git); got != "Original" {
		t.Fatal(got)
	}
}
