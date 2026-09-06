// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandUploadString(t *testing.T) {
	got, err := expandUploadString("{build_dir}/{target}.elf", map[string]string{"build_dir": "/tmp/build", "target": "Hub"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "/tmp/build/Hub.elf" {
		t.Fatalf("got %q", got)
	}
	if _, err := expandUploadString("{unknown}", map[string]string{}); err == nil {
		t.Fatal("expected unknown placeholder error")
	}
}

func TestUploadCommandMethodWithConfiguredExecutable(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("shell-script fixture is Unix-only; Windows behaviour is covered by cross-compilation and config tests")
	}
	root := t.TempDir()
	build := filepath.Join(root, "build")
	if err := os.MkdirAll(build, 0o755); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(build, "app.bin")
	if err := os.WriteFile(artifact, []byte("firmware"), 0o644); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "upload.log")
	script := filepath.Join(root, "uploader.sh")
	body := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$ZAP_UPLOAD_TEST_LOG\"\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZAP_UPLOAD_TEST_LOG", logPath)
	t.Setenv("ZAP_TEST_SERIAL", "ABC123")
	cfg := &Config{
		Schema:  ConfigSchema,
		Project: ProjectConfig{Environment: "generic", Target: "app", BuildDir: "build", DepsDir: "deps", AdaptersDir: "adapters"},
		Build:   BuildConfig{Configuration: "Release"},
		Upload: UploadConfig{
			Default: "test",
			Methods: map[string]*UploadMethodConfig{
				"test": {
					Type:              "command",
					Artifact:          "{build_dir}/{target}.bin",
					Executable:        script,
					Args:              []string{"--file", "{artifact}"},
					Variables:         map[string]string{"serial": "env:ZAP_TEST_SERIAL"},
					VariableOrder:     []string{"serial"},
					OptionalArgs:      map[string][]string{"serial": []string{"--serial", "{serial}"}},
					OptionalArgsOrder: []string{"serial"},
				},
			},
			MethodOrder: []string{"test"},
		},
		Dependencies: map[string]*DependencyConfig{},
	}
	p := OpenProject(root)
	if err := p.Upload(cfg, UploadOptions{}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, want := range []string{"--file", artifact, "--serial", "ABC123"} {
		if !strings.Contains(text, want) {
			t.Fatalf("upload args missing %q: %s", want, text)
		}
	}
}

func TestOptionalUploadArgsCanBeInsertedInPlace(t *testing.T) {
	m := &UploadMethodConfig{
		Args:              []string{"-f", "interface.cfg", "{?probe_serial}", "-f", "target.cfg"},
		OptionalArgs:      map[string][]string{"probe_serial": {"-c", "adapter serial {probe_serial}"}},
		OptionalArgsOrder: []string{"probe_serial"},
	}
	args, err := expandUploadCommandArgs(m, map[string]string{"probe_serial": "XYZ"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-f", "interface.cfg", "-c", "adapter serial XYZ", "-f", "target.cfg"}
	if len(args) != len(want) {
		t.Fatalf("args = %#v", args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args[%d]=%q want %q (%#v)", i, args[i], want[i], args)
		}
	}
}
