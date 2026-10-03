// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type integrityRoot struct {
	ID      string   `json:"id"`
	Path    string   `json:"path"`
	Exclude []string `json:"exclude,omitempty"`
}
type integrityFile struct {
	Root       string `json:"root"`
	Path       string `json:"path"`
	SHA256     string `json:"sha256"`
	Executable bool   `json:"executable,omitempty"`
}
type integrityManifest struct {
	Schema int             `json:"schema"`
	Roots  []integrityRoot `json:"roots"`
	Files  []integrityFile `json:"files"`
}
type integrityChange struct {
	Kind          string
	Path          string
	Before, After *integrityFile
}

func integrityWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// Roots are bound to this checkout. No Git ignore rules or index flags are trusted.
func (p *Project) integrityRoots(c *Config) ([]integrityRoot, error) {
	root, err := filepath.Abs(p.Root)
	if err != nil {
		return nil, err
	}
	roots := []integrityRoot{{ID: "project", Path: root, Exclude: []string{p.buildPath(c, "")}}}
	lock, err := p.ReadLock()
	if err != nil {
		return nil, err
	}
	if lock == nil {
		if len(c.Dependencies) != 0 {
			return nil, fmt.Errorf("dependency lock missing; run zap sync")
		}
		return roots, nil
	}
	if err := validateLock(lock); err != nil {
		return nil, err
	}
	if !lockCompatibleWithConfig(lock, c) {
		return nil, fmt.Errorf("dependency lock does not match zap.yml; run zap sync")
	}
	// CMake can select a source override independently of the environment.
	cache := map[string]string{}
	data, err := os.ReadFile(filepath.Join(p.buildPath(c, ""), "CMakeCache.txt"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || strings.HasPrefix(key, "//") || strings.HasPrefix(key, "#") {
			continue
		}
		key, _, _ = strings.Cut(key, ":")
		cache[key] = value
	}
	for _, name := range lock.DependencyOrder {
		d := lock.Dependencies[name]
		if d == nil {
			return nil, fmt.Errorf("missing locked dependency %s", name)
		}
		path := p.dependencyPathLocked(c, name, d)
		if d.Type == "url" {
			if _, err := os.Stat(filepath.Join(path, "CMakeLists.txt")); os.IsNotExist(err) {
				path = filepath.Join(p.buildPath(c, ""), "_deps", strings.ToLower(safeName(name))+"-src")
			} else if err != nil {
				return nil, err
			}
		}
		// Include cached overrides too, rather than silently omitting a possible build input.
		paths := []string{path}
		if v := cache[strings.ToLower(safeName(name))+"_SOURCE_DIR"]; v != "" {
			paths = append(paths, v)
		}
		if v := cache["FETCHCONTENT_SOURCE_DIR_"+strings.ToUpper(safeName(name))]; v != "" {
			paths = append(paths, v)
		}
		if dep := c.Dependencies[name]; dep != nil && dep.OverrideVar != "" {
			if v := cache[dep.OverrideVar]; v != "" {
				paths = append(paths, v)
			}
		}
		seen := map[string]bool{}
		n := 0
		for _, path := range paths {
			if !filepath.IsAbs(path) {
				path = filepath.Join(root, path)
			}
			path, err = filepath.Abs(path)
			if err != nil {
				return nil, err
			}
			if seen[path] {
				continue
			}
			seen[path] = true
			if path == root || integrityWithin(path, root) {
				return nil, fmt.Errorf("dependency %s root overlaps project ancestor: %s", name, path)
			}
			id := "dependency:" + name
			if n > 0 {
				id = fmt.Sprintf("%s:source%d", id, n)
			}
			n++
			roots = append(roots, integrityRoot{ID: id, Path: path})
			if integrityWithin(root, path) {
				roots[0].Exclude = append(roots[0].Exclude, path)
			}
		}
	}
	for i := range roots {
		for j := range roots[i].Exclude {
			roots[i].Exclude[j], err = filepath.Abs(roots[i].Exclude[j])
			if err != nil {
				return nil, err
			}
			if roots[i].Exclude[j] == roots[i].Path {
				return nil, fmt.Errorf("cannot exclude entire source root")
			}
		}
		sort.Strings(roots[i].Exclude)
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].ID < roots[j].ID })
	return roots, nil
}

