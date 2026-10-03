// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHistoryHelperProcess(t *testing.T) {
	if os.Getenv("ZAP_HISTORY_TEST_CHILD") != "1" {
		return
	}
	fmt.Print("stdout marker\nhttps://user:")
	fmt.Print("PRIVATE_PASSWORD@example.invalid/repo\n")
	fmt.Fprintln(os.Stderr, "stderr marker")
	fmt.Print(strings.Repeat("x", 100000), "\nEND-OF-LONG-OUTPUT\n")
	if err := os.WriteFile("child-ran", []byte("yes"), 0600); err != nil {
		os.Exit(99)
	}
	if err := os.WriteFile("zap.lock", []byte("schema: 1\ndependencies:\n  lib:\n    type: git\n    uri: https://example.invalid/lib\n    resolved: v2.0.0\n    commit: "+strings.Repeat("b", 40)+"\n"), 0600); err != nil {
		os.Exit(98)
	}
	os.Exit(7)
}
func TestHistoryCLIIntegration(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "zap.exe")
	build := exec.Command("go", "build", "-o", binary, "./cmd/zap")
	build.Dir = filepath.Join("..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	root := t.TempDir()
	before := "schema: 1\ndependencies:\n  lib:\n    type: git\n    uri: https://example.invalid/lib\n    resolved: v1.0.0\n    commit: " + strings.Repeat("a", 40) + "\n"
	if err := os.WriteFile(filepath.Join(root, "zap.lock"), []byte(before), 0600); err != nil {
		t.Fatal(err)
	}
	run := func(dir string, args ...string) (string, string, error) {
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "ZAP_HISTORY_TEST_CHILD=1")
		var out, errout bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &errout
		err := cmd.Run()
		return out.String(), errout.String(), err
	}
	out, errout, err := run(root, "e", os.Args[0], "-test.run=^TestHistoryHelperProcess$", "--", "--token", "PRIVATE_ARGUMENT")
	if ExitCode(err) != 7 {
		t.Fatalf("exit code: %v\n%s", err, errout)
	}
	if !strings.Contains(out, "stdout marker") || errout != "stderr marker\n" {
		t.Fatal("live streams missing")
	}
	records, err := readHistories(root)
	if err != nil || len(records) != 1 {
		t.Fatalf("history: %v, %d records", err, len(records))
	}
	r := records[0]
	first, last := r.events[0], r.events[len(r.events)-1]
	if first.Kind != "started" || first.User == "" || first.Time.IsZero() {
		t.Fatal("missing attribution")
	}
	if last.Kind != "finished" || last.ExitCode == nil || *last.ExitCode != 7 || last.BuildSucceeded {
		t.Fatalf("bad completion: %+v", last)
	}
	encoded, _ := json.Marshal(r.events)
	if bytes.Contains(encoded, []byte("PRIVATE_ARGUMENT")) {
		t.Fatal("command secret leaked")
	}
	data, err := os.ReadFile(filepath.Join(r.dir, "stdout.log"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("PRIVATE_PASSWORD")) {
		t.Fatal("chunk-split secret leaked")
	}
	if !bytes.Contains(data, []byte(strings.Repeat("x", 100000))) || !bytes.Contains(data, []byte("END-OF-LONG-OUTPUT")) {
		t.Fatal("output truncated")
	}
	saved, err := os.ReadFile(filepath.Join(r.dir, "before", "zap.lock"))
	if err != nil || string(saved) != before {
		t.Fatal("before snapshot missing")
	}
	saved, err = os.ReadFile(filepath.Join(r.dir, "after", "zap.lock"))
	if err != nil || !strings.Contains(string(saved), "v2.0.0") {
		t.Fatal("failed command's after snapshot missing")
	}
	out, errout, err = run(root, "log", "diff", first.ID)
	if err != nil || !strings.Contains(out, "v1.0.0") || !strings.Contains(out, "v2.0.0") {
		t.Fatalf("diff: %v\n%s\n%s", err, out, errout)
	}
	if _, _, err = run(root, "log", "mark-good", first.ID, "-m", "not actually good"); err == nil {
		t.Fatal("failed command marked good")
	}
	if _, errout, err = run(root, "log", "-m", "board tested"); err != nil {
		t.Fatalf("note: %v\n%s", err, errout)
	}
	out, _, err = run(root, "log")
	if err != nil || !strings.Contains(out, "board tested") || !strings.Contains(out, "FAILED") {
		t.Fatalf("timeline: %v\n%s", err, out)
	}
	if _, _, err = run(root, "log", "show", "../../outside"); err == nil {
		t.Fatal("unsafe operation ID accepted")
	}
	blocked := t.TempDir()
	if err := os.WriteFile(filepath.Join(blocked, ".zap"), []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = run(blocked, "e", os.Args[0], "-test.run=^TestHistoryHelperProcess$"); err == nil {
		t.Fatal("command ran without recording")
	}
	if _, err = os.Stat(filepath.Join(blocked, "child-ran")); !os.IsNotExist(err) {
		t.Fatal("child launched when history initialization failed")
	}
}

func TestHistoryRecordingFailureIsExplicit(t *testing.T) {
	h, err := newHistory(t.TempDir(), []string{"e", "example"})
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(h.dir, "broken.log"))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	sink := &historyOutput{h: h, file: f}
	if _, err := sink.Write([]byte("output\n")); err != nil {
		t.Fatal("logging failure should not abort a running tool")
	}
	if err := h.finish(nil, nil); err == nil {
		t.Fatal("recording failure silently accepted")
	}
	records, err := readHistories(h.root)
	if err != nil {
		t.Fatal(err)
	}
	events := records[0].events
	if events[len(events)-1].Kind != "incomplete" {
		t.Fatal("incomplete recording not marked")
	}
}

