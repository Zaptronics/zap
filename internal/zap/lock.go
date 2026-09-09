// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func ParseLock(text string) (*Lockfile, error) {
	if err := rejectDuplicateMappingKeys(text, "zap.lock"); err != nil {
		return nil, err
	}
	lock := &Lockfile{Schema: LockSchema, Dependencies: map[string]*LockedDependency{}}
	section := ""
	current := ""
	currentList := ""
	s := bufio.NewScanner(strings.NewReader(text))
	lineNo := 0
	for s.Scan() {
		lineNo++
		raw := strings.TrimSuffix(s.Text(), "\r")
		if strings.TrimSpace(raw) == "" || strings.HasPrefix(strings.TrimSpace(raw), "#") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		if indent%2 != 0 {
			return nil, fmt.Errorf("zap.lock line %d uses odd indentation; use two spaces per level", lineNo)
		}
		t := strings.TrimSpace(raw)
		if indent == 0 {
			current, currentList = "", ""
			if t == "dependencies:" {
				section = "dependencies"
				continue
			}
			k, v, ok := splitKV(t)
			if !ok || k != "schema" {
				return nil, fmt.Errorf("invalid top-level zap.lock line %d", lineNo)
			}
			n, err := parseInt(v)
			if err != nil {
				return nil, fmt.Errorf("invalid zap.lock schema on line %d", lineNo)
			}
			lock.Schema = n
			continue
		}
		if section != "dependencies" {
			return nil, fmt.Errorf("unsupported zap.lock structure on line %d", lineNo)
		}
		if indent == 2 && strings.HasSuffix(t, ":") {
			name := strings.TrimSpace(strings.TrimSuffix(t, ":"))
			if name == "" {
				return nil, fmt.Errorf("empty dependency name on zap.lock line %d", lineNo)
			}
			lock.Dependencies[name] = &LockedDependency{}
			lock.DependencyOrder = append(lock.DependencyOrder, name)
			current, currentList = name, ""
			continue
		}
		if current == "" {
			return nil, fmt.Errorf("zap.lock dependency setting on line %d appears before a dependency name", lineNo)
		}
		dep := lock.Dependencies[current]
		if indent == 4 {
			k, v, ok := splitKV(t)
			if !ok {
				return nil, fmt.Errorf("invalid zap.lock dependency setting on line %d", lineNo)
			}
			switch k {
			case "requested", "components", "root_components", "parents":
				if v != "" {
					return nil, fmt.Errorf("zap.lock line %d: %s must be a list", lineNo, k)
				}
				currentList = k
				continue
			case "type":
				dep.Type = v
			case "uri":
				dep.URI = v
			case "root_requested":
				dep.RootRequested = v
			case "resolved":
				dep.Resolved = v
			case "commit":
				dep.Commit = v
			case "hash":
				dep.Hash = v
			case "manifest_sha256":
				dep.ManifestSHA256 = strings.ToLower(v)
			case "direct":
				dep.Direct = boolScalar(v)
			case "zephyr_module":
				dep.ZephyrModule = boolScalar(v)
			case "root_zephyr_module":
				dep.RootZephyrModule = boolScalar(v)
			default:
				return nil, fmt.Errorf("unknown zap.lock dependency key %q on line %d", k, lineNo)
			}
			currentList = ""
			continue
		}
		if indent == 6 && strings.HasPrefix(t, "- ") && currentList != "" {
			item := scalar(strings.TrimSpace(strings.TrimPrefix(t, "- ")))
			switch currentList {
			case "requested":
				dep.Requested = append(dep.Requested, item)
			case "components":
				dep.Components = append(dep.Components, item)
			case "root_components":
				dep.RootComponents = append(dep.RootComponents, item)
			case "parents":
				dep.Parents = append(dep.Parents, item)
			}
			continue
		}
		return nil, fmt.Errorf("unsupported zap.lock structure on line %d: %s", lineNo, t)
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	lock.normalize()
	if err := validateLock(lock); err != nil {
		return nil, err
	}
	return lock, nil
}

