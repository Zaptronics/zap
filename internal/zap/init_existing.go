// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

func setupWelcome(p *Project, c *Config, existing bool) {
	title := "Welcome to Zap"
	if existing {
		title = "Welcome back to Zap"
	}
	uiBanner("Setup", title+" · "+p.displayName(c))
	uiHint("⚡ Configure your project, keep dependencies repeatable, and review source changes.")
	if existing {
		uiHint("Your current settings are the defaults. Enter keeps them; nothing is saved until you approve the summary.")
	} else {
		uiHint("We will discover your build settings, review dependency sources, and prepare the Zap integration.")
	}
	uiHint("Use - to clear an optional text setting. Ctrl+C leaves setup before saving.")
}

func setupText(r *bufio.Reader, label, current string) (string, error) {
	value, err := prompt(r, label, current)
	if err != nil {
		return "", err
	}
	if value == "-" {
		return "", nil
	}
	return value, nil
}

func setupPrivacy(r *bufio.Reader, mode, label string) (string, string, error) {
	uiSection("2/4 · Your signing identity")
	uiHint("Signing can record your account and host without publishing their readable names.")
	choices := []string{"hashed - publish identifiers, hide readable account/host names", "public - publish readable account and host names"}
	selected, err := promptChoice(r, "What should your signer entry publish?", choices, indexOf([]string{"hashed", "public"}, mode))
	if err != nil {
		return "", "", err
	}
	mode = "hashed"
	if selected == choices[1] {
		mode = "public"
	}
	label, err = setupText(r, "Signer display label (optional, public; e.g. Firmware maintainer)", label)
	if err != nil {
		return "", "", err
	}
	uiHint("A display label is your chosen alias. Hashes remain guessable; the signature identifies the key, not a verified person or host.")
	return mode, label, nil
}

func (o InitOptions) supplied(name string) bool { return o.Explicit != nil && o.Explicit[name] }

type setupEdit struct {
	Path          []string
	Before, After string
	Lines         []string
	List          bool
}

func setupScalar(edits *[]setupEdit, path []string, old, next string) {
	if old != next {
		*edits = append(*edits, setupEdit{Path: path, Before: old, After: next, Lines: []string{yamlQuote(next)}})
	}
}

// Modify only the chosen supported mapping/list, preserving other lines verbatim.
func setupPatch(data []byte, edit setupEdit) ([]byte, error) {
	lines := strings.SplitAfter(string(data), "\n")
	newline := "\n"
	if bytes.Contains(data, []byte("\r\n")) {
		newline = "\r\n"
	}
	start, end := 0, len(lines)
	for depth, key := range edit.Path {
		found := -1
		indent := depth * 2
		for i := start; i < end; i++ {
			raw := strings.TrimRight(lines[i], "\r\n")
			trimmed := strings.TrimSpace(raw)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			spaces := len(raw) - len(strings.TrimLeft(raw, " "))
			if spaces != indent {
				continue
			}
			candidate, _, ok := splitKV(trimmed)
			if depth == 1 && edit.Path[0] == "dependencies" && strings.HasSuffix(trimmed, ":") {
				candidate = strings.TrimSuffix(trimmed, ":")
				ok = true
			}
			if ok && candidate == key {
				found = i
				break
			}
		}
		if found < 0 {
			if depth != len(edit.Path)-1 {
				var addition strings.Builder
				for j := depth; j < len(edit.Path)-1; j++ {
					addition.WriteString(strings.Repeat(" ", j*2) + edit.Path[j] + ":" + newline)
				}
				prefix := strings.Repeat(" ", (len(edit.Path)-1)*2) + edit.Path[len(edit.Path)-1] + ":"
				if edit.List {
					addition.WriteString(prefix + newline)
					for _, v := range edit.Lines {
						addition.WriteString(strings.Repeat(" ", len(edit.Path)*2) + "- " + yamlQuote(v) + newline)
					}
				} else {
					addition.WriteString(prefix + " " + edit.Lines[0] + newline)
				}
				if end > 0 && !strings.HasSuffix(lines[end-1], "\n") {
					lines[end-1] += newline
				}
				out := append([]string{}, lines[:end]...)
				out = append(out, addition.String())
				out = append(out, lines[end:]...)
				return []byte(strings.Join(out, "")), nil
			}
			found = end
		}
		blockEnd := found + 1
		for blockEnd < end {
			raw := strings.TrimRight(lines[blockEnd], "\r\n")
			trimmed := strings.TrimSpace(raw)
			if trimmed != "" && !strings.HasPrefix(trimmed, "#") && len(raw)-len(strings.TrimLeft(raw, " ")) <= indent {
				break
			}
			blockEnd++
		}
		if depth != len(edit.Path)-1 {
			start, end = found+1, blockEnd
			continue
		}
		prefix := strings.Repeat(" ", indent) + key + ":"
		replacement := []string{}
		if edit.List {
			replacement = append(replacement, prefix+newline)
			for _, v := range edit.Lines {
				replacement = append(replacement, strings.Repeat(" ", indent+2)+"- "+yamlQuote(v)+newline)
			}
		} else {
			replacement = append(replacement, prefix+" "+edit.Lines[0]+newline)
		}
		replaceEnd := found
		if found < end {
			replaceEnd = found + 1
			if edit.List {
				replaceEnd = blockEnd
				// Keep full-line comments and blank separators in an edited list block.
				for _, line := range lines[found+1 : blockEnd] {
					t := strings.TrimSpace(line)
					if t == "" || strings.HasPrefix(t, "#") {
						replacement = append(replacement, line)
					}
				}
			} else if !strings.HasSuffix(lines[found], "\n") {
				replacement[0] = strings.TrimSuffix(replacement[0], newline)
			}
		}
		if found > 0 && !strings.HasSuffix(lines[found-1], "\n") {
			lines[found-1] += newline
		}
		out := append([]string{}, lines[:found]...)
		out = append(out, replacement...)
		out = append(out, lines[replaceEnd:]...)
		return []byte(strings.Join(out, "")), nil
	}
	return data, nil
}

