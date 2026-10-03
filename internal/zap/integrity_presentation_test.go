// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditDiffContextAndInvisibleChanges(t *testing.T) {
	file := &integrityFile{Root: "project", Path: "source.c"}
	ch := integrityChange{Kind: "MODIFIED", Path: "project/source.c", Before: file, After: file}
	before := []byte(strings.Repeat("unchanged\n", 20) + "old\n" + strings.Repeat("tail\n", 20))
	after := []byte(strings.Repeat("unchanged\n", 20) + "new\n" + strings.Repeat("tail\n", 20))
	var b bytes.Buffer
	writeAuditDiff(&b, ch, before, after, false)
	text := b.String()
	if !strings.Contains(text, "@@ -18,7 +18,7 @@") || !strings.Contains(text, "-old\n+new\n") || strings.Count(text, "unchanged") != 3 || strings.Count(text, "tail") != 3 {
		t.Fatalf("bad context diff: %s", text)
	}
	b.Reset()
	writeAuditDiff(&b, ch, []byte("line\r\n"), []byte("line "), true)
	for _, want := range []string{"line\\r", "line·", "No newline at end of file", ansiRed, ansiGreen} {
		if !strings.Contains(b.String(), want) {
			t.Fatalf("missing %q: %s", want, b.String())
		}
	}
	b.Reset()
	writeAuditDiff(&b, ch, []byte("safe\n"), []byte("\x1b[2Junsafe\n"), false)
	if strings.Contains(b.String(), "\x1b") || !strings.Contains(b.String(), "\\u001b") {
		t.Fatalf("unsafe diff: %q", b.String())
	}
}

func TestAuditLineDiffPreservesBothSides(t *testing.T) {
	samples := [][]string{nil, {"a\n"}, {"b\n"}, {"a\n", "b\n", "a\n"}, {"a\n", "c", "b\n"}}
	for _, a := range samples {
		for _, b := range samples {
			var old, new strings.Builder
			for _, op := range auditLineDiff(a, b) {
				if op.kind != '+' {
					old.WriteString(op.text)
				}
				if op.kind != '-' {
					new.WriteString(op.text)
				}
			}
			if old.String() != strings.Join(a, "") || new.String() != strings.Join(b, "") {
				t.Fatal("diff lost file contents")
			}
		}
	}
	a, b := make([]string, 1500), make([]string, 1500)
	for i := range a {
		a[i] = "old\n"
		b[i] = "new\n"
	}
	ops := auditLineDiff(a, b)
	if len(ops) != 3000 {
		t.Fatalf("large-file fallback lost changes: %d", len(ops))
	}
}

func TestAuditReportCompleteAndConsoleBounded(t *testing.T) {
	root := t.TempDir()
	h, err := newHistory(root, []string{"audit"})
	if err != nil {
		t.Fatal(err)
	}
	previous := currentHistory
	currentHistory = h
	defer func() { currentHistory = previous; h.events.Close() }()
	m := &integrityManifest{Schema: 1, Roots: []integrityRoot{{ID: "project", Path: root}}}
	for i := 0; i < 2310; i++ {
		m.Files = append(m.Files, integrityFile{Root: "project", Path: fmt.Sprintf("file-%04d.c", i), SHA256: historyDigest([]byte("source"))})
	}
	changes := integrityChanges(nil, m)
	path, err := auditReport(root, nil, m, changes, nil)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(h.dir, "audit-report.json") {
		t.Fatal(path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		After   integrityManifest
		Changes []integrityChange
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.After.Files) != 2310 || len(report.Changes) != 2310 {
		t.Fatal("report truncated")
	}
	out, err := os.CreateTemp(t.TempDir(), "console")
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = out
	t.Setenv("NO_COLOR", "1")
	auditSummary(m, changes, true)
	auditSummary(m, changes, false)
	os.Stdout = old
	out.Close()
	text, _ := os.ReadFile(out.Name())
	if strings.Count(string(text), "file-") != 8 || !strings.Contains(string(text), "2310 files") || !strings.Contains(string(text), "2302 more") {
		t.Fatalf("unbounded summary: %s", text)
	}
	events, _ := os.ReadFile(filepath.Join(h.dir, "events.jsonl"))
	if !bytes.Contains(events, []byte("audit-report.json")) {
		t.Fatal("report absent from operation events")
	}
}

func TestAuditHistoryConsoleAndProgress(t *testing.T) {
	h, err := newHistory(t.TempDir(), []string{"audit"})
	if err != nil {
		t.Fatal(err)
	}
	defer h.events.Close()
	oldOut, oldErr := os.Stdout, os.Stderr
	stop, err := h.captureOutput()
	if err != nil {
		t.Fatal(err)
	}
	if uiConsole(os.Stdout) != oldOut || uiConsole(os.Stderr) != oldErr {
		stop()
		t.Fatal("lost terminal identity")
	}
	t.Setenv("NO_COLOR", "1")
	finish := auditProgress("Test scan")
	finish()
	finish()
	stop()
	if historyConsoleOut != nil || historyConsoleErr != nil {
		t.Fatal("console identity leaked")
	}
	text, err := os.ReadFile(filepath.Join(h.dir, "stdout.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(text, []byte("Test scan")) || bytes.Contains(text, []byte("\x1b[2K")) {
		t.Fatalf("incorrect progress recording: %q", text)
	}
}
