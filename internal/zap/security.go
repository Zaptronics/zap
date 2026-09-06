// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	dependencyNameRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)
	cmakeTargetRE    = regexp.MustCompile(`^[A-Za-z0-9_.+\-]+(?:::[A-Za-z0-9_.+\-]+)*$`)
	cmakeVarRE       = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	gitCommitRE      = regexp.MustCompile(`^[0-9A-Fa-f]{40}$`)
	gitRefRE         = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/+:-]{0,255}$`)
	sha256RE         = regexp.MustCompile(`^SHA256=[0-9A-Fa-f]{64}$`)
	sparsePathRE     = regexp.MustCompile(`^[A-Za-z0-9._+\-]+(?:/[A-Za-z0-9._+\-]+)*$`)
)

func validateConfig(cfg *Config, requireGitLocks bool) error {
	if cfg == nil {
		return fmt.Errorf("nil config")
	}
	cfg.normalize()
	if cfg.Schema < 1 || cfg.Schema > ConfigSchema {
		return fmt.Errorf("unsupported zap.yml schema %d; this zap supports schemas 1 through %d", cfg.Schema, ConfigSchema)
	}
	if !cmakeTargetRE.MatchString(cfg.Project.Target) {
		return fmt.Errorf("project.target %q is not a safe CMake target name", cfg.Project.Target)
	}
	switch cfg.Build.Configuration {
	case "Debug", "Release", "RelWithDebInfo", "MinSizeRel":
	default:
		return fmt.Errorf("build.configuration %q is invalid; use Debug, Release, RelWithDebInfo, or MinSizeRel", cfg.Build.Configuration)
	}
	if strings.ContainsAny(cfg.Build.Generator, "\r\n\x00") {
		return fmt.Errorf("build.generator contains invalid control characters")
	}
	for _, name := range cfg.Build.CMakeOrder {
		if !cmakeVarRE.MatchString(name) {
			return fmt.Errorf("build.cmake key %q is not a safe CMake cache variable name", name)
		}
		v := cfg.Build.CMake[name]
		if strings.ContainsAny(v, "\r\n\x00") {
			return fmt.Errorf("build.cmake value for %q contains invalid control characters", name)
		}
		if strings.HasPrefix(v, "env:") {
			envName := strings.TrimPrefix(v, "env:")
			if !cmakeVarRE.MatchString(envName) {
				return fmt.Errorf("build.cmake value for %q references invalid environment variable %q", name, envName)
			}
		}
	}
	if cfg.Upload.Default != "" && !strings.EqualFold(cfg.Upload.Default, "auto") {
		if !dependencyNameRE.MatchString(cfg.Upload.Default) {
			return fmt.Errorf("upload.default %q is not a safe method name", cfg.Upload.Default)
		}
	}
	for _, name := range cfg.Upload.Order {
		if !dependencyNameRE.MatchString(name) {
			return fmt.Errorf("upload.order method %q is invalid", name)
		}
		if cfg.Upload.Methods[name] == nil {
			return fmt.Errorf("upload.order references undefined method %q", name)
		}
	}
	for _, name := range cfg.Upload.MethodOrder {
		m := cfg.Upload.Methods[name]
		if !dependencyNameRE.MatchString(name) {
			return fmt.Errorf("upload method name %q is invalid", name)
		}
		if m == nil {
			return fmt.Errorf("upload method %q is missing configuration", name)
		}
		switch strings.ToLower(m.Type) {
		case "volume-copy":
			if len(m.VolumeLabels) == 0 {
				return fmt.Errorf("upload method %q type volume-copy requires volume_labels", name)
			}
		case "command":
			if strings.TrimSpace(m.Executable) == "" {
				return fmt.Errorf("upload method %q type command requires executable", name)
			}
		default:
			return fmt.Errorf("upload method %q has unsupported type %q", name, m.Type)
		}
		for field, value := range map[string]string{"artifact": m.Artifact, "executable": m.Executable, "working_dir": m.WorkingDir} {
			if strings.ContainsAny(value, "\r\n\x00") {
				return fmt.Errorf("upload method %q %s contains invalid control characters", name, field)
			}
		}
		for _, v := range append(append(append([]string{}, m.VolumeLabels...), m.Search...), m.Args...) {
			if strings.ContainsAny(v, "\r\n\x00") {
				return fmt.Errorf("upload method %q contains an invalid list value", name)
			}
		}
		for _, key := range m.VariableOrder {
			if !cmakeVarRE.MatchString(key) {
				return fmt.Errorf("upload method %q variable name %q is invalid", name, key)
			}
			v := m.Variables[key]
			if strings.ContainsAny(v, "\r\n\x00") {
				return fmt.Errorf("upload method %q variable %q contains invalid control characters", name, key)
			}
			if strings.HasPrefix(v, "env:") && !cmakeVarRE.MatchString(strings.TrimPrefix(v, "env:")) {
				return fmt.Errorf("upload method %q variable %q references invalid environment variable", name, key)
			}
		}
		for _, key := range m.FindOrder {
			if !cmakeVarRE.MatchString(key) {
				return fmt.Errorf("upload method %q find name %q is invalid", name, key)
			}
			f := m.Find[key]
			if f == nil || strings.TrimSpace(f.Root) == "" || strings.TrimSpace(f.File) == "" {
				return fmt.Errorf("upload method %q find %q requires root and file", name, key)
			}
			if f.Parent < 0 || strings.ContainsAny(f.Root+f.File, "\r\n\x00") || filepath.Base(f.File) != f.File {
				return fmt.Errorf("upload method %q find %q has invalid root/file/parent", name, key)
			}
		}
		for _, key := range m.OptionalArgsOrder {
			if !cmakeVarRE.MatchString(key) {
				return fmt.Errorf("upload method %q optional_args key %q is invalid", name, key)
			}
			if _, ok := m.Variables[key]; !ok {
				return fmt.Errorf("upload method %q optional_args %q has no matching variable", name, key)
			}
			for _, a := range m.OptionalArgs[key] {
				if strings.ContainsAny(a, "\r\n\x00") {
					return fmt.Errorf("upload method %q optional_args %q contains invalid control characters", name, key)
				}
			}
		}
	}
	if cfg.Upload.Default != "" && !strings.EqualFold(cfg.Upload.Default, "auto") && cfg.Upload.Methods[cfg.Upload.Default] == nil {
		return fmt.Errorf("upload.default references undefined method %q", cfg.Upload.Default)
	}
	seenSafe := map[string]string{}
	for _, name := range cfg.DependencyOrder {
		dep := cfg.Dependencies[name]
		if dep == nil {
			return fmt.Errorf("dependency %q is missing configuration", name)
		}
		if !dependencyNameRE.MatchString(name) {
			return fmt.Errorf("dependency name %q is invalid; use letters, digits, '_' or '-' and start with a letter", name)
		}
		sn := safeName(name)
		if prior, ok := seenSafe[sn]; ok && prior != name {
			return fmt.Errorf("dependency names %q and %q collide after CMake-safe normalization", prior, name)
		}
		seenSafe[sn] = name
		if dep.OverrideVar != "" && !cmakeVarRE.MatchString(dep.OverrideVar) {
			return fmt.Errorf("dependency %q override_var %q is not a safe CMake variable name", name, dep.OverrideVar)
		}
		for _, c := range dep.Components {
			if !cmakeTargetRE.MatchString(c) {
				return fmt.Errorf("dependency %q component %q is not a safe CMake target name", name, c)
			}
		}
		switch strings.ToLower(dep.Type) {
		case "git":
			if dep.URI == "" || dep.Version == "" {
				return fmt.Errorf("dependency %q requires uri and version", name)
			}
			if !validGitRef(dep.Version) {
				return fmt.Errorf("dependency %q version/ref %q contains unsafe or invalid characters", name, dep.Version)
			}
			if dep.Commit != "" && !gitCommitRE.MatchString(dep.Commit) {
				return fmt.Errorf("dependency %q commit must be a 40-character Git commit SHA", name)
			}
			if requireGitLocks && dep.Commit == "" {
				return fmt.Errorf("dependency %q is not commit-locked; run 'zap sync' or 'zap version %s <ref>' first", name, name)
			}
		case "url":
			if dep.URI == "" {
				return fmt.Errorf("dependency %q requires uri", name)
			}
			if !sha256RE.MatchString(dep.Hash) {
				return fmt.Errorf("dependency %q is a URL dependency and requires hash: SHA256=<64 hex characters>", name)
			}
		case "path":
			if dep.URI == "" {
				return fmt.Errorf("dependency %q requires uri", name)
			}
		default:
			return fmt.Errorf("dependency %q has unsupported type %q", name, dep.Type)
		}
	}
	return nil
}

func validGitRef(s string) bool {
	if gitCommitRE.MatchString(s) {
		return true
	}
	if !gitRefRE.MatchString(s) {
		return false
	}
	if strings.Contains(s, "..") || strings.Contains(s, "@{") || strings.Contains(s, "//") || strings.HasSuffix(s, "/") || strings.HasSuffix(s, ".") {
		return false
	}
	return true
}

func validatePackageManifest(m *PackageManifest) error {
	if m == nil {
		return fmt.Errorf("nil package manifest")
	}
	m.normalize()
	if m.Schema != PackageSchema {
		return fmt.Errorf("unsupported zap-package.yml schema %d", m.Schema)
	}
	if m.Package.CMakeProject != "" && !cmakeTargetRE.MatchString(m.Package.CMakeProject) {
		return fmt.Errorf("package cmake_project %q is not a safe CMake project identifier", m.Package.CMakeProject)
	}
	for _, p := range m.Package.Paths {
		if err := validateSparsePath(p); err != nil {
			return fmt.Errorf("package path %q: %w", p, err)
		}
	}
	for _, name := range m.ComponentOrder {
		c := m.Components[name]
		if c == nil {
			return fmt.Errorf("package component %q is missing", name)
		}
		if !cmakeTargetRE.MatchString(name) {
			return fmt.Errorf("package component name %q is not a safe CMake target name", name)
		}
		if c.PicoTarget != "" && !cmakeTargetRE.MatchString(c.PicoTarget) {
			return fmt.Errorf("component %q pico target %q is not a safe CMake target name", name, c.PicoTarget)
		}
		if c.ZephyrTarget != "" && !cmakeTargetRE.MatchString(c.ZephyrTarget) {
			return fmt.Errorf("component %q zephyr target %q is not a safe CMake target name", name, c.ZephyrTarget)
		}
		for _, d := range c.Depends {
			if !cmakeTargetRE.MatchString(d) {
				return fmt.Errorf("component %q dependency %q is not a safe CMake target name", name, d)
			}
		}
		for _, p := range c.Paths {
			if err := validateSparsePath(p); err != nil {
				return fmt.Errorf("component %q path %q: %w", name, p, err)
			}
		}
	}
	return nil
}

func validateSparsePath(p string) error {
	if p == "" {
		return fmt.Errorf("path is empty")
	}
	if strings.ContainsRune(p, '\x00') {
		return fmt.Errorf("path contains NUL")
	}
	if strings.Contains(p, "\\") || !sparsePathRE.MatchString(p) {
		return fmt.Errorf("path must use simple repository-relative '/' separated segments")
	}
	if filepath.IsAbs(p) {
		return fmt.Errorf("absolute paths are not allowed")
	}
	slash := filepath.ToSlash(p)
	if strings.HasPrefix(slash, "/") || slash == "." || slash == ".." || strings.HasPrefix(slash, "../") || strings.Contains(slash, "/../") {
		return fmt.Errorf("path must stay inside the dependency repository")
	}
	clean := filepath.ToSlash(filepath.Clean(p))
	if clean != slash {
		return fmt.Errorf("path must be normalized (use %q)", clean)
	}
	if clean == ".git" || strings.HasPrefix(clean, ".git/") {
		return fmt.Errorf(".git paths are not allowed")
	}
	return nil
}

func cmakeBracket(s string) string {
	// Bracket arguments do not perform variable expansion or escape processing.
	// Increase the '=' depth until the closing delimiter cannot appear in s.
	eq := ""
	for strings.Contains(s, "]"+eq+"]") {
		eq += "="
	}
	return "[" + eq + "[" + s + "]" + eq + "]"
}