func FormatLock(lock *Lockfile) string {
	if lock == nil {
		lock = &Lockfile{}
	}
	lock.normalize()
	lock.Schema = LockSchema
	var b strings.Builder
	b.WriteString("# Zap dependency lockfile. Commit this file. DO NOT EDIT BY HAND.\n")
	fmt.Fprintf(&b, "schema: %d\n\n", lock.Schema)
	b.WriteString("dependencies:\n")
	for _, name := range lock.DependencyOrder {
		dep := lock.Dependencies[name]
		if dep == nil {
			continue
		}
		fmt.Fprintf(&b, "  %s:\n", name)
		fmt.Fprintf(&b, "    type: %s\n", yamlQuote(dep.Type))
		if dep.URI != "" {
			fmt.Fprintf(&b, "    uri: %s\n", yamlQuote(dep.URI))
		}
		if dep.Direct {
			b.WriteString("    direct: true\n")
		}
		if dep.RootRequested != "" {
			fmt.Fprintf(&b, "    root_requested: %s\n", yamlQuote(dep.RootRequested))
		}
		if len(dep.Requested) > 0 {
			b.WriteString("    requested:\n")
			for _, v := range uniqueSorted(dep.Requested) {
				fmt.Fprintf(&b, "      - %s\n", yamlQuote(v))
			}
		}
		if dep.Resolved != "" {
			fmt.Fprintf(&b, "    resolved: %s\n", yamlQuote(dep.Resolved))
		}
		if dep.Commit != "" {
			fmt.Fprintf(&b, "    commit: %s\n", strings.ToLower(dep.Commit))
		}
		if dep.Hash != "" {
			fmt.Fprintf(&b, "    hash: %s\n", yamlQuote(dep.Hash))
		}
		if dep.ManifestSHA256 != "" {
			fmt.Fprintf(&b, "    manifest_sha256: %s\n", strings.ToLower(dep.ManifestSHA256))
		}
		if dep.ZephyrModule {
			b.WriteString("    zephyr_module: true\n")
		}
		if dep.RootZephyrModule {
			b.WriteString("    root_zephyr_module: true\n")
		}
		if len(dep.RootComponents) > 0 {
			b.WriteString("    root_components:\n")
			for _, c := range uniqueSorted(dep.RootComponents) {
				fmt.Fprintf(&b, "      - %s\n", yamlQuote(c))
			}
		}
		if len(dep.Components) > 0 {
			b.WriteString("    components:\n")
			for _, c := range uniqueSorted(dep.Components) {
				fmt.Fprintf(&b, "      - %s\n", yamlQuote(c))
			}
		}
		if len(dep.Parents) > 0 {
			b.WriteString("    parents:\n")
			for _, parent := range uniqueSorted(dep.Parents) {
				fmt.Fprintf(&b, "      - %s\n", yamlQuote(parent))
			}
		}
	}
	return b.String()
}

func validateLock(lock *Lockfile) error {
	if lock == nil {
		return fmt.Errorf("nil lockfile")
	}
	lock.normalize()
	if lock.Schema != LockSchema {
		return fmt.Errorf("unsupported zap.lock schema %d; this zap supports schema %d", lock.Schema, LockSchema)
	}
	seenSafeNames := map[string]string{}
	for _, name := range lock.DependencyOrder {
		safe := canonicalDependencyIdentity(name)
		if previous, ok := seenSafeNames[safe]; ok && previous != name {
			return fmt.Errorf("zap.lock dependency names %q and %q collide after CMake-safe case-folded normalization", previous, name)
		}
		seenSafeNames[safe] = name
	}
	for _, name := range lock.DependencyOrder {
		dep := lock.Dependencies[name]
		if dep == nil {
			return fmt.Errorf("zap.lock dependency %q is missing configuration", name)
		}
		if !dependencyNameRE.MatchString(name) {
			return fmt.Errorf("zap.lock dependency name %q is invalid", name)
		}
		if err := validateSourceURI(strings.ToLower(dep.Type), dep.URI); err != nil {
			return fmt.Errorf("zap.lock dependency %q: %w", name, err)
		}
		switch strings.ToLower(dep.Type) {
		case "git":
			if dep.URI == "" || dep.Commit == "" {
				return fmt.Errorf("zap.lock git dependency %q requires uri and commit", name)
			}
			if !gitCommitRE.MatchString(dep.Commit) {
				return fmt.Errorf("zap.lock dependency %q has invalid Git commit", name)
			}
		case "path":
			if dep.URI == "" {
				return fmt.Errorf("zap.lock path dependency %q requires uri", name)
			}
		case "url":
			if dep.URI == "" || !sha256RE.MatchString(dep.Hash) {
				return fmt.Errorf("zap.lock URL dependency %q requires uri and SHA256 hash", name)
			}
		default:
			return fmt.Errorf("zap.lock dependency %q has unsupported type %q", name, dep.Type)
		}
		if dep.ManifestSHA256 != "" && !regexpSHA256Hex.MatchString(dep.ManifestSHA256) {
			return fmt.Errorf("zap.lock dependency %q has invalid manifest_sha256", name)
		}
		for _, c := range append(append([]string{}, dep.Components...), dep.RootComponents...) {
			if !cmakeTargetRE.MatchString(c) {
				return fmt.Errorf("zap.lock dependency %q component %q is invalid", name, c)
			}
		}
	}
	return nil
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func equalStringSet(a, b []string) bool {
	aa := uniqueSorted(a)
	bb := uniqueSorted(b)
	if len(aa) != len(bb) {
		return false
	}
	for i := range aa {
		if aa[i] != bb[i] {
			return false
		}
	}
	return true
}

func containsString(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func (p *Project) ReadLock() (*Lockfile, error) {
	b, err := os.ReadFile(p.LockPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return ParseLock(string(b))
}

func (p *Project) WriteLock(lock *Lockfile) error {
	if err := validateLock(lock); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p.LockPath), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(p.LockPath), ".zap.lock-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := f.WriteString(FormatLock(lock)); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Chmod(0o644); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, p.LockPath); err != nil {
		return err
	}
	ok = true
	return nil
}
