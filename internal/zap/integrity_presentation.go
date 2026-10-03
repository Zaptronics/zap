// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Resolve the console behind history's pipes, so capture does not disable colour.
var historyConsoleOut, historyConsoleErr *os.File

func uiConsole(f *os.File) *os.File {
	if f == os.Stdout && historyConsoleOut != nil {
		return historyConsoleOut
	}
	if f == os.Stderr && historyConsoleErr != nil {
		return historyConsoleErr
	}
	return f
}

// Only transient frames bypass recording. A persistent phase line is always logged.
// Call stop before printing other output or asking a terminal question.
func auditProgress(label string) func() {
	uiStep("Working", label)
	console := uiConsole(os.Stdout)
	if !ansiEnabled(console) || !enableANSI(console) {
		return func() {}
	}
	done, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(120 * time.Millisecond)
		defer ticker.Stop()
		frames := []string{"⚡ ·  ", "⚡ ·· ", "⚡ ···", "⚡  ··", "⚡   ·"}
		i := 0
		for {
			select {
			case <-done:
				fmt.Fprint(console, "\r\x1b[2K")
				return
			case <-ticker.C:
				fmt.Fprintf(console, "\r\x1b[2K  %s %s", paint(true, ansiBold+ansiYellow, frames[i%len(frames)]), truncateToDisplayWidth(label, 48))
				i++
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { close(done); <-stopped }) }
}

func auditScan(roots []integrityRoot, objects, label string) (*integrityManifest, error) {
	stop := auditProgress(label)
	defer stop()
	return scanIntegrity(roots, objects)
}

func auditPrompt(reader *bufio.Reader, label string) (string, error) {
	fmt.Println()
	uiWarning("Review required")
	return prompt(reader, label, "no")
}

// Quote control characters in paths/source text so a file cannot paint terminal UI.
func auditText(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			switch r {
			case '\t':
				b.WriteString("\\t")
			case '\r':
				b.WriteString("\\r")
			default:
				fmt.Fprintf(&b, "\\u%04x", r)
			}
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func auditSummary(m *integrityManifest, changes []integrityChange, initial bool, projectName ...string) {
	counts := map[string]int{}
	grouped := map[string][]integrityChange{}
	paths := map[string]string{}
	for _, r := range m.Roots {
		paths[r.ID] = r.Path
	}
	for _, f := range m.Files {
		counts[f.Root]++
	}
	for _, ch := range changes {
		f := ch.After
		if f == nil {
			f = ch.Before
		}
		grouped[f.Root] = append(grouped[f.Root], ch)
		if _, ok := counts[f.Root]; !ok {
			counts[f.Root] = 0
		}
	}
	for _, r := range m.Roots {
		if _, ok := counts[r.ID]; !ok {
			counts[r.ID] = 0
		}
	}
	ids := make([]string, 0, len(counts))
	for id := range counts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if initial {
		uiWarning("First baseline: review the source scope before signing")
	} else if len(changes) > 0 {
		uiWarning(fmt.Sprintf("%d source changes need review", len(changes)))
	}
	color := ansiEnabled(os.Stdout)
	for _, id := range ids {
		label := id
		if id == "project" {
			name := filepath.Base(paths[id])
			if len(projectName) > 0 {
				name = projectName[0]
			}
			label += ": " + name
		}
		fmt.Printf("  %s %s (%d files; %d changes)\n", paint(color, ansiCyan, "├─"), paint(color, ansiBold, auditText(label)), counts[id], len(grouped[id]))
		if paths[id] != "" {
			fmt.Printf("  │  %s\n", auditText(paths[id]))
		}
		if initial {
			continue
		}
		for i, ch := range grouped[id] {
			if i == 8 {
				fmt.Printf("  │  └─ … %d more changes in audit-report.json\n", len(grouped[id])-i)
				break
			}
			fmt.Printf("  │  ├─ %s %s\n", paint(color, ansiYellow, ch.Kind), auditText(ch.Path))
		}
	}
}

// The full inventory is an operation attachment, not thousands of console lines.
// It describes a scanned observation; the signed baseline remains hash.json.
func auditReport(root string, before, after *integrityManifest, changes []integrityChange, sources any) (string, error) {
	report := struct {
		Schema          int                `json:"schema"`
		Before          *integrityManifest `json:"before,omitempty"`
		After           *integrityManifest `json:"after"`
		Changes         []integrityChange  `json:"changes"`
		ExternalSources any                `json:"external_sources"`
	}{1, before, after, changes, sources}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	var path string
	if currentHistory != nil {
		path = filepath.Join(currentHistory.dir, "audit-report.json")
	} else {
		// Direct/internal callers do not have a CLI operation.
		dir, err := integrityDirs(root)
		if err != nil {
			return "", err
		}
		f, err := os.CreateTemp(dir, "audit-report-*.json")
		if err != nil {
			return "", err
		}
		path = f.Name()
		if err := f.Close(); err != nil {
			return "", err
		}
	}
	if err := integrityAtomicWrite(path, data); err != nil {
		return "", err
	}
	if currentHistory != nil {
		if err := currentHistory.event(historyEvent{Kind: "audit-report", Reference: "audit-report.json", Message: "Complete scanned file inventory, scope and changes; not a signed approval"}); err != nil {
			return "", err
		}
	}
	return path, nil
}
