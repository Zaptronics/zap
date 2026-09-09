// SPDX-License-Identifier: Apache-2.0
package zap

import "testing"

func TestAddRemoveAcceptNameBeforeFlags(t *testing.T) {
	p := OpenProject(t.TempDir())
	cfg := &Config{
		Schema: ConfigSchema,
		Project: ProjectConfig{
			Environment: "generic",
			Target:      "app",
			BuildDir:    "build",
			DepsDir:     "deps",
			AdaptersDir: "adapters",
		},
		Build:        BuildConfig{Configuration: "Release"},
		Dependencies: map[string]*DependencyConfig{},
	}
	if err := p.WriteConfig(cfg); err != nil {
		t.Fatal(err)
	}

	if err := runAdd(p, []string{"local", "--path", "../local", "--no-sync"}); err != nil {
		t.Fatal(err)
	}
	got, err := p.ReadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if dep := got.Dependencies["local"]; dep == nil || dep.Type != "path" || dep.URI != "../local" {
		t.Fatalf("unexpected added dependency: %#v", dep)
	}

	if err := runRemove(p, []string{"local", "--no-sync"}); err != nil {
		t.Fatal(err)
	}
	got, err = p.ReadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.Dependencies["local"] != nil {
		t.Fatalf("dependency was not removed: %#v", got.Dependencies["local"])
	}
}
