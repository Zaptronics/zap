// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func historyLockedState(dir string) (map[string]*LockedDependency, error) {
	data, err := os.ReadFile(filepath.Join(dir, "zap.lock"))
	if os.IsNotExist(err) {
		return map[string]*LockedDependency{}, nil
	}
	if err != nil {
		return nil, err
	}
	lock, err := ParseLock(string(data))
	if err != nil {
		return nil, err
	}
	return lock.Dependencies, nil
}
func (h *operationHistory) recordDependencyChanges() {
	before, err := historyLockedState(filepath.Join(h.dir, "before"))
	if err != nil {
		h.recordFailure(h.event(historyEvent{Kind: "dependency-summary-unavailable", Message: historyRedact(err.Error())}))
		return
	}
	after, err := historyLockedState(filepath.Join(h.dir, "after"))
	if err != nil {
		h.recordFailure(h.event(historyEvent{Kind: "dependency-summary-unavailable", Message: historyRedact(err.Error())}))
		return
	}
	names := map[string]bool{}
	for name := range before {
		names[name] = true
	}
	for name := range after {
		names[name] = true
	}
	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	for _, name := range sorted {
		a, _ := json.Marshal(before[name])
		b, _ := json.Marshal(after[name])
		if string(a) == string(b) {
			continue
		}
		h.recordFailure(h.event(historyEvent{Kind: "dependency-changed", Reference: name, Message: historyRedact("before: " + string(a) + "\nafter: " + string(b))}))
	}
}
func (h *operationHistory) recordExternalChanges() {
	records, err := readHistories(h.root)
	if err != nil {
		h.recordFailure(h.event(historyEvent{Kind: "comparison-unavailable", Message: historyRedact(err.Error())}))
		return
	}
	var prior historyEvent
	for _, r := range records {
		if filepath.Base(r.dir) == h.id || r.incomplete {
			continue
		}
		for _, e := range r.events {
			if (e.Kind == "finished" || e.Kind == "incomplete") && e.Time.After(prior.Time) {
				prior = e
			}
		}
	}
	if prior.State == nil {
		return
	}
	for _, name := range []string{"zap.yml", "zap.lock"} {
		data, err := os.ReadFile(filepath.Join(h.dir, "before", name))
		now := "absent"
		if err == nil {
			now = historyDigest(data)
		} else if !os.IsNotExist(err) {
			h.recordFailure(err)
			return
		}
		if previous, ok := prior.State[name]; ok && previous != now {
			h.recordFailure(h.event(historyEvent{Kind: "external-state-change", Reference: prior.ID, Message: fmt.Sprintf("%s differs from the previous completed operation; changed outside that recorded interval or by an overlapping operation. Actor unknown.", name)}))
		}
	}
}
func (h *operationHistory) recordBuildContext(c *Config, o MakeOptions) error {
	details := map[string]any{"configuration": o.Configuration, "board": o.Board, "build_dir": o.BuildDir, "generator": o.Generator, "defines": o.Defines, "project": c.Project, "build": c.Build}
	// This is observational context, not a reproducibility or source-snapshot guarantee.
	if git, err := findProgram("git"); err == nil {
		if head, err := runCapture(h.root, git, "rev-parse", "HEAD"); err == nil {
			details["project_commit"] = head
		} else {
			details["project_commit_error"] = err.Error()
		}
		if status, err := runCapture(h.root, git, "-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false", "status", "--porcelain", "--untracked-files=normal"); err == nil {
			details["project_status"] = status
		} else {
			details["project_status_error"] = err.Error()
		}
	}
	for _, name := range []string{"git", "cmake"} {
		if program, err := findProgram(name); err == nil {
			version, err := runCapture(h.root, program, "--version")
			if err == nil {
				details[name+"_version"] = strings.TrimSpace(version)
			}
		}
	}
	data, err := json.Marshal(details)
	if err != nil {
		return err
	}
	return h.event(historyEvent{Kind: "build-context", Message: historyRedact(string(data))})
}

func ensureHistoryIgnore(path string) error {
	const rules = "*\n!.gitignore\n!signers.json\n!hash.json\n"
	data, err := integrityRead(path)
	if err == nil {
		normalized := strings.ReplaceAll(string(data), "\r\n", "\n")
		if normalized == rules {
			return nil
		}
		if strings.TrimSpace(normalized) != "*" {
			return fmt.Errorf("%s has custom rules; retain private history exclusions and use the documented shared-file allowlist", path)
		}
		return integrityAtomicWrite(path, []byte(rules))
	}
	if !os.IsNotExist(err) {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(rules); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