func TestHistoryConcurrentOperationFiles(t *testing.T) {
	root := t.TempDir()
	// Distinct operation journals avoid shared append interleaving.
	a, err := newHistory(root, []string{"first"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := newHistory(root, []string{"second"})
	if err != nil {
		t.Fatal(err)
	}
	if a.id == b.id {
		t.Fatal("operation IDs collide")
	}
	if err := b.finish(nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := a.finish(nil, nil); err != nil {
		t.Fatal(err)
	}
	records, err := readHistories(root)
	if err != nil || len(records) != 2 {
		t.Fatalf("history: %v", err)
	}
	for _, r := range records {
		if len(r.events) != 2 || r.incomplete {
			t.Fatal("interleaved journal")
		}
	}
}

func TestHistorySuccessfulBuildAndKnownGood(t *testing.T) {
	if _, err := exec.LookPath("cmake"); err != nil {
		t.Skip("CMake required")
	}
	binary := filepath.Join(t.TempDir(), "zap.exe")
	build := exec.Command("go", "build", "-o", binary, "./cmd/zap")
	build.Dir = filepath.Join("..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	root := t.TempDir()
	p := OpenProject(root)
	c := &Config{Schema: ConfigSchema, Project: ProjectConfig{Target: "app", Environment: "generic", BuildDir: "build", DepsDir: "deps", AdaptersDir: "adapters"}, Build: BuildConfig{Configuration: "Release", CMake: map[string]string{}}, Dependencies: map[string]*DependencyConfig{}}
	if err := p.WriteConfig(c); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.CMakePath, []byte("cmake_minimum_required(VERSION 3.20)\nproject(history LANGUAGES NONE)\nadd_custom_target(app ALL COMMAND \"${CMAKE_COMMAND}\" -E echo built)\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "make", "--no-sync")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("make: %v\n%s", err, out)
	}
	records, err := readHistories(root)
	if err != nil {
		t.Fatal(err)
	}
	id := ""
	for _, r := range records {
		for _, e := range r.events {
			if e.Kind == "finished" && e.BuildSucceeded {
				id = e.ID
			}
		}
	}
	if id == "" {
		t.Fatal("successful build not recorded")
	}
	if _, err := os.Stat(filepath.Join(root, ".zap", "history", "operations", id, "build-input", "zap.yml")); err != nil {
		t.Fatal("build input snapshot missing")
	}
	cmd = exec.Command(binary, "log", "mark-good", id, "-m", "validated on test board")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("mark good: %v\n%s", err, out)
	}
	records, err = readHistories(root)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range records {
		for _, e := range r.events {
			if e.Kind == "known-good" && e.Reference == id {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("known-good annotation missing")
	}
}

func TestHistoryRootPrefersNearestGit(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	for _, dir := range []string{root, nested} {
		if err := os.MkdirAll(filepath.Join(dir, ".git"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if got := historyRoot(nested); got != nested {
		t.Fatalf("history root = %s, want %s", got, nested)
	}
}