type setupWrite struct {
	path          string
	before, after []byte
	exists        bool
}

func setupPrepare(path string, after []byte) (setupWrite, error) {
	before, err := integrityRead(path)
	if err != nil && !os.IsNotExist(err) {
		return setupWrite{}, err
	}
	return setupWrite{path: path, before: before, after: after, exists: err == nil}, nil
}

func setupCommit(writes []setupWrite) error {
	for _, w := range writes {
		data, err := integrityRead(w.path)
		if w.exists {
			if err != nil || !bytes.Equal(data, w.before) {
				return fmt.Errorf("%s changed during setup; run init again", w.path)
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("%s appeared during setup; run init again", w.path)
		}
	}
	for i, w := range writes {
		if err := os.MkdirAll(filepath.Dir(w.path), 0700); err != nil {
			return fmt.Errorf("setup stopped before %s; earlier changes may have been saved: %w", w.path, err)
		}
		if err := integrityAtomicWrite(w.path, w.after); err != nil {
			// Best-effort rollback of this process's earlier writes, never another writer.
			rollbackOK := true
			for j := i - 1; j >= 0; j-- {
				prev := writes[j]
				now, e := integrityRead(prev.path)
				if e != nil || !bytes.Equal(now, prev.after) {
					rollbackOK = false
					continue
				}
				if prev.exists {
					e = integrityAtomicWrite(prev.path, prev.before)
				} else {
					e = os.Remove(prev.path)
				}
				if e != nil {
					rollbackOK = false
				}
			}
			return fmt.Errorf("save %s failed (earlier writes restored: %t): %w", w.path, rollbackOK, err)
		}
	}
	return nil
}

func (p *Project) initExisting(opts InitOptions) error {
	original, err := integrityRead(p.ConfigPath)
	if err != nil {
		return err
	}
	current, err := ParseConfig(string(original))
	if err != nil {
		return err
	}
	next, err := ParseConfig(string(original))
	if err != nil {
		return err
	}
	pref, err := loadSignerPreferences(p.Root)
	if err != nil {
		return err
	}
	mode, label := pref.Mode, pref.Label
	if opts.supplied("signer-identity") {
		mode, err = signerIdentityMode(opts.SignerIdentity)
		if err != nil {
			return err
		}
	}
	if opts.supplied("signer-label") {
		label = opts.SignerLabel
	}
	interactive := IsInteractive() && !opts.NonInteractive
	r := bufio.NewReader(os.Stdin)
	setupWelcome(p, current, true)
	if opts.Force {
		uiHint("--force is no longer needed: existing setup preserves settings and asks before saving.")
	}
	uiSection("1/4 · Project identity")
	if current.Project.Name == "" {
		uiHint("No display name is stored. You can enter one or keep the folder-name fallback.")
	}
	fields := []struct {
		flag, label string
		path        []string
		old         string
		dest        *string
		override    string
	}{
		{"name", "Project display name", []string{"project", "name"}, current.Project.Name, &next.Project.Name, opts.Name},
		{"environment", "Environment (pico-sdk, zephyr, generic)", []string{"project", "environment"}, current.Project.Environment, &next.Project.Environment, opts.Environment},
		{"target", "Application CMake target", []string{"project", "target"}, current.Project.Target, &next.Project.Target, opts.Target},
		{"board", "Board (optional)", []string{"project", "board"}, current.Project.Board, &next.Project.Board, opts.Board},
		{"build-dir", "Build folder", []string{"project", "build_dir"}, current.Project.BuildDir, &next.Project.BuildDir, opts.BuildDir},
		{"deps-dir", "Dependency folder", []string{"project", "deps_dir"}, current.Project.DepsDir, &next.Project.DepsDir, opts.DepsDir},
		{"adapters-dir", "Generated adapter folder", []string{"project", "adapters_dir"}, current.Project.AdaptersDir, &next.Project.AdaptersDir, opts.AdaptersDir},
		{"", "Build configuration", []string{"build", "configuration"}, current.Build.Configuration, &next.Build.Configuration, ""},
		{"", "Build generator (optional)", []string{"build", "generator"}, current.Build.Generator, &next.Build.Generator, ""},
	}
	var edits []setupEdit
	for i, f := range fields {
		if i == 1 {
			if interactive {
				mode, label, err = setupPrivacy(r, mode, label)
				if err != nil {
					return err
				}
			}
			uiSection("3/4 · Build and dependencies")
			uiHint("The target is the CMake application target; the display name is only for people. Existing lockfiles and generated files stay in place.")
		}
		if f.flag != "" && opts.supplied(f.flag) {
			*f.dest = f.override
		}
		if interactive {
			*f.dest, err = setupText(r, f.label, *f.dest)
			if err != nil {
				return err
			}
		}
		setupScalar(&edits, f.path, f.old, *f.dest)
	}
	for _, name := range next.DependencyOrder {
		dep := next.Dependencies[name]
		if dep == nil {
			continue
		}
		uiDetail("Dependency", auditText(name)+" ("+dep.Type+")")
		editDependency := name == "zapee" && (opts.supplied("zapee-uri") || opts.supplied("zapee-version") || opts.supplied("component"))
		if interactive {
			uiDetail("Source", auditText(dep.URI))
			answer, e := prompt(r, "Review settings for "+auditText(name)+"? (yes/no)", "no")
			if e != nil {
				return e
			}
			editDependency = editDependency || auditYes(answer)
		}
		if !editDependency {
			continue
		}
		old := current.Dependencies[name]
		if name == "zapee" {
			if opts.supplied("zapee-uri") {
				dep.URI = opts.ZapEEURI
			}
			if opts.supplied("zapee-version") {
				dep.Version = opts.ZapEEVersion
			}
			if opts.supplied("component") {
				dep.Components = append([]string{}, opts.Components...)
			}
		}
		if interactive {
			dep.URI, err = setupText(r, "Source URI / local path", dep.URI)
			if err != nil {
				return err
			}
			if dep.Type == "git" {
				dep.Version, err = setupText(r, "Version / reference", dep.Version)
				if err != nil {
					return err
				}
			}
			if dep.Type == "url" {
				dep.Hash, err = setupText(r, "Archive hash", dep.Hash)
				if err != nil {
					return err
				}
			}
			value, e := setupText(r, "Components (comma-separated; - clears)", strings.Join(dep.Components, ", "))
			if e != nil {
				return e
			}
			dep.Components = nil
			for _, part := range strings.Split(value, ",") {
				if part = strings.TrimSpace(part); part != "" {
					dep.Components = append(dep.Components, part)
				}
			}
		}
		for _, f := range []struct{ key, old, next string }{{"uri", old.URI, dep.URI}, {"version", old.Version, dep.Version}, {"hash", old.Hash, dep.Hash}} {
			setupScalar(&edits, []string{"dependencies", name, f.key}, f.old, f.next)
		}
		if strings.Join(old.Components, "\x00") != strings.Join(dep.Components, "\x00") {
			edits = append(edits, setupEdit{Path: []string{"dependencies", name, "components"}, Before: strings.Join(old.Components, ", "), After: strings.Join(dep.Components, ", "), Lines: dep.Components, List: true})
		}
	}
	if next.Dependencies["zapee"] == nil && (opts.supplied("zapee-uri") || opts.supplied("zapee-version") || opts.supplied("component")) {
		return fmt.Errorf("this project has no zapee dependency; setup will not add one implicitly")
	}
	proposed := append([]byte{}, original...)
	for _, edit := range edits {
		proposed, err = setupPatch(proposed, edit)
		if err != nil {
			return err
		}
	}
	parsed, err := ParseConfig(string(proposed))
	if err != nil {
		return err
	}
	for _, cfg := range []*Config{parsed, next} {
		for _, dep := range cfg.Dependencies {
			if dep != nil && len(dep.Components) == 0 {
				dep.Components = nil
			}
		}
	}
	if !reflect.DeepEqual(parsed, next) {
		return fmt.Errorf("setup could not preserve the manifest structure; no changes saved")
	}
	if len(edits) > 0 {
		if err := validateConfig(parsed, false); err != nil {
			return err
		}
	}
	uiSection("4/4 · Review before saving")
	var writes []setupWrite
	if !bytes.Equal(original, proposed) {
		for _, edit := range edits {
			uiDetail(strings.Join(edit.Path, "."), auditText(edit.Before)+" -> "+auditText(edit.After))
		}
		writes = append(writes, setupWrite{path: p.ConfigPath, before: original, after: proposed, exists: true})
	}
	if mode != pref.Mode || label != pref.Label {
		uiDetail("Signer privacy", pref.Mode+" -> "+mode)
		if label != pref.Label {
			uiDetail("Public display label", auditText(pref.Label)+" -> "+auditText(label))
		}
		path, err := integrityTrustPath(p.Root)
		if err != nil {
			return err
		}
		data, err := json.MarshalIndent(signerIdentityPreference{Mode: mode, Label: label}, "", "  ")
		if err != nil {
			return err
		}
		w, err := setupPrepare(path+".preferences.json", append(data, '\n'))
		if err != nil {
			return err
		}
		writes = append(writes, w)
	}
	if interactive {
		// Only this checkout's own enrolled key can have its descriptive fields changed.
		registry, changed, e := proposeSignerLabels(p.Root, mode, label)
		if e != nil {
			uiWarning("Could not prepare an existing signer-label update: " + e.Error())
		}
		if e == nil && changed {
			uiHint("Your existing public signer entry uses different labels. Updating it changes the signed inventory and needs a later zap audit.")
			answer, e := prompt(r, "Also update your existing signer entry? (yes/no)", "no")
			if e != nil {
				return e
			}
			if auditYes(answer) {
				writes = append(writes, registry)
				uiDetail("Signer entry", "Update only your labels; retain keys and teammates' entries")
			}
		}
	}
	if len(writes) == 0 {
		uiSuccess("Everything is already up to date. No setup changes saved.")
		return nil
	}
	if !interactive && !opts.Yes {
		return fmt.Errorf("setup changes were previewed only; add --yes to apply explicit non-interactive changes")
	}
	if interactive {
		answer, err := prompt(r, "Save these setup changes? (yes/no)", "no")
		if err != nil {
			return err
		}
		if !auditYes(answer) {
			uiHint("Setup cancelled. No proposed changes saved.")
			return nil
		}
	}
	if err := setupCommit(writes); err != nil {
		return err
	}
	uiResult("SETUP UPDATED", uiRow{Label: "Project", Value: p.displayName(next)}, uiRow{Label: "Files saved", Value: fmt.Sprint(len(writes))})
	uiHint("⚡ Next: zap sync applies dependency/build configuration changes; zap audit reviews the updated source and signer labels.")
	uiHint("Run zap init again any time to discover new setup options.")
	return nil
}