func scanIntegrity(roots []integrityRoot, objects string) (*integrityManifest, error) {
	m := &integrityManifest{Schema: 1, Roots: roots, Files: []integrityFile{}}
	for _, root := range roots {
		// Reject redirected roots and symlinks instead of pretending their targets are covered.
		resolved, err := filepath.EvalSymlinks(root.Path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", root.ID, err)
		}
		a, _ := filepath.Abs(resolved)
		b, _ := filepath.Abs(root.Path)
		if a != b {
			return nil, fmt.Errorf("%s: symlink/reparse source root is unsupported", root.ID)
		}
		st, err := os.Lstat(root.Path)
		if err != nil {
			return nil, err
		}
		if !st.IsDir() {
			return nil, fmt.Errorf("%s is not a directory", root.ID)
		}
		err = filepath.WalkDir(root.Path, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			// Only the project's public signer list is included from .zap.
			zapDir := filepath.Join(root.Path, ".zap")
			if root.ID == "project" && path == zapDir {
				if d.Type()&os.ModeSymlink != 0 || !d.IsDir() {
					return fmt.Errorf("unsafe .zap directory")
				}
				return nil
			}
			if root.ID == "project" && strings.HasPrefix(path, zapDir+string(filepath.Separator)) && path != filepath.Join(zapDir, "signers.json") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if path != root.Path && (d.Name() == ".git" || d.Name() == ".zap") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			for _, excluded := range root.Exclude {
				if root.ID == "project" && path == filepath.Join(zapDir, "signers.json") {
					break
				}
				if integrityWithin(excluded, path) {
					if d.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
			}
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("symlink is outside supported integrity scope: %s", path)
			}
			if d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("non-regular source file: %s", path)
			}
			rel, err := filepath.Rel(root.Path, path)
			if err != nil {
				return err
			}
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			opened, err := f.Stat()
			if err != nil {
				f.Close()
				return err
			}
			if !os.SameFile(info, opened) {
				f.Close()
				return fmt.Errorf("source changed while opening: %s", path)
			}
			h := sha256.New()
			var tmp *os.File
			var w io.Writer = h
			if objects != "" {
				tmp, err = os.CreateTemp(objects, "capture-")
				if err != nil {
					f.Close()
					return err
				}
				defer os.Remove(tmp.Name())
				w = io.MultiWriter(h, tmp)
			}
			_, copyErr := io.Copy(w, f)
			after, statErr := f.Stat()
			closeErr := f.Close()
			if tmp != nil {
				if err := tmp.Close(); copyErr == nil {
					copyErr = err
				}
			}
			if copyErr != nil {
				return copyErr
			}
			if statErr != nil {
				return statErr
			}
			if closeErr != nil {
				return closeErr
			}
			if opened.Size() != after.Size() || !opened.ModTime().Equal(after.ModTime()) {
				return fmt.Errorf("source changed while hashing: %s", path)
			}
			digest := hex.EncodeToString(h.Sum(nil))
			if tmp != nil {
				target := filepath.Join(objects, digest)
				if _, err := os.Lstat(target); os.IsNotExist(err) {
					if err := os.Rename(tmp.Name(), target); err != nil {
						return err
					}
				} else if err != nil {
					return err
				}
			}
			m.Files = append(m.Files, integrityFile{Root: root.ID, Path: filepath.ToSlash(rel), SHA256: digest, Executable: info.Mode().Perm()&0111 != 0})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("integrity scan %s: %w", root.ID, err)
		}
	}
	sort.Slice(m.Files, func(i, j int) bool {
		a, b := m.Files[i], m.Files[j]
		if a.Root != b.Root {
			return a.Root < b.Root
		}
		return a.Path < b.Path
	})
	return m, nil
}

func integrityChanges(before, after *integrityManifest) []integrityChange {
	changes := []integrityChange{}
	a, b := map[string]integrityFile{}, map[string]integrityFile{}
	if before != nil {
		for _, f := range before.Files {
			a[f.Root+"/"+f.Path] = f
		}
	}
	for _, f := range after.Files {
		b[f.Root+"/"+f.Path] = f
	}
	keys := map[string]bool{}
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	for k := range keys {
		old, oldOK := a[k]
		next, nextOK := b[k]
		switch {
		case !oldOK:
			changes = append(changes, integrityChange{Kind: "ADDED", Path: k, After: &next})
		case !nextOK:
			changes = append(changes, integrityChange{Kind: "DELETED", Path: k, Before: &old})
		case old != next:
			changes = append(changes, integrityChange{Kind: "MODIFIED", Path: k, Before: &old, After: &next})
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes
}
func integrityBytes(m *integrityManifest) []byte {
	b, _ := json.MarshalIndent(portableIntegrity(m), "", "  ")
	return append(b, '\n')
}
